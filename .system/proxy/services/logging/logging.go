// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

// Package logging dựng console cho người đọc: một handler slog gọn và khối khởi động.
// Ở services vì console câm thì dòng chính vẫn chảy, và core không import nó — core gọi
// `slog` của stdlib, main đặt handler này làm mặc định (§ Phân lớp · § Quan sát được).
package logging

import (
	"context"
	"io"
	"log/slog"
	"net"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Ba mức log. Tên lạ rơi về info: một chữ gõ sai không đáng làm hệ không lên.
const (
	LevelInfo   = "info"
	LevelDebug  = "debug"
	LevelSilent = "silent"
)

// Hình một dòng log: giờ (không ngày — ngày đứng một lần ở khối khởi động) · dấu mức ·
// msg · attr. `indent` canh dòng nối đúng dưới chữ đầu của msg: 8 + 1 + 1 + 1.
const (
	indent      = "           "
	clockFormat = "15:04:05"
)

// tookKey — khoá chở quãng thời gian tính bằng mili-giây; `render` dựng nó thành thời
// lượng, nên tên khoá không mang đơn vị.
const tookKey = "took"

// maxNoteCol — trần cột giá trị khi canh chú thích trong khối khởi động. Giá trị vượt
// trần vẫn in trọn, chỉ thôi kéo cột của hàng khác.
const maxNoteCol = 34

const bannerDate = "02/01/2006 15:04:05"

// Normalize quy một tên mức về ba mức có thật.
func Normalize(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case LevelDebug:
		return LevelDebug
	case LevelSilent:
		return LevelSilent
	default:
		return LevelInfo
	}
}

// Console — mặt chữ của tiến trình. Giữ chính writer của handler để khối khởi động và
// các dòng log không rơi vào hai chỗ khác nhau ở mức silent.
type Console struct {
	w     io.Writer
	level string
}

// Open dựng handler theo mức và đặt làm slog mặc định. Silent ghi vào io.Discard chứ
// không bỏ handler: `fatal` đi thẳng ra stderr nên cái chết vẫn thấy được ở mọi mức.
//
// newBlock là các msg mở một khối mới — handler chèn một dòng trắng trước chúng. Danh
// sách do main khai vì handler không biết nghĩa của bản ghi nào (§ Quan sát được).
func Open(level string, newBlock ...string) *Console {
	lv := Normalize(level)
	var w io.Writer = os.Stdout
	if lv == LevelSilent {
		w = io.Discard
	}
	min := slog.LevelInfo
	if lv == LevelDebug {
		min = slog.LevelDebug
	}
	opens := make(map[string]bool, len(newBlock))
	for _, m := range newBlock {
		if m != "" {
			opens[m] = true
		}
	}
	slog.SetDefault(slog.New(&handler{w: w, mu: &sync.Mutex{}, min: min, opens: opens}))
	return &Console{w: w, level: lv}
}

// Level — mức đang chạy, đã quy về ba mức có thật.
func (c *Console) Level() string { return c.level }

// handler — text handler cho console: một dòng một bản ghi, trừ giá trị dài (§ appendAttr).
type handler struct {
	w      io.Writer
	mu     *sync.Mutex
	min    slog.Level
	opens  map[string]bool // msg mở khối mới — chèn dòng trắng trước
	attrs  []slog.Attr
	groups []string
}

func (h *handler) Enabled(_ context.Context, l slog.Level) bool { return l >= h.min }

func (h *handler) WithAttrs(as []slog.Attr) slog.Handler {
	if len(as) == 0 {
		return h
	}
	n := *h
	n.attrs = append(slices.Clip(h.attrs), as...)
	return &n
}

func (h *handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	n := *h
	n.groups = append(slices.Clip(h.groups), name)
	return &n
}

// Handle dựng trọn dòng vào buffer rồi ghi MỘT lần dưới khoá: hai lời gọi Write cho một
// bản ghi là hai dòng đan vào nhau khi nhiều goroutine cùng nói (`plugin.Gather`).
//
// Dòng trắng đứng TRƯỚC bản ghi mở khối: một lần chạy hỏng giữa chừng không có bản ghi
// khép nào để bám vào.
func (h *handler) Handle(_ context.Context, r slog.Record) error {
	var line []byte
	if h.opens[r.Message] {
		line = append(line, '\n')
	}
	if !r.Time.IsZero() {
		line = r.Time.AppendFormat(line, clockFormat)
	} else {
		line = append(line, strings.Repeat(" ", len(clockFormat))...)
	}
	line = append(line, ' ')
	line = append(line, mark(r.Level)...)
	line = append(line, ' ')
	line = append(line, r.Message...)

	// Dòng nối phải đi sau MỌI giá trị gọn của cùng bản ghi, nên hai loại gom ra hai chỗ.
	var wrapped []byte
	emit := func(a slog.Attr) {
		line, wrapped = appendAttr(line, wrapped, h.groups, a)
	}
	for _, a := range h.attrs {
		emit(a)
	}
	r.Attrs(func(a slog.Attr) bool { emit(a); return true })

	line = append(line, '\n')
	line = append(line, wrapped...)

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := h.w.Write(line)
	return err
}

// mark — mức hiện bằng một dấu, và nhánh thường không có dấu: một trường chỉ có nghĩa
// khi nó khác thường.
func mark(l slog.Level) string {
	switch {
	case l >= slog.LevelError:
		return "✗"
	case l >= slog.LevelWarn:
		return "!"
	case l >= slog.LevelInfo:
		return " "
	default:
		return "·"
	}
}

// appendAttr xếp một attr vào một trong hai chỗ: giá trị gọn vào dòng chính, giá trị có
// khoảng trắng xuống dòng nối. Cắt như thế thì không cần ngoặc kép ở đâu cả, và chỉ dòng
// nối ĐẦU mang `khoá=`. Giá trị rỗng bị bỏ hẳn — một trường không mang gì là dòng thừa.
func appendAttr(line, wrapped []byte, groups []string, a slog.Attr) ([]byte, []byte) {
	a.Value = a.Value.Resolve()
	if a.Equal(slog.Attr{}) {
		return line, wrapped
	}
	if a.Value.Kind() == slog.KindGroup {
		inner := a.Value.Group()
		if a.Key != "" {
			groups = append(slices.Clip(groups), a.Key)
		}
		for _, g := range inner {
			line, wrapped = appendAttr(line, wrapped, groups, g)
		}
		return line, wrapped
	}

	key := a.Key
	if len(groups) > 0 {
		key = strings.Join(groups, ".") + "." + key
	}
	val := render(key, a.Value)
	if val == "" {
		return line, wrapped
	}

	if strings.ContainsAny(val, " \t\n") {
		for i, seg := range strings.Split(val, "\n") {
			wrapped = append(wrapped, indent...)
			if i == 0 {
				wrapped = append(wrapped, key...)
				wrapped = append(wrapped, '=')
			} else {
				wrapped = append(wrapped, strings.Repeat(" ", len(key)+1)...)
			}
			wrapped = append(wrapped, strings.TrimRight(seg, "\r")...)
			wrapped = append(wrapped, '\n')
		}
		return line, wrapped
	}
	line = append(line, ' ')
	line = append(line, key...)
	line = append(line, '=')
	return append(line, val...), wrapped
}

// render đọc giá trị ra chữ, rồi áp hai luật theo TÊN KHOÁ: khoá địa chỉ được thêm scheme
// cho terminal bấm được, khoá `took` dựng thành thời lượng. Lỗi đi qua ShortenErr.
func render(key string, v slog.Value) string {
	s := v.String()
	if v.Kind() == slog.KindAny {
		if err, ok := v.Any().(error); ok && err != nil {
			s = ShortenErr(err.Error())
		}
	}
	if webKeys[key] {
		return WebAddr(s)
	}
	if key == tookKey && v.Kind() == slog.KindInt64 {
		return Duration(v.Int64())
	}
	return s
}

// webKeys — danh sách ĐÓNG các khoá mang địa chỉ. Đóng vì luật này chạy theo tên khoá:
// mở ra thì một khoá mới trùng tên bị sửa giá trị mà không ai khai gì.
var webKeys = map[string]bool{
	"url": true, "addr": true, "listen": true, "target": true,
	"control": true, "base_url": true, "upstream": true,
}

// WebAddr thêm `http://` cho thứ đọc ra được là host:port và chưa có scheme; chuỗi khác
// trả nguyên văn, vì đoán sai một địa chỉ tệ hơn để nó không bấm được.
func WebAddr(s string) string {
	t := strings.TrimSpace(s)
	if t == "" || strings.Contains(t, "://") {
		return s
	}
	if _, _, err := net.SplitHostPort(t); err != nil {
		return s
	}
	return "http://" + t
}

// Duration đọc mili-giây ra quãng ước lượng được ngay, không phải nhẩm.
func Duration(ms int64) string {
	switch {
	case ms < 1000:
		return strconv.FormatInt(ms, 10) + "ms"
	case ms < 60_000:
		return strconv.FormatFloat(float64(ms)/1000, 'f', 1, 64) + "s"
	case ms < 3_600_000:
		return strconv.FormatInt(ms/60_000, 10) + "m" + pad2(ms%60_000/1000) + "s"
	default:
		return strconv.FormatInt(ms/3_600_000, 10) + "h" + pad2(ms%3_600_000/60_000) + "m"
	}
}

func pad2(n int64) string {
	if n < 10 {
		return "0" + strconv.FormatInt(n, 10)
	}
	return strconv.FormatInt(n, 10)
}

// osNoise — lời dài của hệ điều hành và bản ngắn cùng nghĩa. Chỉ đổi chữ, không đổi nghĩa.
var osNoise = []struct{ long, short string }{
	{"No such host is known.", "host not found"},
	{"A socket operation was attempted to an unreachable network.", "network unreachable"},
	{"No connection could be made because the target machine actively refused it.", "connection refused"},
	{"Only one usage of each socket address (protocol/network address/port) is normally permitted.", "address already in use"},
	{"A connection attempt failed because the connected party did not properly respond after a period of time, or established connection failed because connected host has failed to respond.", "timed out"},
}

// syscallNoise — tên lời gọi hệ thống: nó nói API nào hỏng, không nói cái gì hỏng.
var syscallNoise = []string{"wsarecv: ", "wsasend: ", "connectex: ", "wsarecvfrom: "}

// ShortenErr rút lỗi thô của Go/OS về phần hành động được: dịch lời dài của OS, bỏ tên
// syscall, rồi cắt phần bọc `Post "<url>": ` khi bên trong đã là lỗi dial — đích ở đó
// lặp lại attr khác của cùng bản ghi. Cái mất là path của URL bọc ngoài.
func ShortenErr(s string) string {
	for _, n := range osNoise {
		s = strings.ReplaceAll(s, n.long, n.short)
	}
	for _, w := range syscallNoise {
		s = strings.ReplaceAll(s, w, "")
	}
	// Không có khoảng trắng sau `tcp`: khớp cả `dial tcp 1.2.3.4:80` lẫn `dial tcp: lookup x`.
	if i := strings.Index(s, "dial tcp"); i > 0 {
		s = s[i:]
	}
	return strings.TrimSpace(s)
}

// Row — một hàng của khối khởi động. Note đứng sau giá trị và nói giá trị ấy dùng để làm
// gì; Detail là chi tiết của chính hàng đó, in xuống dòng nối.
type Row struct {
	Key    string
	Note   string
	Value  string
	Detail string
}

// Banner — khối khởi động: một sự kiện, một hình. Ngày đứng ở đây, đúng một lần.
type Banner struct {
	At    time.Time
	Rows  []Row
	Title string
}

// Add xếp thêm một hàng; giá trị rỗng thì bỏ qua, để chỗ gọi không phải viết if quanh
// từng hàng.
func (b *Banner) Add(key, value string, note ...string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	r := Row{Key: key, Value: value}
	if len(note) > 0 {
		r.Note = note[0]
	}
	b.Rows = append(b.Rows, r)
}

// Detail gắn chi tiết vào hàng vừa thêm. Chưa có hàng nào, hoặc chữ rỗng → không làm gì.
func (b *Banner) Detail(detail string) {
	if strings.TrimSpace(detail) == "" || len(b.Rows) == 0 {
		return
	}
	b.Rows[len(b.Rows)-1].Detail = detail
}

// Print in khối khởi động. Cột chú thích canh theo giá trị dài nhất nhưng không quá
// maxNoteCol, để một giá trị dài bất thường không kéo mọi chú thích khác dạt sang phải.
// Silent → writer là io.Discard, nên không cần nhánh riêng.
func (c *Console) Print(b Banner) {
	var sb strings.Builder
	sb.WriteString(b.Title)
	if !b.At.IsZero() {
		sb.WriteString(" · " + b.At.Format(bannerDate))
	}
	sb.WriteByte('\n')

	width := 0
	for _, r := range b.Rows {
		if n := utf8.RuneCountInString(r.Key); n > width {
			width = n
		}
	}
	valWidth := 0
	for _, r := range b.Rows {
		if r.Note == "" {
			continue
		}
		if n := utf8.RuneCountInString(r.Value); n > valWidth && n <= maxNoteCol {
			valWidth = n
		}
	}

	for _, r := range b.Rows {
		sb.WriteString("  " + r.Key + strings.Repeat(" ", width-utf8.RuneCountInString(r.Key)) + "  ")
		if r.Note == "" {
			sb.WriteString(r.Value + "\n")
		} else {
			pad := valWidth - utf8.RuneCountInString(r.Value)
			if pad < 1 {
				pad = 1
			}
			sb.WriteString(r.Value + strings.Repeat(" ", pad) + "  " + r.Note + "\n")
		}
		if r.Detail != "" {
			sb.WriteString(strings.Repeat(" ", width+4) + r.Detail + "\n")
		}
	}
	io.WriteString(c.w, sb.String())
}
