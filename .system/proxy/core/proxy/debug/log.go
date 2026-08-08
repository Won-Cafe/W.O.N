// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

// Package debug là nhật ký chẩn bệnh của proxy: một thư mục cho một phiên.
//
//	<mốc mở>_<đệ>_s-<phiên>/  debug_detail.json · debug_input.json · debug_output.json
//
// Ba file trả ba câu: "hệ làm gì" · "chủ gửi gì" · "ta gửi gì đi". Ghi đè mỗi lượt,
// không cắt theo lượt ra thư mục riêng: một request mang trọn lịch sử hội thoại
// (debug_detail.json vẽ lại TRỌN phiên từ nó, xem collector.go), nên ba file của
// lượt mới nhất đã là bản đủ nhất — giữ thêm bản của các lượt trước chỉ nhân bản
// càng lúc càng to của cùng một lịch sử đang lớn dần.
//
// Hai file thân luôn thuộc lần chạy CUỐI trong debug_detail.json, và bất biến ấy được
// giữ bằng cách XOÁ: lượt không có thân thì file bị dọn đi chứ không để bản của lượt cũ
// đứng lại đội lốt lượt mới.
//
// Chỉ sống ở log_level=debug (#5); `New("")` hoặc dir không mở được → nil, mọi việc
// gọi qua *Log nil-safe.
package debug

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"won/proxy/core/request"
)

// Log — hai phần khoá riêng: ghi file (mu, dir, other), và nhịp từng lần chạy để gắn
// lại vào đúng lượt khi vẽ lại phiên (recMu, recs).
type Log struct {
	mu    sync.Mutex
	dir   string
	other []json.RawMessage

	recMu     sync.Mutex
	recs      map[string]map[uint64][]runRec // khoá phiên → vân tay lời người → nhịp từng lần chạy
	lastMarks map[string][]request.CacheMark // khoá phiên → chuỗi khối của lần chạy trước
}

// maxOtherLines — bản ghi rời (không phiên) chỉ cần thấy cái gần nhất.
const maxOtherLines = 20

// maxRecSessions — trần phiên giữ nhịp trong RAM. Nhật ký chẩn bệnh, không phải
// kho: quá trần thì xoá sạch rồi gom lại từ đầu.
const maxRecSessions = 64

// Tên ba file trong thư mục phiên.
const (
	fileDetail = "debug_detail.json"
	fileInput  = "debug_input.json"
	fileOutput = "debug_output.json"
)

// New nhận thư mục. Không tạo được → nil, debug tắt im lặng.
func New(dir string) *Log {
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil
	}
	return &Log{dir: dir}
}

// writeSession ghi trọn thư mục phiên, ghi đè không nối. in/out là thân THẬT — cắt ở đây,
// tức sau khi nó đã đi ra upstream (§ cut.go).
func (l *Log) writeSession(agent, session, opened string, doc any, in, out []byte) {
	if l == nil || session == "" {
		return
	}
	b, ok := encode(doc)
	if !ok {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	dir := l.path(agent, session, opened)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	os.WriteFile(filepath.Join(dir, fileDetail), b, 0o644)
	writeBody(filepath.Join(dir, fileInput), in)
	writeBody(filepath.Join(dir, fileOutput), out)
}

// renameSession dời thư mục khi khoá phiên thăng cấp.
func (l *Log) renameSession(agent, from, to, opened string) {
	if l == nil || from == "" || to == "" || from == to {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	os.Rename(l.path(agent, from, opened), l.path(agent, to, opened))
}

// writeOther thêm một bản ghi rời vào others.json ở gốc run/.
func (l *Log) writeOther(entry any) {
	if l == nil {
		return
	}
	b, ok := encode(entry)
	if !ok {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.other = append(l.other, json.RawMessage(b))
	if len(l.other) > maxOtherLines {
		l.other = l.other[len(l.other)-maxOtherLines:]
	}
	if out, ok := encode(l.other); ok {
		os.WriteFile(filepath.Join(l.dir, "others.json"), out, 0o644)
	}
}

// path — thư mục phiên mang cả MỐC MỞ PHIÊN, không chỉ khoá phiên. Khoá dẫn xuất từ đệ
// + lời người đầu, nên hỏi lại đúng câu cũ ở một hội thoại MỚI ra đúng khoá cũ — và
// phiên cũ bị ghi đè, mất trọn. Mốc mở phiên tách hai hội thoại ấy ra.
func (l *Log) path(agent, session, opened string) string {
	name := safeName(agent) + "_s-" + strings.TrimPrefix(safeName(session), "s-")
	if opened != "" {
		name = safeName(opened) + "_" + name
	}
	return filepath.Join(l.dir, name)
}

// remember giữ nhịp một lần chạy, gắn vào lượt người. Chỉ trong RAM vì nhịp không nằm
// trong request: khởi động lại thì lượt cũ còn hội thoại nhưng mất nhịp.
func (l *Log) remember(session string, turn uint64, rec runRec) {
	l.recMu.Lock()
	defer l.recMu.Unlock()
	// Lần đầu chỉ DỰNG sổ, không dọn: compareRun chạy trước remember trong cùng một lượt
	// ghi, nên dọn ở đây là xoá đúng cái vừa đo được — và hai lần chạy đầu của mỗi tiến
	// trình mất phép đo. Quá trần thì mới dọn, và lúc ấy dọn cả hai sổ.
	if l.recs == nil {
		l.recs = map[string]map[uint64][]runRec{}
	}
	if len(l.recs) > maxRecSessions {
		l.recs = map[string]map[uint64][]runRec{}
		l.lastMarks = map[string][]request.CacheMark{}
	}
	if l.recs[session] == nil {
		l.recs[session] = map[uint64][]runRec{}
	}
	l.recs[session][turn] = append(l.recs[session][turn], rec)
}

func (l *Log) recall(session string, turn uint64) []runRec {
	l.recMu.Lock()
	defer l.recMu.Unlock()
	return l.recs[session][turn]
}

// compareRun đo lần chạy này với lần chạy TRƯỚC của cùng phiên, rồi ghi đè sổ. Nhịp không
// nằm trong request nên sổ chỉ sống trong RAM: khởi động lại thì lượt đầu không có gì để so.
func (l *Log) compareRun(session string, marks []request.CacheMark) *cacheView {
	l.recMu.Lock()
	defer l.recMu.Unlock()
	if l.lastMarks == nil {
		l.lastMarks = map[string][]request.CacheMark{}
	}
	v := compareMarks(l.lastMarks[session], marks)
	if len(marks) > 0 {
		l.lastMarks[session] = marks
	}
	return v
}

// migrateRecs mang nhịp đã gom sang khoá mới khi phiên thăng cấp khoá.
func (l *Log) migrateRecs(from, to string) {
	l.recMu.Lock()
	defer l.recMu.Unlock()
	if old, ok := l.recs[from]; ok {
		if l.recs[to] == nil {
			l.recs[to] = old
		}
		delete(l.recs, from)
	}
	if m, ok := l.lastMarks[from]; ok {
		if _, exists := l.lastMarks[to]; !exists {
			l.lastMarks[to] = m
		}
		delete(l.lastMarks, from)
	}
}

// writeBody cắt rồi thụt lề nếu thân là JSON đọc được, không thì ghi nguyên byte: thân là
// chữ người ngoài gửi, không đoán nó đúng hình (#6). Lượt không có thân thì XOÁ file — bản
// của lượt cũ nằm lại là nói dối về lượt đang đọc.
func writeBody(path string, raw []byte) {
	if len(raw) == 0 {
		os.Remove(path)
		return
	}
	cut := cutBody(raw)
	var buf bytes.Buffer
	if json.Indent(&buf, cut, "", "  ") == nil {
		os.WriteFile(path, buf.Bytes(), 0o644)
		return
	}
	os.WriteFile(path, cut, 0o644)
}

// safeName rửa khoá phiên thành tên thư mục: chỉ nhận chữ-số-gạch, cắt 64 ký tự, rỗng
// → "unnamed". Khoá đến từ header của client nên `../../etc/passwd` không đi đâu được.
func safeName(s string) string {
	var sb strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			sb.WriteRune(r)
		default:
			sb.WriteRune('_')
		}
		if sb.Len() >= 64 {
			break
		}
	}
	out := strings.Trim(sb.String(), "_")
	if out == "" {
		return "unnamed"
	}
	return out
}

// encode — JSON thụt lề, không escape HTML. Tài liệu cho người đọc: `<W.O.N>`
// phải ra đúng dấu ngoặc.
func encode(entry any) ([]byte, bool) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(entry); err != nil {
		return nil, false
	}
	return buf.Bytes(), true
}
