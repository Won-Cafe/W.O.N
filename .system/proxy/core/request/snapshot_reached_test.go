// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package request

import "testing"

// Hình thật của lượt assistant gọi tool trên đường OpenAI: content là CHUỖI rỗng,
// tool_calls mang tên món. Đọc ra từ nhật ký chẩn bệnh của một phiên thật.
func TestReachedOpenAIToolCallWithStringContent(t *testing.T) {
	b := mustParseFmt(t, `{"messages":[
		{"role":"user","content":"hỏi"},
		{"role":"assistant","content":"","tool_calls":[{"id":"c1","type":"function","function":{"name":"read_file","arguments":"{\"file_path\":\"README.md\"}"}}]},
		{"role":"tool","tool_call_id":"c1","content":"ruột file"}
	]}`, FormatOpenAI)
	got := b.Snapshot(FrameRules{}).Reached
	if len(got) != 1 {
		t.Fatalf("với tới 1 món mà đọc ra %d: %+v", len(got), got)
	}
	if got[0].Name != "read_file" || got[0].Target != "README.md" {
		t.Errorf("got %+v", got[0])
	}
}

// content: null cũng phải đọc được — nhà nào cũng gửi kiểu này.
func TestReachedOpenAIToolCallWithNullContent(t *testing.T) {
	b := mustParseFmt(t, `{"messages":[
		{"role":"assistant","content":null,"tool_calls":[{"function":{"name":"file_search","arguments":"{\"query\":\"x\"}"}}]}
	]}`, FormatOpenAI)
	if got := b.Snapshot(FrameRules{}).Reached; len(got) != 1 || got[0].Name != "file_search" {
		t.Fatalf("got %+v", got)
	}
}

// Công cụ chủ mở MỌI phiên bằng cùng một message việc nhà (`<environment_info>`), nên neo
// khoá phiên vào message user đầu là neo vào một hằng số: mọi hội thoại của cùng một đệ ra
// cùng một khoá. Đo trên nhật ký thật — mọi phiên Sun đều mang đúng
// `derive(Sun, "<environment_info>…")`, và khoá ấy không bao giờ thăng cấp vì lượt gọi tool
// không có chữ để làm mỏ neo thứ hai.
func TestFirstUserAnchorsOnAHumanTurnNotTheHostPreamble(t *testing.T) {
	rules := FrameRules{Unwrap: []string{"userRequest"}}
	body := mustParseFmt(t, `{"messages":[
		{"role":"user","content":"<environment_info>\nThe user's current OS is: Windows\n</environment_info>"},
		{"role":"user","content":"👋 Tzu gọi — dò kỹ shields.io"}]}`, FormatOpenAI)

	snap := body.Snapshot(rules)
	if snap.FirstUser != "👋 Tzu gọi — dò kỹ shields.io" {
		t.Errorf("mỏ neo phải là LƯỢT của người, got %q", snap.FirstUser)
	}
	if snap.HumanTurns != 1 {
		t.Errorf("message việc nhà không phải một lượt: %d", snap.HumanTurns)
	}
}

// Chưa khai vỏ nào chở lời người thì lõi KHÔNG siết (xem humanTurn): mọi message user có chữ
// vẫn là một lượt, và mỏ neo về đúng nếp cũ. Siết mà không biết vỏ tên gì thì mọi lượt hoá
// việc nhà, và lúc ấy không phiên nào có mỏ neo.
func TestFirstUserKeepsTheOldAnchorWhenNoWrapperIsDeclared(t *testing.T) {
	body := mustParseFmt(t, `{"messages":[
		{"role":"user","content":"<environment_info>OS: Windows</environment_info>"},
		{"role":"user","content":"câu thật"}]}`, FormatOpenAI)

	if snap := body.Snapshot(FrameRules{}); snap.FirstUser != "<environment_info>OS: Windows</environment_info>" {
		t.Errorf("chưa khai vỏ thì không siết: %q", snap.FirstUser)
	}
}

// Đích rút được, nhưng nó là LOẠI gì thì phải nói ra. Chỗ này quyết định một nhãn ở tầng
// trên: hỏi vùng-trong-cây của một câu lệnh PowerShell thì hàm đọc vùng cắt tiền tố câu
// lệnh và trả nó về như một tên vùng (`cd C:/`). Đo trên nhật ký thật: 35 trong 232 dòng
// bản kê gửi cho Outfitter mang một nhãn kiểu ấy.
func TestReachedTellsWhatKindOfTargetItRead(t *testing.T) {
	b := mustParseFmt(t, `{"messages":[
		{"role":"assistant","content":null,"tool_calls":[
			{"function":{"name":"run_in_terminal","arguments":"{\"command\":\"cd C:\\\\won; go vet ./...\"}"}},
			{"function":{"name":"read_file","arguments":"{\"file_path\":\"README.md\"}"}},
			{"function":{"name":"fetch","arguments":"{\"url\":\"https://example.com/a\"}"}},
			{"function":{"name":"grep_search","arguments":"{\"pattern\":\"func .*\"}"}},
			{"function":{"name":"task_complete","arguments":"{}"}}
		]}
	]}`, FormatOpenAI)

	got := b.Snapshot(FrameRules{}).Reached
	want := []TargetKind{TargetCommand, TargetPath, TargetURL, TargetPattern, TargetNone}
	if len(got) != len(want) {
		t.Fatalf("với tới %d món mà đọc ra %d: %+v", len(want), len(got), got)
	}
	for i, k := range want {
		if got[i].Kind != k {
			t.Errorf("%s: loại đích %v, muốn %v", got[i].Name, got[i].Kind, k)
		}
	}
	// Loại đọc-ra-vùng không mang tên loại: chỗ gọi bằng tên vùng của nó.
	if TargetPath.Label() != "" || TargetNone.Label() != "" {
		t.Error("chỗ không được mang tên loại — nhãn của nó là tên vùng")
	}
	for _, k := range LabeledKinds() {
		if k.Label() == "" {
			t.Errorf("loại %v không có tên gọi", k)
		}
	}
}
