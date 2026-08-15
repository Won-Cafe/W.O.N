// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package memory

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"won/proxy/core/session"
)

// Dựng một phiên đã đi qua nhiều phiên trước, với sổ trang cho sẵn.
func sessionAt(t *testing.T, sessions int, opens map[string]int) *session.Session {
	t.Helper()
	return sessionAtTurn(t, sessions, opens, 1)
}

// sessionAtTurn — như trên, rồi đẩy chính phiên cuối tới lượt người thứ `turn`. Sổ kho chỉ
// đặt xuống từ lượt hai, nên một phiên đứng ở lượt một không kiểm được nhịp của nó.
func sessionAtTurn(t *testing.T, sessions int, opens map[string]int, turn int) *session.Session {
	t.Helper()
	st := session.NewStore(filepath.Join(t.TempDir(), "state.json"))
	now := time.Now()
	var s *session.Session
	var key string
	for i := 1; i <= sessions; i++ {
		key = "k" + string(rune('a'+i))
		s = st.Touch(key, "", "Tzu", session.Reach{}, 1, now)
		s.SeePages([]string{"moments/2026-07-10-alpha.md", "moments/2026-07-19-beta.md", "personal/self.md"})
		for path, n := range opens {
			if i <= n {
				s.Open(path, "ruột")
			}
		}
		st.Flush()
	}
	for i := 2; i <= turn; i++ {
		s = st.Touch(key, "", "Tzu", session.Reach{}, i, now)
	}
	return s
}

// Lời nhắc nói SỐ, không phán: không một chữ "đang phai", "nên dời", "quá lâu".
// Đệ đọc con số rồi tự quyết gọi Shu hay thôi.
func TestNudgeSpeaksNumbersNotVerdicts(t *testing.T) {
	// beta mở 4 phiên đầu, self mở đủ 6 phiên (lành mạnh), alpha chưa lần nào.
	sess := sessionAt(t, 6, map[string]int{"moments/2026-07-19-beta.md": 4, "personal/self.md": 6})
	pages := []page{
		{Path: "moments/2026-07-10-alpha.md", Head: "Alpha"},
		{Path: "moments/2026-07-19-beta.md", Head: "Beta"},
		{Path: "personal/self.md", Head: "Self"},
	}
	got := nudge(pages, sess, 10, "Shu")
	if got == "" {
		t.Fatal("kho đã đi qua 6 phiên mà không có gì để nhắc")
	}
	for _, want := range []string{"moments/2026-07-19-beta.md — mở", "chưa lần nào được mở lại", "phiên"} {
		if !strings.Contains(got, want) {
			t.Errorf("thiếu %q:\n%s", want, got)
		}
	}
	// HAI đầu sổ phải cùng có mặt: cắt theo một thứ tự chung thì đầu "chưa ai mở"
	// bị đẩy khỏi bảng — bắt được khi chạy thật.
	if !strings.Contains(got, "alpha.md") {
		t.Errorf("đầu đang-nhạt bị đẩy khỏi bảng:\n%s", got)
	}
	// Trang ở vùng bền mà vẫn được mở: không gọi ai làm gì, không nhắc.
	if strings.Contains(got, "personal/self.md") {
		t.Errorf("trang lành mạnh không cần nhắc:\n%s", got)
	}
	for _, verdict := range []string{"đang phai", "nên dời", "quá lâu", "hãy ", "cần phải"} {
		if strings.Contains(got, verdict) {
			t.Errorf("đang phán thay đệ: %q\n%s", verdict, got)
		}
	}
	// Trang được mở nhiều nhất nổi lên trước — đó là phép sắp, không phải phép phán.
	if i, j := strings.Index(got, "beta.md"), strings.Index(got, "alpha.md"); i < 0 || j < 0 || i > j {
		t.Errorf("trang đã chín phải đứng trước:\n%s", got)
	}
}

// Trang vừa viết xong mà đã bị hỏi "sao chưa ai mở lại" thì lời nhắc thành cằn nhằn.
func TestNudgeSkipsYoungPages(t *testing.T) {
	sess := sessionAt(t, 2, nil)
	pages := []page{{Path: "moments/2026-07-10-alpha.md"}, {Path: "personal/self.md"}}
	if got := nudge(pages, sess, 10, "Shu"); got != "" {
		t.Errorf("trang mới chưa đủ tuổi để nhắc:\n%s", got)
	}
}

// Sổ kho xuống NHỊP LƯỢT, không nằm trong lời hệ thống: nó là thứ duy nhất trong khối
// này gọi tới một việc, và một lời gọi việc chôn dưới tiền tố là một lời bị lướt qua.
func TestNoteGoesToTheTurnBlockNotTheSystemBlock(t *testing.T) {
	root := writeStore(t, map[string]string{"working/a.md": focusPage, "personal/self.md": selfPage})
	m := newMem(t, root, nil)
	sess := sessionAtTurn(t, 5, nil, 2)
	sess.SetNote("nhắc lần đầu")

	c, _ := m.give(context.Background(), snap(), sess)
	if !strings.Contains(c.Usr, "nhắc lần đầu") {
		t.Errorf("sổ kho phải xuống khối của lượt:\n%s", c.Usr)
	}
	if strings.Contains(c.Sys, "nhắc lần đầu") {
		t.Errorf("sổ kho không được nằm trong lời hệ thống:\n%s", c.Sys)
	}
}

// Nói một lần rồi thôi: lõi ghim khối của lượt lại đúng chỗ nó, nên nói lại mỗi lượt
// chỉ là chép lại chính nó vào ngữ cảnh.
func TestNoteSpeaksOncePerSession(t *testing.T) {
	root := writeStore(t, map[string]string{"working/a.md": focusPage, "personal/self.md": selfPage})
	m := newMem(t, root, nil)
	sess := sessionAtTurn(t, 5, nil, 2)
	sess.SetNote("nhắc lần đầu")

	c, _ := m.give(context.Background(), snap(), sess)
	if !strings.Contains(c.Usr, "nhắc lần đầu") {
		t.Fatalf("lần đầu phải nói:\n%s", c.Usr)
	}
	if got := sess.Note(); got != noteSpent {
		t.Errorf("nói xong phải đánh dấu đã tiêu, got %q", got)
	}
	c2, _ := m.give(context.Background(), snapAsking("hỏi tiếp"), sess)
	if c2 != nil && strings.Contains(c2.Usr, "nhắc lần đầu") {
		t.Errorf("đã nói rồi thì không nói lại:\n%s", c2.Usr)
	}
}

// Lượt đầu chưa đặt sổ xuống — cùng cửa với bộ chọn, cùng lý do: đệ chưa có gì để sổ
// phải chạm vào. Và sổ vẫn còn nguyên, chờ lượt hai.
func TestNoteWaitsForTheSecondHumanTurn(t *testing.T) {
	root := writeStore(t, map[string]string{"working/a.md": focusPage, "personal/self.md": selfPage})
	m := newMem(t, root, nil)
	sess := sessionAt(t, 5, nil)
	sess.SetNote("nhắc lần đầu")

	c, _ := m.give(context.Background(), snap(), sess)
	if c.Usr != "" {
		t.Errorf("lượt đầu chưa có sổ:\n%s", c.Usr)
	}
	if got := sess.Note(); got != "nhắc lần đầu" {
		t.Errorf("chưa nói thì chưa được tiêu, got %q", got)
	}
}

// Giữa vòng tool thì lõi không đặt khối mới xuống, nên tiêu sổ ở đó là mất hẳn lời
// nhắc của cả phiên. Cửa của plugin phải khớp cửa của Apply.
func TestNoteNotSpentInsideToolLoop(t *testing.T) {
	root := writeStore(t, map[string]string{"working/a.md": focusPage, "personal/self.md": selfPage})
	m := newMem(t, root, nil)
	sess := sessionAtTurn(t, 5, nil, 2)
	sess.SetNote("nhắc lần đầu")

	loop := snap()
	loop.HumanSpokeLast = false
	c, _ := m.give(context.Background(), loop, sess)
	if c.Usr != "" {
		t.Errorf("giữa vòng tool không đặt khối mới:\n%s", c.Usr)
	}
	if got := sess.Note(); got != "nhắc lần đầu" {
		t.Errorf("sổ phải còn nguyên cho lượt người kế, got %q", got)
	}
}

// nudge trả rỗng → Contribute đặt dấu đã-tiêu, không tính lại ở lượt sau.
func TestNudgeEmptySetsSentinelNoRecompute(t *testing.T) {
	root := writeStore(t, map[string]string{"working/a.md": focusPage, "personal/self.md": selfPage})
	m := newMem(t, root, nil)
	// Phiên 2: sổ chưa đủ phiên (nudgeAge=3) → nudge trả rỗng.
	sess := sessionAt(t, 2, nil)

	c, _ := m.give(context.Background(), snap(), sess)
	if strings.Contains(c.Text, "sổ kho") {
		t.Errorf("nudge rỗng không được in lời nhắc:\n%s", c.Text)
	}
	if got := sess.Note(); got != noteSpent {
		t.Errorf("nudge rỗng phải đặt dấu đã tiêu, got %q", got)
	}

	c2, _ := m.give(context.Background(), snapAsking("hỏi tiếp"), sess)
	if strings.Contains(c2.Text, "sổ kho") {
		t.Errorf("dấu đã tiêu không được in thành lời nhắc:\n%s", c2.Text)
	}
	if got := sess.Note(); got != noteSpent {
		t.Errorf("dấu phải giữ nguyên, got %q", got)
	}
}

// Sổ kho đo bằng PHIÊN trên sổ trang của chính nó, không bằng ngày trên dấu thời gian
// của tệp — cái đó là trục của Wayfarer. Hai dòng về hiện tại: quãng kho không lớn, và
// working/ đang mở gì.
func TestNudgeReadsThePresentNotOnlyOldPages(t *testing.T) {
	sess := sessionAt(t, 6, map[string]int{"personal/self.md": 6})
	pages := []page{
		{Path: "moments/2026-07-10-alpha.md", Head: "Alpha"},
		{Path: "moments/2026-07-19-beta.md", Head: "Beta"},
		{Path: "personal/self.md", Head: "Self"},
	}
	got := nudge(pages, sess, 10, "Shu")
	if !strings.Contains(got, "5 phiên kho chưa nhận trang nào mới") {
		t.Errorf("thiếu quãng kho đứng yên, đo bằng phiên:\n%s", got)
	}
	if !strings.Contains(got, "`working/` — không trang nào đang mở") {
		t.Errorf("working/ rỗng là dữ kiện, phải nói ra:\n%s", got)
	}
	// Trục của Wayfarer không được lẫn sang đây.
	for _, wrong := range []string{"ngày trước", "giờ trước"} {
		if strings.Contains(got, wrong) {
			t.Errorf("đang đo bằng trục của Wayfarer: %q\n%s", wrong, got)
		}
	}
}

// Kho vừa nhận trang trong phiên này thì không còn quãng đứng yên nào để đếm — im.
func TestNudgeSilentOnGrowthWhenStoreJustGrew(t *testing.T) {
	sess := sessionAt(t, 6, map[string]int{"personal/self.md": 6})
	pages := []page{
		{Path: "moments/2026-07-10-alpha.md", Head: "Alpha"},
		{Path: "moments/2026-07-19-beta.md", Head: "Beta"},
		{Path: "personal/self.md", Head: "Self"},
		{Path: "working/2026-08-08-vua-viet.md", Head: "Vừa viết"},
	}
	got := nudge(pages, sess, 10, "Shu")
	if strings.Contains(got, "chưa nhận trang nào mới") {
		t.Errorf("kho vừa lớn mà vẫn kể quãng đứng yên:\n%s", got)
	}
	if !strings.Contains(got, "`working/` — 1 trang đang mở") {
		t.Errorf("phải đếm được trang working/ đang mở:\n%s", got)
	}
}
