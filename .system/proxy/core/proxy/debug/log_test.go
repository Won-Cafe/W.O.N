// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package debug

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"won/proxy/core/request"
)

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Một file cho một phiên, ghi ĐÈ chứ không nối: mỗi lần ghi là vẽ lại trọn phiên
// từ request. Nhờ thế tắt proxy bật lại thì file vẫn đúng.
func TestWriteSessionOverwrites(t *testing.T) {
	dir := t.TempDir()
	l := New(dir)
	if l == nil {
		t.Fatal("không mở được thư mục")
	}
	l.writeSession("Tzu", "s-abc", "", map[string]any{"kind": "session", "turns": 1}, nil, nil)
	l.writeSession("Tzu", "s-abc", "", map[string]any{"kind": "session", "turns": 2}, nil, nil)

	path := filepath.Join(dir, "Tzu_s-abc", fileDetail)
	got := readFile(t, path)
	if strings.Count(got, `"kind"`) != 1 {
		t.Errorf("phải ghi đè, không nối:\n%s", got)
	}
	if !strings.Contains(got, `"turns": 2`) {
		t.Errorf("thiếu bản mới nhất:\n%s", got)
	}
	// Thụt lề sẵn: đây là tài liệu để mắt người đọc, không phải dòng chảy cho máy.
	if !strings.Contains(got, "\n  ") {
		t.Errorf("phải thụt lề:\n%s", got)
	}
}

// Khoá phiên do client gửi qua header, nên nó là chữ của người ngoài: không được
// thành đường thoát thư mục.
func TestSafeNameBlocksPathEscape(t *testing.T) {
	cases := map[string]string{
		"s-881404b9b011f709":     "s-881404b9b011f709",
		"../../etc/passwd":       "etc_passwd",
		`..\..\Windows\evil`:     "Windows_evil",
		"":                       "unnamed",
		"///":                    "unnamed",
		"ổn định":                "n___nh", // dấu tiếng Việt không phải chữ ASCII → thay hết
		strings.Repeat("a", 200): strings.Repeat("a", 64),
	}
	for in, want := range cases {
		if got := safeName(in); got != want {
			t.Errorf("safeName(%q) = %q, muốn %q", in, got, want)
		}
	}

	dir := t.TempDir()
	l := New(dir)
	l.writeSession("Tzu", "../../escaped", "", map[string]any{"kind": "session"}, nil, nil)
	// Mọi thứ ghi ra phải nằm THẲNG trong run/, không leo ra ngoài. Tên đệ + "_s-"
	// rửa đường thoát "../../escaped" thành "Tzu_s-escaped" — một thư mục gọn trong dir.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || strings.ContainsAny(entries[0].Name(), "./\\") {
		t.Errorf("tên lọt ra ngoài quy ước: %v", entries)
	}
}

// Khoá thăng cấp khi hội thoại có lượt trả lời đầu. Không dời file thì mỗi phiên
// để lại một file cụt đúng một lượt.
func TestRenameSessionOnKeyUpgrade(t *testing.T) {
	dir := t.TempDir()
	l := New(dir)
	l.writeSession("Tzu", "s-base", "", map[string]any{"kind": "session", "n": 1}, nil, nil)
	l.renameSession("Tzu", "s-base", "s-refined", "")

	if _, err := os.Stat(filepath.Join(dir, "Tzu_s-base")); err == nil {
		t.Error("thư mục khoá cũ phải được dời đi")
	}
	if got := readFile(t, filepath.Join(dir, "Tzu_s-refined", fileDetail)); !strings.Contains(got, `"n": 1`) {
		t.Errorf("nội dung phải theo sang khoá mới: %s", got)
	}
}

// Bản ghi không có phiên (lỗi trước khi nhận ra đệ) vào file rời,
// vòng giữ ngắn.
func TestWriteOtherKeepsRing(t *testing.T) {
	dir := t.TempDir()
	l := New(dir)
	for i := 0; i < maxOtherLines+3; i++ {
		l.writeOther(map[string]any{"kind": "passthrough", "n": i})
	}
	var lines []map[string]any
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(dir, "others.json"))), &lines); err != nil {
		t.Fatal(err)
	}
	if len(lines) != maxOtherLines {
		t.Fatalf("vòng giữ %d, got %d", maxOtherLines, len(lines))
	}
	if lines[len(lines)-1]["n"].(float64) != float64(maxOtherLines+2) {
		t.Errorf("bản ghi mới nhất phải ở cuối: %v", lines[len(lines)-1])
	}
}

// Nhịp từng lần chạy gắn vào lượt người mà nó nằm trong: đệ chạy chục vòng tool
// trước khi người nói lại, và cả chục vòng đó thuộc cùng một lượt.
func TestRememberGroupsRunsUnderTheirTurn(t *testing.T) {
	l := &Log{}
	l.remember("s1", 0, runRec{Ms: 10})
	l.remember("s1", 0, runRec{Ms: 20})
	l.remember("s1", 1, runRec{Ms: 30})

	if got := l.recall("s1", 0); len(got) != 2 || got[1].Ms != 20 {
		t.Errorf("lượt 0 phải có 2 lần chạy: %+v", got)
	}
	if got := l.recall("s1", 1); len(got) != 1 {
		t.Errorf("lượt 1 phải có 1 lần chạy: %+v", got)
	}
	if got := l.recall("s2", 0); got != nil {
		t.Errorf("phiên khác không dính: %+v", got)
	}

	// Khoá thăng cấp: nhịp đã gom phải theo sang khoá mới.
	l.migrateRecs("s1", "s1-refined")
	if got := l.recall("s1-refined", 0); len(got) != 2 {
		t.Errorf("nhịp phải theo sang khoá mới: %+v", got)
	}
	if got := l.recall("s1", 0); got != nil {
		t.Errorf("khoá cũ phải sạch: %+v", got)
	}
}

// Phép đo tiền tố chỉ so được với lần chạy trước của CÙNG phiên, và tiền lệ phải theo
// khoá khi phiên thăng cấp — không thì lượt kế báo gãy oan.
func TestCompareRunStaysWithinSession(t *testing.T) {
	l := &Log{}
	one := []request.CacheMark{mark("user", "chào")}
	two := []request.CacheMark{mark("user", "chào"), mark("assistant", "ừ")}

	if v := l.compareRun("s1", one); v != nil {
		t.Errorf("lần chạy đầu của phiên không có tiền lệ để so: %+v", v)
	}
	if v := l.compareRun("s1", two); v == nil || v.Kept != 1 {
		t.Errorf("lần hai phải so được với lần một: %+v", v)
	}
	if v := l.compareRun("s2", two); v != nil {
		t.Errorf("phiên khác không được dùng tiền lệ của phiên này: %+v", v)
	}
	l.migrateRecs("s1", "s1-refined")
	if v := l.compareRun("s1-refined", two); v == nil || v.Kept != 2 {
		t.Errorf("tiền lệ chưa theo sang khoá mới: %+v", v)
	}
}

// Không escape HTML: `<W.O.N>` phải đọc ra được bằng mắt.
func TestEncodeKeepsAngleBrackets(t *testing.T) {
	dir := t.TempDir()
	l := New(dir)
	l.writeSession("Tzu", "s", "", map[string]any{"injected": "<W.O.N>đất</W.O.N>"}, nil, nil)
	got := readFile(t, filepath.Join(dir, "Tzu_s-s", fileDetail))
	if strings.Contains(got, "\\u003c") {
		t.Errorf("dấu ngoặc bị escape: %s", got)
	}
	if !strings.Contains(got, "<W.O.N>") {
		t.Errorf("thiếu khối đọc được: %s", got)
	}
}

// Khoá phiên dẫn xuất từ đệ + lời người đầu, nên hỏi lại ĐÚNG câu cũ ở một hội thoại
// MỚI ra đúng khoá cũ — và file cũ bị ghi đè, mất trọn phiên trước. Mốc mở phiên tách
// hai hội thoại ấy ra. Đã xảy ra: một phiên 8 lượt bị một phiên 2 lượt đè lên.
func TestSameKeyDifferentSessionsDoNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	l := New(dir)

	l.writeSession("Tzu", "s-abc", "20260728-001200", map[string]any{"n": "phiên trước"}, nil, nil)
	l.writeSession("Tzu", "s-abc", "20260728-003755", map[string]any{"n": "phiên sau"}, nil, nil)

	dirs, err := filepath.Glob(filepath.Join(dir, "*_Tzu_s-abc"))
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 2 {
		t.Fatalf("hai hội thoại = hai thư mục, got %d: %v", len(dirs), dirs)
	}
	// Cùng hội thoại (cùng mốc mở) thì vẫn ghi đè — một thư mục cho một phiên.
	l.writeSession("Tzu", "s-abc", "20260728-003755", map[string]any{"n": "phiên sau, lượt hai"}, nil, nil)
	if dirs, _ := filepath.Glob(filepath.Join(dir, "*_Tzu_s-abc")); len(dirs) != 2 {
		t.Errorf("cùng phiên phải ghi đè chính nó, got %d thư mục", len(dirs))
	}
}

// Phiên là một THƯ MỤC, ba file bên trong trả lời ba câu khác nhau: hệ làm gì (detail),
// chủ gửi gì (input), ta gửi gì đi (output). Trước đây chỉ có câu đầu, nên mỗi lần nghi
// lõi chèn sai lại phải dựng lại tay bằng curl.
func TestSessionIsAFolderWithThreeFiles(t *testing.T) {
	dir := t.TempDir()
	l := New(dir)

	in := []byte(`{"system":"LỜI CHỦ","messages":[{"role":"user","content":"chào"}]}`)
	out := []byte(`{"system":[{"type":"text","text":"<W.O.N>đất</W.O.N>"}],"messages":[{"role":"user","content":"chào"}]}`)
	l.writeSession("Tzu", "s-abc", "20260728-120000", map[string]any{"kind": "session"}, in, out)

	base := filepath.Join(dir, "20260728-120000_Tzu_s-abc")
	for _, name := range []string{fileDetail, fileInput, fileOutput} {
		if _, err := os.Stat(filepath.Join(base, name)); err != nil {
			t.Errorf("thiếu %s: %v", name, err)
		}
	}
	// input giữ NGUYÊN lời chủ; output phải mang khối lõi đã chèn.
	gotIn, _ := os.ReadFile(filepath.Join(base, fileInput))
	if !strings.Contains(string(gotIn), "LỜI CHỦ") || strings.Contains(string(gotIn), "<W.O.N>") {
		t.Errorf("input phải là bản chủ gửi, chưa chèn gì:\n%s", gotIn)
	}
	gotOut, _ := os.ReadFile(filepath.Join(base, fileOutput))
	if !strings.Contains(string(gotOut), "<W.O.N>") {
		t.Errorf("output phải mang khối lõi:\n%s", gotOut)
	}
	// Thân JSON được thụt lề — file này để người đọc, không để máy đọc lại.
	if !strings.Contains(string(gotIn), "\n  ") {
		t.Errorf("input chưa thụt lề:\n%s", gotIn)
	}
}

// Lượt không có thân thì file thân bị XOÁ, không để bản của lượt cũ nằm lại: hai file thân
// luôn thuộc lần chạy CUỐI của detail, và một bản cũ đứng lại là nói dối về lượt đang đọc.
func TestSessionFolderDropsStaleBodies(t *testing.T) {
	dir := t.TempDir()
	l := New(dir)
	base := filepath.Join(dir, "Tzu_s-abc")

	l.writeSession("Tzu", "s-abc", "", map[string]any{"n": 1}, []byte(`{"a":1}`), []byte(`{"b":1}`))
	for _, name := range []string{fileInput, fileOutput} {
		if _, err := os.Stat(filepath.Join(base, name)); err != nil {
			t.Fatalf("lượt có thân mà thiếu %s: %v", name, err)
		}
	}

	l.writeSession("Tzu", "s-abc", "", map[string]any{"n": 2}, nil, nil)
	if _, err := os.Stat(filepath.Join(base, fileDetail)); err != nil {
		t.Errorf("detail vẫn phải có: %v", err)
	}
	for _, name := range []string{fileInput, fileOutput} {
		if _, err := os.Stat(filepath.Join(base, name)); err == nil {
			t.Errorf("%s của lượt cũ phải bị dọn đi", name)
		}
	}
}

// Không còn thư mục riêng cho từng lượt: debug_detail.json đã vẽ lại TRỌN phiên ở
// mỗi lần ghi (turns[] mang cả lượt cũ lẫn lượt mới), nên bản của lượt 2 đã chứa đủ
// thông tin của lượt 1 — giữ thêm một bản riêng cho lượt 1 chỉ nhân đôi cùng một
// lịch sử. raw input/out cũng ghi đè theo cùng lý do: mỗi request tự mang trọn lịch
// sử hội thoại, nên bản của lượt sau là bản đủ nhất còn lại.
func TestWriteSessionOverwritesAcrossTurns(t *testing.T) {
	dir := t.TempDir()
	l := New(dir)

	l.writeSession("Tzu", "s-abc", "20260728-120000", map[string]any{"n": 1}, []byte(`{"a":1}`), []byte(`{"b":1}`))
	l.writeSession("Tzu", "s-abc", "20260728-120000", map[string]any{"n": 2}, []byte(`{"a":2}`), []byte(`{"b":2}`))

	base := filepath.Join(dir, "20260728-120000_Tzu_s-abc")
	if got := readFile(t, filepath.Join(base, fileDetail)); !strings.Contains(got, `"n": 2`) || strings.Contains(got, `"n": 1`) {
		t.Errorf("phải chỉ còn bản mới nhất:\n%s", got)
	}
	if got := readFile(t, filepath.Join(base, fileInput)); !strings.Contains(got, `"a": 2`) {
		t.Errorf("input phải bị lượt sau đè:\n%s", got)
	}
	if entries, _ := os.ReadDir(base); len(entries) != 3 {
		t.Errorf("chỉ ba file phẳng, không thư mục lượt con: %v", entries)
	}
}
