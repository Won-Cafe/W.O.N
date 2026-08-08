// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package request

import (
	"strings"
	"testing"
)

func marksOf(t *testing.T, raw string, format Format) []CacheMark {
	t.Helper()
	b, err := ParseBody([]byte(raw), format)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return b.CacheMarks()
}

// Chuỗi phải ra đúng thứ tự upstream đọc: vùng lời hệ thống trước, rồi hội thoại — và mỗi
// khối một mắt, vì cache gãy ở mức khối chứ không ở mức "cả vùng system".
func TestCacheMarksFollowWireOrder(t *testing.T) {
	marks := marksOf(t, `{"system":[{"type":"text","text":"<W.O.N>đất"},{"type":"text","text":"<Memory>index"}],
		"messages":[{"role":"user","content":"chào"},{"role":"assistant","content":"ừ"}]}`, FormatAnthropic)

	want := []string{"system", "system", "user", "assistant"}
	if len(marks) != len(want) {
		t.Fatalf("muốn %d mắt, got %d: %+v", len(want), len(marks), marks)
	}
	for i, w := range want {
		if marks[i].Slot != w {
			t.Errorf("mắt %d muốn slot %q, got %q", i, w, marks[i].Slot)
		}
	}
	if marks[0].Label != "W.O.N" || marks[1].Label != "Memory" {
		t.Errorf("khối tự bọc tên thì nhãn phải đọc ra được: %q, %q", marks[0].Label, marks[1].Label)
	}
	if marks[2].Label != "" {
		t.Errorf("lời người không phải khối của lõi nên không có nhãn, got %q", marks[2].Label)
	}
}

// OpenAI chở lời hệ thống trong chính mảng messages. Đọc cả hai đường là đếm nó hai lần,
// và một khối đếm hai lần thì mọi con số byte đều sai.
func TestCacheMarksCountOpenAISystemOnce(t *testing.T) {
	marks := marksOf(t, `{"messages":[{"role":"system","content":"<W.O.N>đất"},{"role":"user","content":"chào"}]}`, FormatOpenAI)
	if len(marks) != 2 {
		t.Fatalf("muốn 2 mắt, got %d: %+v", len(marks), marks)
	}
	if marks[0].Slot != "system" || marks[0].Label != "W.O.N" {
		t.Errorf("khối system của OpenAI vẫn phải gọi tên được: %+v", marks[0])
	}
}

// Vân tay lấy trên BYTE THẬT của phần tử, nên đổi ở đâu trong message cũng bắt được —
// kể cả ngoài `content`, chỗ cache của nhà cung cấp vẫn tính vào chuỗi.
func TestCacheMarksHashWholeElement(t *testing.T) {
	a := marksOf(t, `{"messages":[{"role":"assistant","content":"ừ","tool_calls":[{"id":"a"}]}]}`, FormatOpenAI)
	b := marksOf(t, `{"messages":[{"role":"assistant","content":"ừ","tool_calls":[{"id":"b"}]}]}`, FormatOpenAI)
	if a[0].Hash == b[0].Hash {
		t.Error("đổi ngoài content mà vân tay không đổi — chuỗi wire đã đổi rồi")
	}

	same := marksOf(t, `{"messages":[{"role":"assistant","content":"ừ","tool_calls":[{"id":"a"}]}]}`, FormatOpenAI)
	if a[0].Hash != same[0].Hash {
		t.Error("cùng byte mà vân tay khác — phép đo không tất định")
	}
}

// Gemini gọi hội thoại là `contents` và vai model là `model`. Chuỗi phải quy về tên chung
// của lõi, không thì so hai lần chạy của cùng phiên lại lệch ở slot.
func TestCacheMarksReadGeminiShape(t *testing.T) {
	marks := marksOf(t, `{"systemInstruction":{"parts":[{"text":"<Soul>Tzu"}]},
		"contents":[{"role":"user","parts":[{"text":"chào"}]},{"role":"model","parts":[{"text":"ừ"}]}]}`, FormatGemini)

	if len(marks) != 3 {
		t.Fatalf("muốn 3 mắt, got %d: %+v", len(marks), marks)
	}
	if marks[0].Slot != "system" || marks[0].Label != "Soul" {
		t.Errorf("part của systemInstruction là một khối: %+v", marks[0])
	}
	if marks[2].Slot != RoleAssistant {
		t.Errorf("vai `model` phải quy về tên chung của lõi, got %q", marks[2].Slot)
	}
}

// Nhãn chỉ nhận tên TRẦN. Lời người mở bằng dấu ngoặc là chuyện thường (`<div>`, `<3`), và
// gán nhãn cho nó là dán tên khối của lõi lên chữ của người khác.
func TestBlockTagTakesBareNamesOnly(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"<W.O.N>\nđất", "W.O.N"},
		{"<Memory>index", "Memory"},
		{"<div align=\"center\">", ""},
		{"<3 chào", ""},
		{"<>", ""},
		{"không mở bằng ngoặc", ""},
		{"<a/b>", ""},
		{"<" + strings.Repeat("x", 5) + ">", "xxxxx"},
	} {
		if got := blockTag(c.in); got != c.want {
			t.Errorf("blockTag(%q) = %q, muốn %q", c.in, got, c.want)
		}
	}
}
