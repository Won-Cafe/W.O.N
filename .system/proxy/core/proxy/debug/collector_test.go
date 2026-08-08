// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package debug

import (
	"context"
	"hash/fnv"
	"strings"
	"testing"

	"won/proxy/core/request"
)

// Một lời người mở một lượt, đánh số từ 1. Chưa có lời người nào (mới chỉ vùng system và
// vòng máy) thì vẫn phải có một lượt để nhịp của lần chạy có nhà — lượt số 0.
func TestSplitTurns(t *testing.T) {
	turns := splitTurns([]string{"đo giúp tôi", "tiếp đi"})
	if len(turns) != 2 {
		t.Fatalf("muốn 2 lượt người, got %d: %+v", len(turns), turns)
	}
	if turns[0].Turn != 1 || turns[0].Asked != "đo giúp tôi" {
		t.Errorf("lượt 1 sai: %+v", turns[0])
	}
	if turns[1].Turn != 2 || turns[1].Asked != "tiếp đi" {
		t.Errorf("lượt 2 sai: %+v", turns[1])
	}
	if got := splitTurns(nil); len(got) != 1 || got[0].Turn != 0 || got[0].Asked != "" {
		t.Errorf("chưa có lời người thì vẫn có một lượt 0: %+v", got)
	}
}

// mark dựng một mắt chuỗi cho test: nhãn suy từ chữ, đúng như đường thật.
func mark(slot, text string) request.CacheMark {
	return request.CacheMark{Slot: slot, Label: tagOf(text), Hash: fnvOf(text), Bytes: len(text)}
}

func fnvOf(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64()
}

func tagOf(text string) string {
	if !strings.HasPrefix(text, "<") {
		return ""
	}
	if end := strings.IndexByte(text, '>'); end > 1 {
		return text[1:end]
	}
	return ""
}

// NỐI vào cuối thì mọi khối cũ dùng lại được, và không mất byte nào — đây là hình duy
// nhất giữ được tiền tố, và là cái phép đo phải cho điểm tuyệt đối.
func TestCompareMarksScoresAppendAsWholeReuse(t *testing.T) {
	prev := []request.CacheMark{mark("system", "<W.O.N>đất"), mark("user", "chào")}
	now := append(append([]request.CacheMark{}, prev...), mark("assistant", "ừ"))

	v := compareMarks(prev, now)
	if v.Kept != 2 || v.Of != 3 {
		t.Errorf("nối thêm một khối thì hai khối cũ phải còn nguyên: kept=%d of=%d", v.Kept, v.Of)
	}
	if v.LostBytes != 0 {
		t.Errorf("nối thì không bỏ đi byte nào, got %d", v.LostBytes)
	}
	if v.Broke != "assistant" || v.Was != "" {
		t.Errorf("chỗ gãy phải là khối MỚI, và lần trước không có gì đứng đó: broke=%q was=%q", v.Broke, v.Was)
	}
}

// Một khối system đổi ở giữa vùng system kéo theo TRỌN hội thoại: cache khớp theo tiền tố
// nên mọi thứ sau chỗ lệch đều mất.
func TestCompareMarksBlamesTheBlockThatBroke(t *testing.T) {
	tail := []request.CacheMark{mark("user", strings.Repeat("a", 500))}
	prev := append([]request.CacheMark{mark("system", "<W.O.N>đất"), mark("system", "<Memory>index cũ")}, tail...)
	now := append([]request.CacheMark{mark("system", "<W.O.N>đất"), mark("system", "<Memory>index mới")}, tail...)

	v := compareMarks(prev, now)
	if v.Kept != 1 {
		t.Errorf("phải gãy ngay ở khối thứ hai, kept=%d", v.Kept)
	}
	if v.Broke != "system Memory" || v.Was != "system Memory" {
		t.Errorf("phải gọi đúng tên khối gãy: broke=%q was=%q", v.Broke, v.Was)
	}
	// Đuôi giống hệt nhau vẫn KHÔNG được tính là dùng lại: nó nằm sau chỗ lệch.
	if v.SharedBytes != len("<W.O.N>đất") {
		t.Errorf("chỉ khối trước chỗ lệch mới được tính, got %d byte", v.SharedBytes)
	}
	if v.Reuse >= 50 {
		t.Errorf("một khối system đổi phải kéo tỉ lệ dùng lại xuống thấp, got %d%%", v.Reuse)
	}
}

// Khối chèn ở CUỐI rồi lượt sau biến mất (tiếng của lượt) — phần bỏ đi phải được khai ra
// bằng số, vì đó là byte upstream đã ghi vào cache mà không lần nào dùng lại được.
func TestCompareMarksCountsWhatWasDropped(t *testing.T) {
	base := []request.CacheMark{mark("user", "hỏi")}
	prev := append(append([]request.CacheMark{}, base...), mark("user", "<Loiterer>một tiếng bên lề"))
	now := append(append([]request.CacheMark{}, base...), mark("assistant", "đáp"))

	v := compareMarks(prev, now)
	if v.Kept != 1 {
		t.Errorf("tiền tố tới hết lượt người vẫn phải khớp, kept=%d", v.Kept)
	}
	if v.LostBytes != len("<Loiterer>một tiếng bên lề") {
		t.Errorf("byte bỏ đi phải đúng bằng khối đã biến mất, got %d", v.LostBytes)
	}
	if v.Was != "user Loiterer" {
		t.Errorf("phải gọi tên được khối đã biến mất, got %q", v.Was)
	}
}

// Lần chạy ĐẦU của phiên không có gì để so. Trả 0% ở đó là khai một phép đo chưa chạy
// thành một kết quả xấu.
func TestCompareMarksSaysNothingOnFirstRun(t *testing.T) {
	if v := compareMarks(nil, []request.CacheMark{mark("user", "chào")}); v != nil {
		t.Errorf("lần đầu của phiên thì không có phép đo nào: %+v", v)
	}
	if v := compareMarks([]request.CacheMark{mark("user", "chào")}, nil); v != nil {
		t.Errorf("không có chuỗi lần này thì không đo được: %+v", v)
	}
}

// Retry sau khi SỬA lời người là một lượt khác — bản ghi của bản nháp cũ không được
// hiện ra dưới lượt đang chảy. Đo trên một phiên thật: tám bản ghi dồn vào turn 2 dù
// lời người đã sửa mấy lần giữa các lần retry (input_tokens khác nhau từng lần).
func TestRecordsKeyedByUtteranceNotTurnIndex(t *testing.T) {
	l := &Log{}
	rec := func(ms int64) runRec { return runRec{Ms: ms} }

	l.remember("s", turnKey("bản nháp một"), rec(1))
	l.remember("s", turnKey("bản nháp một"), rec(2)) // retry cùng lời: cùng lượt
	l.remember("s", turnKey("bản đã sửa"), rec(3))   // sửa lời rồi gửi: lượt khác

	if got := l.recall("s", turnKey("bản nháp một")); len(got) != 2 {
		t.Errorf("hai lần chạy trên cùng một lời = 2 bản ghi, got %d", len(got))
	}
	if got := l.recall("s", turnKey("bản đã sửa")); len(got) != 1 || got[0].Ms != 3 {
		t.Errorf("lời đã sửa chỉ mang bản ghi của chính nó, got %+v", got)
	}
}

// Lượt chảy trọn thì không có `cut`. Khai một lượt bình thường là bị cắt thì trường ấy
// hoá vô nghĩa đúng ở chỗ nó được dựng ra để chỉ mặt một lượt bất thường.
func TestCutOnlyRecordsARealCut(t *testing.T) {
	c := &Collector{on: true}
	c.Cut(nil)
	if c.cut != "" {
		t.Errorf("thân chảy trọn mà vẫn khai bị cắt: %q", c.cut)
	}
	c.Cut(context.Canceled)
	if c.cut != context.Canceled.Error() {
		t.Errorf("cắt thật phải khai đúng lời của context, got %q", c.cut)
	}
	// Nhật ký tắt thì collector không giữ gì.
	off := &Collector{}
	off.Cut(context.Canceled)
	if off.cut != "" {
		t.Errorf("nhật ký tắt mà vẫn giữ chữ: %q", off.cut)
	}
}
