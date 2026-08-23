// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package wayfarer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"won/proxy/core/paths"
	"won/proxy/core/plugin"
	"won/proxy/core/request"
	"won/proxy/core/session"
)

func TestDur(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{40 * time.Second, "40 giây"},
		{9 * time.Minute, "9 phút"},
		{90 * time.Minute, "1 giờ 30 phút"},
		{50 * time.Hour, "2 ngày"},
	}
	for _, c := range cases {
		if got := dur(c.d); got != c.want {
			t.Errorf("dur(%v) = %q, muốn %q", c.d, got, c.want)
		}
	}
}

// Tuổi việc đọc từ NGÀY TRONG TÊN trang. mtime chỉ nói lần cuối trang bị sửa —
// trang đang được làm thì mtime luôn mới, nên lấy mtime làm tuổi việc là bắt đúng
// cái bị bỏ, không phải cái đang kẹt.
func TestDateInName(t *testing.T) {
	if d, ok := dateInName("2026-07-19-promise.md"); !ok || d.Format("02/01/2006") != "19/07/2026" {
		t.Errorf("đọc ngày hụt: %v %v", d, ok)
	}
	if _, ok := dateInName("khong-co-ngay.md"); ok {
		t.Error("tên không mở đầu bằng ngày thì không được bịa ra ngày")
	}
}

func writeRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	focus := filepath.Join(root, ".system", "memory", "working")
	if err := os.MkdirAll(focus, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(focus, "2026-07-19-ship.md"),
		[]byte("# Focus — ship\n\n*Đang mở.*\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// Khối Road bày SỐ, không bày lời phán: không một chữ "dài", "lâu", "bất thường".
// Ngưỡng là việc của soul; code lỡ tay cân hộ một lần là soul mất chỗ đứng.
func TestRoadCarriesNumbersNotVerdicts(t *testing.T) {
	root := writeRoot(t)
	p := &Wayfarer{}
	p.Paths = paths.Tree{Root: root}

	st := session.NewStore(filepath.Join(t.TempDir(), "state.json"))
	t0 := time.Date(2026, 7, 25, 10, 0, 0, 0, time.UTC)
	var sess *session.Session
	for i := 1; i <= 4; i++ {
		sess = st.Touch("k", "", "Tzu", session.Reach{}, i, t0.Add(time.Duration(i-1)*time.Minute))
	}
	now := t0.Add(10 * time.Minute)
	road := p.road(&request.Snapshot{Agent: "Tzu", ReceivedAt: now}, sess)

	for _, want := range []string{
		"lượt người thứ 4", "chạy 4 lần máy", "mở 10 phút trước", "lượt gần nhất cách đây 7 phút",
		"working/ đang mở: 2026-07-19-ship.md", "mở ngày 19/07/2026", "6 ngày trước",
		"Kho ký ức: ghi lần cuối", "Mốc đã cắm trong phiên này: chưa có",
	} {
		if !strings.Contains(road, want) {
			t.Errorf("Road thiếu %q:\n%s", want, road)
		}
	}
	for _, verdict := range []string{"bất thường", "quá lâu", "đã dài", "nên nghỉ"} {
		if strings.Contains(road, verdict) {
			t.Errorf("Road đang phán thay soul: %q\n%s", verdict, road)
		}
	}

	// Mốc đã cắm quay lại trong Road để soul không nhắc lại nó.
	sess.Say("Wayfarer", "🛣️ Wayfarer: vắng 4 giờ trước phiên này.")
	if r2 := p.road(&request.Snapshot{Agent: "Tzu", ReceivedAt: now}, sess); !strings.Contains(r2, "đừng cắm lại") ||
		!strings.Contains(r2, "vắng 4 giờ") {
		t.Errorf("mốc đã cắm phải quay lại trong Road:\n%s", r2)
	}
}

// Giữa một vòng tool, người chưa đi thêm bước nào — không có gì để cắm mốc, và
// không tốn một lượt gọi model.
// Giữa vòng tool thì Wayfarer không được GỌI — quãng đường đo bằng bước của người.
// Cửa ấy nằm ở lõi, không ở plugin: chỗ đây chỉ khoá lại rằng plugin đã KHAI mình là
// tiếng của lượt. Cửa thật có test riêng ở core/plugin (TestTurnVoiceSkippedInToolLoop).
func TestSilentInsideToolLoop(t *testing.T) {
	p := &Wayfarer{book: nil}
	var _ plugin.Plugin = p
	v, ok := any(p).(plugin.TurnVoice)
	if !ok || !v.SpeaksOnHumanTurn() {
		t.Error("Wayfarer phải khai mình chỉ nói ở lượt của người")
	}
}

// File mẫu không phải một việc đang mở, và không phải một lần có người đặt bút. Đo được
// trên nhật ký thật: `<Road>` khai `working/ đang mở: template-working.md` — một việc không
// ai mở bao giờ. Chỗ thứ hai nguy hơn: một lần chạm file mẫu làm cả kho đứng yên trông
// như vừa được ghi, mà đó đúng là tín hiệu Road sinh ra để nói.
func TestTemplatesAreNotPages(t *testing.T) {
	root := writeRoot(t)
	for _, f := range []struct{ zone, name string }{
		{paths.ZoneWorking, "template-working.md"},
		{paths.ZoneMoments, "template-moments.md"},
	} {
		dir := filepath.Join(root, ".system", "memory", f.zone)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, f.name), []byte("# Mẫu\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	p := &Wayfarer{}
	p.Paths = paths.Tree{Root: root}
	now := time.Date(2026, 7, 25, 10, 0, 0, 0, time.UTC)

	for _, line := range p.openFocus(now) {
		if strings.Contains(line, "template-") {
			t.Errorf("file mẫu khai thành việc đang mở: %s", line)
		}
	}
	if got := p.lastWritten(now); strings.Contains(got, "template-") {
		t.Errorf("file mẫu tính thành lần cuối đặt bút: %s", got)
	}
}
