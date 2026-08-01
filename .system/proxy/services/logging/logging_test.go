// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package logging

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

func newTestHandler(min slog.Level) (*handler, *bytes.Buffer) {
	var buf bytes.Buffer
	return &handler{w: &buf, mu: &sync.Mutex{}, min: min}, &buf
}

func emit(h *handler, lv slog.Level, msg string, args ...any) {
	at := time.Date(2026, 8, 9, 16, 10, 1, 503_000_000, time.UTC)
	r := slog.NewRecord(at, lv, msg, 0)
	r.Add(args...)
	if err := h.Handle(context.Background(), r); err != nil {
		panic(err)
	}
}

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{
		"debug": LevelDebug, "DEBUG": LevelDebug, "  debug  ": LevelDebug,
		"silent": LevelSilent, "info": LevelInfo, "": LevelInfo, "chữ lạ": LevelInfo,
	} {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, muốn %q", in, got, want)
		}
	}
}

// Dòng info không được mang ngày, không mang `level=`, không mang `msg=`: ba thứ ấy là
// khung lặp, và bốn dòng khởi động cách nhau 8ms thì khung chiếm hơn tin.
func TestHandleStripsScaffolding(t *testing.T) {
	h, buf := newTestHandler(slog.LevelInfo)
	emit(h, slog.LevelInfo, "souls loaded", "agents", 9)

	got := buf.String()
	for _, banned := range []string{"2026-08-09", "level=", "msg=", `"`} {
		if strings.Contains(got, banned) {
			t.Errorf("dòng còn khung lặp %q: %s", banned, got)
		}
	}
	if !strings.HasPrefix(got, "16:10:01   souls loaded agents=9") {
		t.Errorf("hình dòng info sai: %q", got)
	}
}

// Mức hiện bằng một dấu, và cột msg phải thẳng hàng giữa các mức — lệch cột thì mắt
// phải dò lại chỗ bắt đầu ở từng dòng.
func TestMarkAligns(t *testing.T) {
	h, buf := newTestHandler(slog.LevelDebug)
	emit(h, slog.LevelDebug, "d")
	emit(h, slog.LevelInfo, "i")
	emit(h, slog.LevelWarn, "w")
	emit(h, slog.LevelError, "e")

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("muốn 4 dòng, có %d: %q", len(lines), buf.String())
	}
	wantMarks := []string{"·", " ", "!", "✗"}
	wantMsg := []string{"d", "i", "w", "e"}
	// Canh cột đo bằng RUNE, không bằng byte: `·` và `✗` nhiều byte, nên byte-index lệch
	// trong khi mắt vẫn thấy thẳng hàng.
	for i, ln := range lines {
		rs := []rune(ln)
		if len(rs) != 12 {
			t.Fatalf("dòng %d dài %d rune, muốn 12: %q", i, len(rs), ln)
		}
		if got := string(rs[9]); got != wantMarks[i] {
			t.Errorf("dòng %d: dấu %q, muốn %q — %q", i, got, wantMarks[i], ln)
		}
		if got := string(rs[11]); got != wantMsg[i] {
			t.Errorf("dòng %d: msg bắt đầu ở cột sai — %q", i, ln)
		}
	}
}

// Dòng trắng tách từng lần chạy: mắt phải thấy đâu là đầu, đâu là cuối của một request.
// Nó đứng TRƯỚC bản ghi mở khối — một lần chạy hỏng giữa chừng thì không có bản ghi khép
// nào để bám vào.
func TestNewBlockSeparatesRuns(t *testing.T) {
	var buf bytes.Buffer
	h := &handler{w: &buf, mu: &sync.Mutex{}, min: slog.LevelDebug, opens: map[string]bool{"proxy: mainline": true}}

	emit(h, slog.LevelDebug, "proxy: mainline", "path", "/v1/messages")
	emit(h, slog.LevelDebug, "plugin: silent", "plugin", "memory")
	emit(h, slog.LevelInfo, "proxy: run", "agent", "Tzu")
	emit(h, slog.LevelDebug, "proxy: mainline", "path", "/v1/messages")

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	// trắng · mainline · plugin · run · trắng · mainline
	if len(lines) != 6 {
		t.Fatalf("muốn 6 dòng kể cả dòng trắng, có %d: %q", len(lines), buf.String())
	}
	for _, i := range []int{0, 4} {
		if lines[i] != "" {
			t.Errorf("dòng %d phải trắng, mở một khối mới: %q", i, lines[i])
		}
	}
	for _, i := range []int{1, 2, 3, 5} {
		if lines[i] == "" {
			t.Errorf("dòng %d không được trắng: %q", i, buf.String())
		}
	}
	if !strings.Contains(lines[1], "proxy: mainline") || !strings.Contains(lines[5], "proxy: mainline") {
		t.Errorf("khối phải mở bằng chính bản ghi đã khai: %q", buf.String())
	}
}

// Không khai msg nào mở khối → không dòng trắng nào. Mức info in một dòng cho một lần
// chạy, nên tự nó đã là dấu tách; thêm dòng trắng ở đó là gấp đôi chiều cao đổi lấy không.
func TestNoBlockMeansNoBlankLines(t *testing.T) {
	h, buf := newTestHandler(slog.LevelInfo)
	emit(h, slog.LevelInfo, "proxy: run", "agent", "Tzu")
	emit(h, slog.LevelInfo, "proxy: run", "agent", "Tzu")

	if strings.Contains(buf.String(), "\n\n") {
		t.Errorf("chưa khai khối thì không được có dòng trắng: %q", buf.String())
	}
}

// Trường rỗng bị bỏ hẳn: `encoding=` in ra ở mọi lượt upstream lỗi mà không nói gì —
// thấy được trên console thật.
func TestEmptyValuesAreDropped(t *testing.T) {
	h, buf := newTestHandler(slog.LevelWarn)
	emit(h, slog.LevelWarn, "upstream: error response", "status", 404, "path", "/v1/messages", "encoding", "")

	got := strings.TrimRight(buf.String(), "\n")
	if strings.Contains(got, "encoding") {
		t.Errorf("trường rỗng phải bị bỏ: %q", got)
	}
	if !strings.HasSuffix(got, "status=404 path=/v1/messages") {
		t.Errorf("các trường còn lại phải nguyên: %q", got)
	}
}

func TestEnabledRespectsLevel(t *testing.T) {
	h, buf := newTestHandler(slog.LevelInfo)
	if h.Enabled(context.Background(), slog.LevelDebug) {
		t.Fatal("mức info không được cho debug qua")
	}
	emit(h, slog.LevelInfo, "qua")
	if buf.Len() == 0 {
		t.Fatal("mức info phải cho info qua")
	}
}

// Địa chỉ phải bấm được: đó là lý do luật này có mặt. Và nó chỉ chạm host:port —
// đoán sai một địa chỉ tệ hơn để nó không bấm được.
func TestWebAddr(t *testing.T) {
	for in, want := range map[string]string{
		"127.0.0.1:7777":            "http://127.0.0.1:7777",
		"localhost:8787":            "http://localhost:8787",
		"https://api.anthropic.com": "https://api.anthropic.com",
		"http://127.0.0.1:11434":    "http://127.0.0.1:11434",
		"Tzu":                       "Tzu",
		"":                          "",
		"identity,memory":           "identity,memory",
	} {
		if got := WebAddr(in); got != want {
			t.Errorf("WebAddr(%q) = %q, muốn %q", in, got, want)
		}
	}
}

func TestWebKeysOnly(t *testing.T) {
	h, buf := newTestHandler(slog.LevelInfo)
	emit(h, slog.LevelInfo, "listening", "addr", "127.0.0.1:8787", "agent", "127.0.0.1:8787")

	got := buf.String()
	if !strings.Contains(got, "addr=http://127.0.0.1:8787") {
		t.Errorf("khoá địa chỉ phải có scheme: %s", got)
	}
	if !strings.Contains(got, "agent=127.0.0.1:8787") {
		t.Errorf("khoá ngoài danh sách không được đụng: %s", got)
	}
}

// Giá trị dài xuống dòng nối. Không ngoặc kép ở đâu thì không có gì để escape — đó là
// cách `\"` biến mất khỏi console. Ca này đo RIÊNG việc xuống dòng, dùng một câu `fix`
// chứ không dùng lỗi dial: lỗi dial còn đi qua ShortenErr, và trộn hai luật vào một ca
// thì hỏng cái nào cũng đọc ra cái kia.
func TestLongValuesWrap(t *testing.T) {
	h, buf := newTestHandler(slog.LevelWarn)
	fix := "free that port, pick another one, or comment out `control`"
	emit(h, slog.LevelWarn, "control: not opened", "addr", "127.0.0.1:7777", "fix", fix)

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("muốn dòng chính + một dòng nối, có %d: %q", len(lines), buf.String())
	}
	if !strings.HasSuffix(lines[0], "addr=http://127.0.0.1:7777") {
		t.Errorf("giá trị gọn phải ở dòng chính: %q", lines[0])
	}
	if lines[1] != indent+"fix="+fix {
		t.Errorf("giá trị dài phải xuống dòng nối, thụt đúng cột: %q", lines[1])
	}
	if strings.Contains(buf.String(), `"`) {
		t.Errorf("không được có ngoặc kép ở đâu cả: %s", buf.String())
	}
}

// Lời winsock dài 150 ký tự nói đúng một ý. Rút nó là việc của handler — một nhà, và
// core không phải import gì để được hưởng.
func TestShortenErr(t *testing.T) {
	cases := map[string]string{
		`Post "http://127.0.0.1:11434/api/generate": dial tcp 127.0.0.1:11434: connectex: No connection could be made because the target machine actively refused it.`: "dial tcp 127.0.0.1:11434: connection refused",
		"listen tcp 127.0.0.1:7777: bind: Only one usage of each socket address (protocol/network address/port) is normally permitted.":                                "listen tcp 127.0.0.1:7777: bind: address already in use",
		"lỗi thường không ai phải rút":                                 "lỗi thường không ai phải rút",
		"dial tcp 1.2.3.4:80: i/o timeout":                             "dial tcp 1.2.3.4:80: i/o timeout",
		`Get "http://x/y": dial tcp: lookup x: No such host is known.`: "dial tcp: lookup x: host not found",
	}
	for in, want := range cases {
		if got := ShortenErr(in); got != want {
			t.Errorf("ShortenErr:\n vào  %q\n ra   %q\n muốn %q", in, got, want)
		}
	}
}

// Lỗi đi qua handler phải đã được rút, và không mang một dấu escape nào.
func TestErrValueShortened(t *testing.T) {
	h, buf := newTestHandler(slog.LevelWarn)
	err := errors.New(`Post "http://127.0.0.1:11434/api/generate": dial tcp 127.0.0.1:11434: connectex: No connection could be made because the target machine actively refused it.`)
	emit(h, slog.LevelWarn, "localmodel: warm-up failed", "err", err)

	got := buf.String()
	if !strings.Contains(got, "err=dial tcp 127.0.0.1:11434: connection refused") {
		t.Errorf("lỗi chưa được rút: %s", got)
	}
	if strings.Contains(got, "connectex") || strings.Contains(got, "actively refused") {
		t.Errorf("còn lời thô của OS: %s", got)
	}
}

// `177629` phải nhẩm mới ra gần ba phút. Dòng chạy mỗi lần chạy thì không được bắt nhẩm.
func TestDuration(t *testing.T) {
	for in, want := range map[int64]string{
		0: "0ms", 7: "7ms", 181: "181ms", 999: "999ms",
		1000: "1.0s", 7439: "7.4s", 59_999: "60.0s",
		60_000: "1m00s", 177_629: "2m57s", 3_599_000: "59m59s",
		3_600_000: "1h00m", 7_530_000: "2h05m",
	} {
		if got := Duration(in); got != want {
			t.Errorf("Duration(%d) = %q, muốn %q", in, got, want)
		}
	}
}

// Khoá `took` mang mili-giây, và handler là chỗ dựng nó thành thời lượng — nhờ vậy core
// không phải import gì để được hưởng.
func TestTookRendersAsDuration(t *testing.T) {
	h, buf := newTestHandler(slog.LevelInfo)
	emit(h, slog.LevelInfo, "proxy: run", "took", int64(177629), "input", 1988)

	got := buf.String()
	if !strings.Contains(got, "took=2m57s") {
		t.Errorf("took phải thành thời lượng: %s", got)
	}
	if !strings.Contains(got, "input=1988") {
		t.Errorf("khoá khác không được đụng: %s", got)
	}
}

func TestWithAttrsAndGroup(t *testing.T) {
	h, buf := newTestHandler(slog.LevelInfo)
	sub := h.WithAttrs([]slog.Attr{slog.String("c", "memory")}).WithGroup("g")
	r := slog.NewRecord(time.Date(2026, 8, 9, 16, 10, 1, 0, time.UTC), slog.LevelInfo, "x", 0)
	r.Add("k", "v")
	if err := sub.Handle(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "c=memory") || !strings.Contains(got, "g.k=v") {
		t.Errorf("WithAttrs/WithGroup sai: %s", got)
	}
}

// Giá trị nhiều dòng: chỉ dòng ĐẦU mang `khoá=`. Dấu `=` mồ côi ở dòng hai đọc lên như
// một khoá rỗng — thấy được trên console thật trước khi sửa.
func TestMultilineValueHasNoOrphanEquals(t *testing.T) {
	h, buf := newTestHandler(slog.LevelInfo)
	emit(h, slog.LevelInfo, "localmodel: said", "text", "Edit → a.md  [tầng cơ học]\nGlob → b/*.md  [trục W.O.N]")

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("muốn dòng chính + hai dòng nối, có %d: %q", len(lines), buf.String())
	}
	if !strings.HasPrefix(lines[1], indent+"text=Edit → ") {
		t.Errorf("dòng nối đầu phải mang tên khoá: %q", lines[1])
	}
	if strings.Contains(lines[2], "=") {
		t.Errorf("dòng nối thứ hai không được có dấu `=`: %q", lines[2])
	}
	if !strings.HasSuffix(lines[2], "Glob → b/*.md  [trục W.O.N]") {
		t.Errorf("dòng nối thứ hai mất nội dung: %q", lines[2])
	}
	// Cột NỘI DUNG phải trùng — không phải số khoảng trắng đầu dòng: dòng đầu tiêu `text=`
	// làm tiền tố, dòng sau tiêu khoảng trắng đúng bằng chừng ấy.
	if strings.Index(lines[1], "Edit") != strings.Index(lines[2], "Glob") {
		t.Errorf("hai dòng nối lệch cột nội dung:\n%q\n%q", lines[1], lines[2])
	}
}

// Chi tiết của một hàng nằm DƯỚI hàng ấy — không thành một bản ghi rời in trước khối.
func TestBannerRowDetail(t *testing.T) {
	var buf bytes.Buffer
	c := &Console{w: &buf, level: LevelDebug}
	b := Banner{Title: "W.O.N Proxy Inject"}
	b.Add("agents", "9 souls + house · default Tzu")
	b.Detail("Fan Han Loiterer Mo Outfitter Shu Sun Tzu Wayfarer")
	b.Add("plugins", "identity memory")
	b.Detail("   ") // trắng thì không gắn gì
	c.Print(b)

	got := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(got) != 4 {
		t.Fatalf("muốn tiêu đề + agents + chi tiết + plugins, có %d: %q", len(got), buf.String())
	}
	if !strings.Contains(got[1], "9 souls") || !strings.Contains(got[2], "Wayfarer") {
		t.Errorf("chi tiết phải đứng ngay dưới hàng của nó: %q", buf.String())
	}
	if strings.TrimSpace(got[3]) != "plugins  identity memory" {
		t.Errorf("chi tiết trắng không được tạo dòng: %q", got[3])
	}
}

// Khối khởi động: một sự kiện một hình. Cột giá trị canh nhau để chú thích đứng thẳng.
func TestBannerAligns(t *testing.T) {
	var buf bytes.Buffer
	c := &Console{w: &buf, level: LevelInfo}
	b := Banner{Title: "W.O.N Proxy Inject", At: time.Date(2026, 8, 9, 16, 10, 1, 0, time.UTC)}
	b.Add("proxy", "http://127.0.0.1:8787", "← ANTHROPIC_BASE_URL")
	b.Add("cockpit", "http://127.0.0.1:7777")
	b.Add("bỏ qua", "")

	c.Print(b)
	got := buf.String()
	if !strings.HasPrefix(got, "W.O.N Proxy Inject · 09/08/2026 16:10:01\n") {
		t.Errorf("tiêu đề phải mang ngày đúng một lần: %q", got)
	}
	if strings.Contains(got, "bỏ qua") {
		t.Errorf("dòng giá trị rỗng phải bị bỏ: %s", got)
	}
	if !strings.Contains(got, "  proxy    http://127.0.0.1:8787   ← ANTHROPIC_BASE_URL\n") {
		t.Errorf("cột khoá chưa canh: %q", got)
	}
	if !strings.Contains(got, "  cockpit  http://127.0.0.1:7777\n") {
		t.Errorf("dòng không chú thích phải hết ở giá trị: %q", got)
	}
}

// Silent ghi vào io.Discard, không phải bỏ handler: `fatal` ở main đi thẳng ra stderr
// nên cái chết vẫn nhìn thấy được ở mọi mức.
func TestOpenSilent(t *testing.T) {
	old := slog.Default()
	defer slog.SetDefault(old)

	c := Open("silent")
	if c.Level() != LevelSilent {
		t.Fatalf("mức = %q", c.Level())
	}
	slog.Info("không được thấy dòng này")
}
