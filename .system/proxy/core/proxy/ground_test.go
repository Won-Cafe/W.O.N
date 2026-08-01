// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package proxy

import (
	"strings"
	"testing"
	"time"

	"won/proxy/core/plugin"
	"won/proxy/core/request"
	"won/proxy/core/session"
)

func parseBody(t *testing.T, raw string) *request.Body {
	t.Helper()
	b, err := request.ParseBody([]byte(raw), request.FormatAnthropic)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// house / ground — hai lối gọi của dòng chính, gói lại cho test đọc được ý chứ
// không đọc tham số. Cả hai kiểm "đã có chưa" trên ảnh chụp lời hệ thống TRƯỚC khi
// chèn, đúng như mainline làm.
func (p *Proxy) house(body *request.Body, now time.Time) bool {
	return p.appendBlock(body, body.SystemText(), houseTag, p.houseText(now))
}

func (p *Proxy) ground(body *request.Body) bool {
	return p.appendBlock(body, body.SystemText(), groundTag, p.d.Ground)
}

// Thứ tự đọc: lời công cụ chủ → đất → bản đồ nhà → mình là ai. Khối của chủ đứng
// TRƯỚC vì nó khẳng định đệ là ai bằng một câu ta không cắt tới được, và đứng sau là
// chỗ được đọc gần lúc trả lời nhất. Trong phần của ta thì đất và nhà đi trước — chúng
// là chỗ đứng, giống nhau cho mọi đệ; bản sắc đi sau. Đây là thứ tự người dùng thấy,
// nên nó phải bị khoá bằng test.
func TestGroundBeforeSoul(t *testing.T) {
	p := &Proxy{d: Deps{Ground: "W.O.N là một vòng lặp.", House: "Circle — sáu đệ"}}
	body := parseBody(t, `{"system":"GỐC","messages":[{"role":"user","content":"chào"}]}`)

	// Đúng thứ tự dòng chính gọi: đất, nhà, rồi plugin.
	if !p.ground(body) {
		t.Fatal("có đất mà không chèn")
	}
	if !p.house(body, time.Now()) {
		t.Fatal("có bản đồ mà không chèn")
	}
	plugin.Apply(body, []plugin.Contribution{
		{Plugin: "identity", Kind: plugin.KindSystem, Tag: "Soul", Text: "# Tzu"},
	}, session.NewEphemeral().Touch("k", "", "Tzu", session.Reach{}, 1, time.Now()), nil)

	sys := body.SystemText()
	iGround := strings.Index(sys, "<W.O.N>")
	iHouse := strings.Index(sys, "<House>")
	iSoul := strings.Index(sys, "<Soul>")
	iRoot := strings.Index(sys, "GỐC")
	if iGround < 0 || iHouse < 0 || iSoul < 0 || iRoot < 0 {
		t.Fatalf("thiếu khối trong system: %q", sys)
	}
	if !(iRoot < iGround && iGround < iHouse && iHouse < iSoul) {
		t.Errorf("thứ tự phải là lời gốc → đất → nhà → bản sắc, got:\n%s", sys)
	}
	for _, close := range []string{"</W.O.N>", "</House>", "</Soul>"} {
		if !strings.Contains(sys, close) {
			t.Errorf("khối phải đóng tag: thiếu %s", close)
		}
	}
}

// Bản đồ đã có mặt thì không chèn lần hai, và vắng bản đồ thì không chạm gì.
func TestHouseInjectOnceAndNoopWhenEmpty(t *testing.T) {
	p := &Proxy{d: Deps{House: "Circle — sáu đệ"}}
	body := parseBody(t, `{"system":"GỐC","messages":[]}`)
	if !p.house(body, time.Now()) {
		t.Fatal("lần đầu phải chèn")
	}
	if p.house(body, time.Now()) {
		t.Error("lần hai phải bỏ qua")
	}
	if n := strings.Count(body.SystemText(), "<House>"); n != 1 {
		t.Errorf("đếm được %d khối nhà, muốn 1", n)
	}

	// Vắng House.md và vắng workspace → không có bản đồ, không chèn khối rỗng: bản
	// đồ là lời khai của người giữ hệ, lõi không bịa (#6).
	empty := &Proxy{}
	body2 := parseBody(t, `{"system":"GỐC","messages":[]}`)
	if empty.house(body2, time.Now()) || body2.SystemText() != "GỐC" {
		t.Errorf("vắng bản đồ mà vẫn chạm: %q", body2.SystemText())
	}
}

// Chỗ và lúc: đệ không tự biết mình đứng ở thư mục nào, và model không có đồng hồ.
func TestHouseCarriesWorkspaceAndSessionStart(t *testing.T) {
	p := &Proxy{d: Deps{House: "Circle — sáu đệ", Workspace: `C:\W.O.N`}}
	body := parseBody(t, `{"system":"GỐC","messages":[]}`)
	p.house(body, time.Date(2026, 7, 25, 22, 30, 5, 0, time.UTC))

	sys := body.SystemText()
	for _, want := range []string{
		`Workspace: C:\W.O.N`, "Session opened: 25/07/2026 - 22:30:05", "Circle — sáu đệ",
	} {
		if !strings.Contains(sys, want) {
			t.Errorf("khối nhà thiếu %q:\n%s", want, sys)
		}
	}
	iWs, iMap := strings.Index(sys, "Workspace:"), strings.Index(sys, "Circle")
	if iWs > iMap {
		t.Error("chỗ-và-lúc đứng trước bản đồ: đây là đâu, mở lúc nào, rồi mới tới nhà có ai")
	}
}

// Khối nhà đi lại ở MỌI request, nên nó phải ra cùng một chữ suốt phiên — không thì
// tiền tố cache đổi mỗi lượt, và mốc của công cụ chủ nằm sau nó cũng trượt theo.
func TestHouseIsByteStableAcrossTurns(t *testing.T) {
	p := &Proxy{d: Deps{House: "Circle", Workspace: `C:\won`}}
	opened := time.Date(2026, 7, 25, 22, 30, 5, 0, time.UTC)
	if p.houseText(opened) != p.houseText(opened) {
		t.Fatal("cùng mốc mở phiên phải ra cùng chữ")
	}
	// Dòng chính truyền sess.FirstSeen(), không truyền time.Now() — mốc mở phiên đứng
	// yên cả phiên. Test này vỡ nếu ai đổi lại thành đồng hồ chạy.
	if p.houseText(opened) == p.houseText(opened.Add(time.Second)) {
		t.Error("khuôn thời gian mất phần giây — mốc mở phiên phải phân biệt được")
	}
}

// Đất đã có mặt thì không chèn lần hai — 50KB trả tiền hai lần cho một thứ.
func TestGroundNotInjectedTwice(t *testing.T) {
	p := &Proxy{d: Deps{Ground: "đất"}}
	body := parseBody(t, `{"system":"GỐC","messages":[]}`)

	if !p.ground(body) {
		t.Fatal("lần đầu phải chèn")
	}
	if p.ground(body) {
		t.Error("lần hai phải bỏ qua")
	}
	if n := strings.Count(body.SystemText(), "<W.O.N>"); n != 1 {
		t.Errorf("đếm được %d khối đất, muốn 1", n)
	}
}

// Không có đất → không chạm gì. Đọc README lỗi thì dòng chính vẫn chảy (#2).
func TestGroundEmptyIsNoop(t *testing.T) {
	p := &Proxy{d: Deps{Ground: ""}}
	body := parseBody(t, `{"system":"GỐC","messages":[]}`)
	if p.ground(body) {
		t.Error("không có đất mà báo đã chèn")
	}
	if body.SystemText() != "GỐC" {
		t.Errorf("system bị chạm: %q", body.SystemText())
	}
}
