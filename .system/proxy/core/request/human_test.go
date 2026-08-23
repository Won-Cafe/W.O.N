// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package request

import (
	"encoding/json"
	"strings"
	"testing"
)

// Cái bẫy: Anthropic gửi KẾT QUẢ TOOL dưới vai `user`, khối `tool_result`. Nên
// `lastRole == "user"` một mình là mốc sai — giữa vòng tool nó vẫn đúng, và cửa
// nhịp mở toang. Điều kiện thật: vai user, và có ít nhất một khối không phải
// tool_result.
func TestHumanSpokeLast(t *testing.T) {
	cases := []struct {
		name, body string
		want       bool
	}{
		{"lời người, chuỗi trần",
			`{"messages":[{"role":"user","content":"đo giúp tôi"}]}`, true},
		{"lời người, khối text",
			`{"messages":[{"role":"user","content":[{"type":"text","text":"đo giúp tôi"}]}]}`, true},
		{"kết quả tool kiểu Anthropic — KHÔNG phải người",
			`{"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"ruột file"}]}]}`, false},
		{"kết quả tool kèm lời người trong cùng lượt",
			`{"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1"},{"type":"text","text":"tiếp đi"}]}]}`, true},
		{"kết quả tool kiểu OpenAI",
			`{"messages":[{"role":"user","content":"hi"},{"role":"tool","content":"ruột"}]}`, false},
		{"đệ vừa nói", `{"messages":[{"role":"assistant","content":"rồi"}]}`, false},
		{"lượt user rỗng ruột", `{"messages":[{"role":"user","content":"  "}]}`, false},
		{"không có message nào", `{"messages":[]}`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := mustParseFmt(t, c.body, FormatAnthropic)
			if got := b.HumanSpokeLast(); got != c.want {
				t.Errorf("muốn %v, got %v", c.want, got)
			}
		})
	}
}

// Lượt gọi tool đọc từ lượt ASSISTANT — chỗ nó với tay, kể cả khi kết quả chưa về.
// Hai nhà, hai hình, và cả hai đều mang được ĐÍCH: "với món gì" một mình không nói
// được đệ đang đọc hay đang làm thay người.
func TestReached(t *testing.T) {
	oai := mustParseFmt(t, `{"messages":[
		{"role":"assistant","tool_calls":[
			{"id":"a","function":{"name":"grep_search","arguments":"{\"pattern\":\"lấn\"}"}},
			{"id":"b","function":{"name":"read_file","arguments":"{\"file_path\":\"Stories/x.md\"}"}}]},
		{"role":"tool","tool_call_id":"a","content":"x"},
		{"role":"assistant","tool_calls":[{"id":"c","function":{"name":"read_file"}}]}
	]}`, FormatOpenAI)
	got := oai.Snapshot(FrameRules{}).Reached
	if len(got) != 3 {
		t.Fatalf("OpenAI: %v", got)
	}
	if got[0] != (ToolCall{Name: "grep_search", Target: "lấn", Kind: TargetPattern}) {
		t.Errorf("OpenAI đích từ arguments dạng chuỗi JSON: %+v", got[0])
	}
	if got[1] != (ToolCall{Name: "read_file", Target: "Stories/x.md", Kind: TargetPath}) {
		t.Errorf("OpenAI: %+v", got[1])
	}
	if got[2] != (ToolCall{Name: "read_file"}) {
		t.Errorf("không có tham số → chỉ tên món, không bịa đích: %+v", got[2])
	}

	anth := mustParseFmt(t, `{"messages":[
		{"role":"assistant","content":[
			{"type":"text","text":"để tôi xem"},
			{"type":"tool_use","id":"t1","name":"Write","input":{"file_path":"Own/a.md","content":"RUỘT FILE"}}]}
	]}`, FormatAnthropic)
	got = anth.Snapshot(FrameRules{}).Reached
	if len(got) != 1 || got[0] != (ToolCall{Name: "Write", Target: "Own/a.md", Kind: TargetPath}) {
		t.Fatalf("Anthropic: %+v", got)
	}
}

// Chỉ những khoá tham số nói ĐÍCH được lấy. Quét bừa thì `content` của một lần ghi
// file lọt vào ngữ cảnh — vừa vô nghĩa vừa là chỗ bí mật rò ra (#5).
func TestReachedTakesOnlyTargetKeys(t *testing.T) {
	b := mustParseFmt(t, `{"messages":[{"role":"assistant","content":[
		{"type":"tool_use","name":"Write","input":{"content":"BÍ MẬT","token":"sk-123"}}]}]}`, FormatAnthropic)
	got := b.Snapshot(FrameRules{}).Reached
	if len(got) != 1 || got[0].Target != "" {
		t.Fatalf("chỉ khoá đích được lấy, got %+v", got)
	}
}

// Trần giữ phần MỚI: nếp tay gần đây mới nói được điều gì, còn cả biên niên thì không.
func TestReachedKeepsNewest(t *testing.T) {
	var calls []string
	for i := 0; i < reachedMax+3; i++ {
		calls = append(calls, `{"function":{"name":"t`+string(rune('a'+i))+`"}}`)
	}
	b := mustParseFmt(t, `{"messages":[{"role":"assistant","tool_calls":[`+
		strings.Join(calls, ",")+`]}]}`, FormatOpenAI)
	got := b.Snapshot(FrameRules{}).Reached
	if len(got) != reachedMax {
		t.Fatalf("phải cắt còn %d, got %d", reachedMax, len(got))
	}
	if want := "t" + string(rune('a'+reachedMax+2)); got[len(got)-1].Name != want {
		t.Errorf("cắt phải giữ cái mới nhất, muốn %q, got %v", want, got)
	}
}

// Việc nhà của công cụ chủ dưới vai user KHÔNG phải một lượt của người. Đo trên hình
// thật VS Code gửi: một message thuần <environment_info>/<workspace_info>, rồi một
// message chở <attachments>/<context> với câu của người nằm trong <userRequest>. Đếm
// theo vai thì lượt người đầu tiên đã mang số 2, và nhật ký mở thư mục ở t-2.
func TestHumanTurnsSkipsHostChores(t *testing.T) {
	rules := FrameRules{Strip: []string{"system-reminder", "instructions"}, Unwrap: []string{"userRequest"}}
	b := mustBody(t, `{"messages":[
		{"role":"system","content":"You are…"},
		{"role":"user","content":"<environment_info>OS: Windows</environment_info> <workspace_info>cây thư mục</workspace_info>"},
		{"role":"user","content":"<attachments>không có</attachments> <context>hôm nay</context> <userRequest>câu thật của người</userRequest>"}
	]}`, FormatOpenAI)
	if got := b.Snapshot(rules).HumanTurns; got != 1 {
		t.Errorf("một lượt người thật, got %d", got)
	}
}

// Vỏ lẻ không đủ cặp thì không phải vỏ: người viết `a < b` không được hoá việc nhà.
func TestHumanTurnKeepsBareText(t *testing.T) {
	b := mustBody(t, `{"messages":[{"role":"user","content":"so sánh a < b rồi <chua-dong"}]}`, FormatOpenAI)
	if got := b.Snapshot(FrameRules{}).HumanTurns; got != 1 {
		t.Errorf("chữ trần là lượt người, got %d", got)
	}
}

// jsonStr — chuỗi Go thành literal JSON. Chữ thử có nhiều dòng và dấu ngoặc, nối tay vào
// là dựng một JSON hỏng rồi đi tìm bug ở chỗ khác.
func jsonStr(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// Nhật ký chia lượt theo LỜI NGƯỜI: vai assistant và vai tool không mở lượt, và ruột kết
// quả tool không lọt vào — một lần grep trả 14.000 ký tự, in vào nhật ký là dìm mọi thứ
// đáng đọc. Lời người thì giữ NGUYÊN VĂN: chỗ duy nhất trong nhật ký không có trần.
func TestHumanTextsKeepsOnlyHumanUtterances(t *testing.T) {
	long := strings.Repeat("đo giúp tôi ", 300)
	b := mustParseFmt(t, `{"messages":[
		{"role":"user","content":"đếm file"},
		{"role":"assistant","content":"","tool_calls":[{"id":"c1","function":{"name":"grep_search"}}]},
		{"role":"tool","tool_call_id":"c1","content":"MỘT ĐỐNG CHỮ RẤT DÀI"},
		{"role":"user","content":`+jsonStr(long)+`}
	]}`, FormatOpenAI)

	got := b.HumanTexts(nil, FrameRules{})
	if len(got) != 2 {
		t.Fatalf("muốn 2 lượt người, got %d: %q", len(got), got)
	}
	if got[0] != "đếm file" {
		t.Errorf("lượt 1 sai: %q", got[0])
	}
	if got[1] != long {
		t.Errorf("lời người bị cắt: dài %d, muốn %d", len(got[1]), len(long))
	}
}

// Chữ lõi vừa đặt phải trừ ra: tiếng của lượt đi vai user, không trừ thì nhật ký mở một
// lượt ma cho chính chữ của mình.
func TestHumanTextsSkipsOwnInsertions(t *testing.T) {
	b := mustParseFmt(t, `{"messages":[
		{"role":"user","content":"đếm file"},
		{"role":"user","content":"<Loiterer>🚶 kìa</Loiterer>"}
	]}`, FormatOpenAI)

	got := b.HumanTexts([]string{"<Loiterer>🚶 kìa</Loiterer>"}, FrameRules{})
	if len(got) != 1 || got[0] != "đếm file" {
		t.Errorf("chỉ lời người mở lượt, got %q", got)
	}
}

// MỘT định nghĩa "lượt người" cho cả hệ. Bộ đếm của phiên (`Snapshot.HumanTurns`) lọc
// việc nhà của công cụ chủ, nên nhật ký phải lọc y hệt — hai vị khác nhau là hai sự thật
// khác nhau về cùng một hội thoại.
//
// Đo được trên nhật ký thật trước khi sửa: một message chỉ có `<system-reminder>` mở một
// lượt trong nhật ký, VÀ một lần chạy bị gán vào nó, trong khi phiên không đếm nó.
func TestHumanTextsAgreesWithHumanTurns(t *testing.T) {
	rules := FrameRules{Unwrap: []string{"userRequest"}}
	b := mustParseFmt(t, `{"messages":[
		{"role":"user","content":"<ide_opened_file>x</ide_opened_file>đếm file giúp tôi"},
		{"role":"assistant","content":"ừ"},
		{"role":"user","content":"<system-reminder>\nPostToolUse:Read hook additional context\n</system-reminder>"}
	]}`, FormatAnthropic)

	got := b.HumanTexts(nil, rules)
	if want := b.Snapshot(rules).HumanTurns; len(got) != want {
		t.Fatalf("nhật ký thấy %d lượt, phiên đếm %d — hai định nghĩa lệch nhau: %q", len(got), want, got)
	}
	if len(got) != 1 {
		t.Fatalf("chỉ một lượt người thật, got %d: %q", len(got), got)
	}
	if !strings.Contains(got[0], "đếm file giúp tôi") {
		t.Errorf("lượt giữ lại phải là lời người: %q", got[0])
	}
}

// Chưa khai vỏ nào chở lời người thì lõi KHÔNG siết (#6) — cùng luật với humanTurn. Siết
// mù thì mọi lượt hoá việc nhà, và nhật ký đếm 0, tệ hơn đếm thừa.
func TestHumanTextsDoesNotTightenWithoutRules(t *testing.T) {
	b := mustParseFmt(t, `{"messages":[
		{"role":"user","content":"<system-reminder>chỉ có vỏ</system-reminder>"}
	]}`, FormatAnthropic)

	if got := b.HumanTexts(nil, FrameRules{}); len(got) != 1 {
		t.Errorf("chưa khai unwrap_tags thì không siết, got %d: %q", len(got), got)
	}
}
