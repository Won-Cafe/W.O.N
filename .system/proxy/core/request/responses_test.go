// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package request

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func responsesBody(t *testing.T, raw string) *Body {
	t.Helper()
	b, err := ParseBody([]byte(raw), FormatOpenAIResponses)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// Cửa vào. /v1/responses → FormatOpenAIResponses, hậu tố lạ → Unknown.
func TestFormatFromPathResponses(t *testing.T) {
	cases := map[string]Format{
		"/v1/responses":                         FormatOpenAIResponses,
		"/v1beta/responses":                     FormatOpenAIResponses,
		"/openai/v1/responses":                  FormatOpenAIResponses,
		"/v1/messages":                          FormatAnthropic,
		"/v1/chat/completions":                  FormatOpenAI,
		"/v1beta/models/gemini:generateContent": FormatGemini,
		"/v1/responses/create":                  FormatUnknown,
		"/anything/else":                        FormatUnknown,
	}
	for path, want := range cases {
		if got := FormatFromPath(path); got != want {
			t.Errorf("%s → %v, muốn %v", path, got, want)
		}
	}
}

func TestFormatStringResponses(t *testing.T) {
	if got := FormatOpenAIResponses.String(); got != "openai-responses" {
		t.Fatalf("String: %q", got)
	}
}

// instructions là chuỗi top-level, đọc thẳng.
func TestResponsesSystemText(t *testing.T) {
	b := responsesBody(t, `{"model":"gpt-5","instructions":"you are helpful","input":"hi"}`)
	if got := b.SystemText(); got != "you are helpful" {
		t.Fatalf("SystemText: %q", got)
	}
	// Vắng instructions → rỗng.
	b = responsesBody(t, `{"model":"gpt-5","input":"hi"}`)
	if got := b.SystemText(); got != "" {
		t.Fatalf("vắng instructions: %q", got)
	}
}

func TestResponsesSystemBlocks(t *testing.T) {
	b := responsesBody(t, `{"instructions":"khối một","input":[]}`)
	blocks := b.SystemBlocks()
	if len(blocks) != 1 || blocks[0] != "khối một" {
		t.Fatalf("SystemBlocks: %+v", blocks)
	}
}

// System item trong input (tương thích ngược Chat Completions) — phải đọc được.
// DeepSeek Harness gửi lời hệ thống theo cách này, không dùng instructions.
func TestResponsesSystemItemInInput(t *testing.T) {
	b := responsesBody(t, `{"input":[
		{"type":"message","role":"system","content":[{"type":"input_text","text":"you are helpful"}]},
		{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}
	]}`)
	if got := b.SystemText(); got != "you are helpful" {
		t.Fatalf("SystemText từ system item: %q", got)
	}
	blocks := b.SystemBlocks()
	if len(blocks) != 1 || blocks[0] != "you are helpful" {
		t.Fatalf("SystemBlocks từ system item: %+v", blocks)
	}
}

// Cả instructions VÀ system items — nối cả hai, instructions trước.
func TestResponsesSystemBothInstructionsAndItem(t *testing.T) {
	b := responsesBody(t, `{"instructions":"từ instructions","input":[
		{"type":"message","role":"system","content":[{"type":"input_text","text":"từ item"}]},
		{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}
	]}`)
	got := b.SystemText()
	if !strings.Contains(got, "từ instructions") || !strings.Contains(got, "từ item") {
		t.Fatalf("phải thấy cả hai nguồn: %q", got)
	}
}

// StripFrame cắt khung trong system item của input (không chỉ instructions).
func TestResponsesStripFrameSystemItemInInput(t *testing.T) {
	rules := FrameRules{Strip: []string{"system-reminder"}}
	b := responsesBody(t, `{"input":[
		{"type":"message","role":"system","content":[{"type":"input_text","text":"<system-reminder>nhiễu</system-reminder>you are helpful"}]},
		{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}
	]}`)
	if !b.StripFrame(b.HostFrame(rules)) {
		t.Fatal("phải cắt được khung trong system item")
	}
	sys := b.SystemText()
	if strings.Contains(sys, "system-reminder") {
		t.Errorf("khung còn: %s", sys)
	}
	if !strings.Contains(sys, "you are helpful") {
		t.Errorf("chữ hệ thống mất: %s", sys)
	}
}

// Tình huống thực tế: system item trong input (content là CHUỖI, không có type:message),
// không có instructions field. AppendSystem 3 lần (ground, house, soul) — tất cả phải
// vào instructions field, và SystemText() phải thấy cả ba.
func TestResponsesInjectAllBlocksIntoInstructions(t *testing.T) {
	// DeepSeek Harness gửi system item trong input, content là chuỗi (không phải mảng).
	b := responsesBody(t, `{"model":"gpt-5","input":[
		{"role":"system","content":"You are an AI agent."},
		{"role":"user","content":[{"type":"input_text","text":"chào"}]}
	]}`)

	// SystemText phải thấy lời hệ thống của công cụ chủ.
	sys := b.SystemText()
	if !strings.Contains(sys, "You are an AI agent.") {
		t.Fatalf("SystemText phải thấy system item: %q", sys)
	}

	// AppendSystem 3 lần — mô phỏng appendBlock cho ground, house, soul.
	b.AppendSystem("<W.O.N>\nđất\n</W.O.N>")
	b.AppendSystem("<House>\nnhà\n</House>")
	b.AppendSystem("<Soul>\nlinh hồn\n</Soul>")

	// SystemText phải thấy cả ba khối VÀ chữ của công cụ chủ.
	got := b.SystemText()
	for _, tag := range []string{"<W.O.N>", "<House>", "<Soul>", "You are an AI agent."} {
		if !strings.Contains(got, tag) {
			t.Errorf("SystemText thiếu %q: %q", tag, got)
		}
	}

	// Marshal phải có system items trong input chứa cả ba — không tạo instructions field.
	out, err := b.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, tag := range []string{"<W.O.N>", "<House>", "<Soul>"} {
		if !strings.Contains(s, tag) {
			t.Errorf("Marshal thiếu %q trong output", tag)
		}
	}
	// Không được tạo field instructions — upstream không nhận.
	if strings.Contains(s, `"instructions"`) {
		t.Errorf("không được tạo instructions field: %s", s[:200])
	}
}

// System item trong input với content là CHUỖI (không phải mảng blocks) —
// phải đọc được. DeepSeek Harness gửi theo cách này.
func TestResponsesSystemItemStringContent(t *testing.T) {
	b := responsesBody(t, `{"input":[
		{"role":"system","content":"You are an AI agent."},
		{"role":"user","content":[{"type":"input_text","text":"hi"}]}
	]}`)
	if got := b.SystemText(); got != "You are an AI agent." {
		t.Fatalf("SystemText từ system item chuỗi: %q", got)
	}
}

// AppendSystem chèn system item MỚI vào input, ở cuối vùng system. Nếu có instructions
// field, nó cũng được đọc — nhưng lõi chèn vào input, không tạo instructions.
func TestResponsesAppendSystem(t *testing.T) {
	b := responsesBody(t, `{"instructions":"root","input":[{"role":"user","content":[{"type":"input_text","text":"hi"}]}]}`)
	b.AppendSystem("đất")
	b.AppendSystem("soul")
	// SystemText thấy cả instructions VÀ system items mới.
	sysText := b.SystemText()
	if !strings.Contains(sysText, "root") || !strings.Contains(sysText, "đất") || !strings.Contains(sysText, "soul") {
		t.Fatalf("SystemText sau append: %q", sysText)
	}
	// System items mới phải ở cuối vùng system (sau instructions, trước user).
	var got struct {
		Input []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"input"`
	}
	out, _ := b.Marshal()
	_ = json.Unmarshal(out, &got)
	// Phải có ít nhất 3 system items (root từ instructions không thành item; đất, soul là items mới).
	// Thực tế: instructions không tạo item, chỉ đất và soul thành items.
	sysCount := 0
	for _, item := range got.Input {
		if item.Role == "system" {
			sysCount++
		}
	}
	if sysCount != 2 {
		t.Fatalf("muốn 2 system items (đất, soul), got %d", sysCount)
	}
}

// Vắng instructions, vắng system items → AppendSystem chèn system item mới vào input.
func TestResponsesAppendSystemCreatesField(t *testing.T) {
	b := responsesBody(t, `{"input":[{"role":"user","content":[{"type":"input_text","text":"hi"}]}]}`)
	b.AppendSystem("<W.O.N>\nđất\n</W.O.N>")
	if !strings.Contains(b.SystemText(), "<W.O.N>") {
		t.Fatalf("không chèn được system item: %s", b.SystemText())
	}
	// Phải có 1 system item trong input.
	var got struct {
		Input []struct {
			Role string `json:"role"`
		} `json:"input"`
	}
	out, _ := b.Marshal()
	_ = json.Unmarshal(out, &got)
	if len(got.Input) != 2 || got.Input[0].Role != "system" || got.Input[1].Role != "user" {
		t.Fatalf("phải có system item trước user: %+v", got.Input)
	}
}

// ReplaceSystem gộp mọi system item trong input thành một.
func TestResponsesReplaceSystem(t *testing.T) {
	b := responsesBody(t, `{"input":[
		{"role":"system","content":"cũ"},
		{"role":"user","content":[{"type":"input_text","text":"hi"}]}
	]}`)
	b.ReplaceSystem("mới")
	if got := b.SystemText(); got != "mới" {
		t.Fatalf("ReplaceSystem: %q", got)
	}
}

// input mảng items — đọc được hội thoại.
func TestResponsesMessageList(t *testing.T) {
	b := responsesBody(t, `{"input":[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"chào"}]},
		{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ừ"}]}
	]}`)
	msgs := b.messages()
	if len(msgs) != 2 {
		t.Fatalf("muốn 2 items, got %d", len(msgs))
	}
}

// input chuỗi (one-shot) — không phải mảng, messageList trả nil.
func TestResponsesStringInputNoMessages(t *testing.T) {
	b := responsesBody(t, `{"input":"chào"}`)
	if msgs := b.messages(); msgs != nil {
		t.Fatalf("string input không phải mảng, muốn nil, got %d", len(msgs))
	}
}

// flattenResponses lọc input_text và output_text, không lọc text.
func TestResponsesFlatten(t *testing.T) {
	b := responsesBody(t, `{"input":[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"chào"}]},
		{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ừ"}]}
	]}`)
	msgs := b.messages()
	if got := b.flatten(b.contentOf(msgs[0])); got != "chào" {
		t.Errorf("input_text: %q", got)
	}
	if got := b.flatten(b.contentOf(msgs[1])); got != "ừ" {
		t.Errorf("output_text: %q", got)
	}
}

// content type "text" (kiểu Chat Completions) không được nhận — Responses dùng input_text/output_text.
func TestResponsesFlattenIgnoresPlainTextType(t *testing.T) {
	b := responsesBody(t, `{"input":[
		{"type":"message","role":"user","content":[{"type":"text","text":"bị bỏ"}]}
	]}`)
	msgs := b.messages()
	if got := b.flatten(b.contentOf(msgs[0])); got != "" {
		t.Errorf("type text phải bị bỏ, got %q", got)
	}
}

// Snapshot đọc đúng items, role, text.
func TestResponsesSnapshot(t *testing.T) {
	b := responsesBody(t, `{"instructions":"sys","input":[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"greeting"}]},
		{"type":"message","role":"assistant","content":[{"type":"output_text","text":"reply"}]},
		{"type":"message","role":"user","content":[{"type":"input_text","text":"turn two"}]}
	]}`)
	snap := b.Snapshot(FrameRules{})
	if snap.System != "sys" {
		t.Errorf("System: %q", snap.System)
	}
	if snap.FirstUser != "greeting" {
		t.Errorf("FirstUser: %q", snap.FirstUser)
	}
	if snap.FirstAssistant != "reply" {
		t.Errorf("FirstAssistant: %q", snap.FirstAssistant)
	}
	if snap.Anchor != "turn two" {
		t.Errorf("Anchor: %q", snap.Anchor)
	}
	if snap.HumanTurns != 2 {
		t.Errorf("HumanTurns: %d", snap.HumanTurns)
	}
}

// function_call items không có role — responsesRoleOf suy assistant.
func TestResponsesRoleInference(t *testing.T) {
	b := responsesBody(t, `{"input":[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"đọc file"}]},
		{"type":"function_call","name":"read_file","arguments":"{\"file_path\":\"README.md\"}","call_id":"c1"},
		{"type":"function_call_output","call_id":"c1","output":"ruột file"},
		{"type":"message","role":"assistant","content":[{"type":"output_text","text":"xong"}]}
	]}`)
	snap := b.Snapshot(FrameRules{})
	// function_call → assistant → Reached thấy.
	if len(snap.Reached) != 1 || snap.Reached[0].Name != "read_file" {
		t.Fatalf("Reached: %+v", snap.Reached)
	}
	if snap.Reached[0].Target != "README.md" {
		t.Errorf("Target: %q", snap.Reached[0].Target)
	}
	// function_call_output → user, nhưng không phải lời người → HumanTurns = 1.
	if snap.HumanTurns != 1 {
		t.Errorf("HumanTurns: %d (function_call_output không phải lời người)", snap.HumanTurns)
	}
	if snap.FirstAssistant != "xong" {
		t.Errorf("FirstAssistant: %q", snap.FirstAssistant)
	}
}

// HumanSpokeLast — message user cuối = true, function_call_output cuối = false.
func TestResponsesHumanSpokeLast(t *testing.T) {
	human := responsesBody(t, `{"input":[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"ừ"}]}
	]}`)
	if !human.HumanSpokeLast() {
		t.Error("lời người cuối → phải true")
	}
	tool := responsesBody(t, `{"input":[
		{"type":"function_call","name":"f","call_id":"c1"},
		{"type":"function_call_output","call_id":"c1","output":"r"}
	]}`)
	if tool.HumanSpokeLast() {
		t.Error("function_call_output cuối → không phải người")
	}
}

// DispatchMarked — mỏ neo nằm trong chữ hội thoại, đi qua flatten.
func TestResponsesDispatchMarked(t *testing.T) {
	b := responsesBody(t, `{"input":[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"`+DispatchMark+` gọi — dò"}]}
	]}`)
	if !b.DispatchMarked(FrameRules{}) {
		t.Error("lời giao phải được nhận ra")
	}
}

// AgentTurnShaped — có instructions → có lời dạy → true.
func TestResponsesAgentTurnShaped(t *testing.T) {
	withInstructions := responsesBody(t, `{"instructions":"you are helpful","input":[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}
	]}`)
	if !withInstructions.AgentTurnShaped(FrameRules{}) {
		t.Error("có instructions → phải là lượt của đệ")
	}
	// Có tools cũng đủ.
	withTools := responsesBody(t, `{"tools":[{"name":"read"}],"input":[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}
	]}`)
	if !withTools.AgentTurnShaped(FrameRules{}) {
		t.Error("có tools → phải là lượt của đệ")
	}
	// Không có gì → false.
	empty := responsesBody(t, `{"input":[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}
	]}`)
	if empty.AgentTurnShaped(FrameRules{}) {
		t.Error("không lời dạy, không tools → không phải lượt của đệ")
	}
}

// ConversationShaped — 2+ items hoặc có tools.
func TestResponsesConversationShaped(t *testing.T) {
	oneMsg := responsesBody(t, `{"input":[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}
	]}`)
	if oneMsg.ConversationShaped() {
		t.Error("một message, không tools → không phải hội thoại")
	}
	twoMsgs := responsesBody(t, `{"input":[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]},
		{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ừ"}]}
	]}`)
	if !twoMsgs.ConversationShaped() {
		t.Error("hai items → phải có hình hội thoại")
	}
	withTools := responsesBody(t, `{"tools":[{"name":"read"}],"input":[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}
	]}`)
	if !withTools.ConversationShaped() {
		t.Error("có tools → phải có hình hội thoại")
	}
}

// ToolInfos — name top-level (như Anthropic), không lồng function.
func TestResponsesToolInfos(t *testing.T) {
	b := responsesBody(t, `{"tools":[
		{"name":"read_file","description":"đọc một file"},
		{"name":"run_shell_command","description":"chạy lệnh"}
	],"input":[]}`)
	got := b.ToolInfos()
	if len(got) != 2 || got[0].Name != "read_file" || got[1].Name != "run_shell_command" {
		t.Fatalf("ToolInfos: %+v", got)
	}
	if got[0].Description != "đọc một file" {
		t.Errorf("description: %q", got[0].Description)
	}
}

// AppendMessage — chèn message item mới ở cuối, hình Responses.
func TestResponsesAppendMessage(t *testing.T) {
	b := responsesBody(t, `{"input":[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"chào"}]}
	]}`)
	b.AppendMessage(RoleUser, "<Wayfarer>\nmốc\n</Wayfarer>")

	var got struct {
		Input []struct {
			Type    string `json:"type"`
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"input"`
	}
	out, _ := b.Marshal()
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Input) != 2 {
		t.Fatalf("phải có 2 items, got %d", len(got.Input))
	}
	last := got.Input[1]
	if last.Type != "message" || last.Role != "user" {
		t.Errorf("type/role sai: %s/%s", last.Type, last.Role)
	}
	if len(last.Content) != 1 || last.Content[0].Type != "input_text" {
		t.Errorf("content type sai: %+v", last.Content)
	}
	if !strings.Contains(last.Content[0].Text, "Wayfarer") {
		t.Errorf("text sai: %q", last.Content[0].Text)
	}
}

// Lossless: field lõi không đụng đi qua nguyên byte.
func TestResponsesMarshalKeepsUntouchedFields(t *testing.T) {
	raw := `{"model":"gpt-5","temperature":0.7,"instructions":"sys","input":[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}],
		"previous_response_id":"resp_abc","store":true}`
	b := responsesBody(t, raw)
	b.AppendSystem("đất")
	out, err := b.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"temperature":0.7`,
		`"previous_response_id":"resp_abc"`,
		`"store":true`,
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("mất %s trong: %s", want, out)
		}
	}
}

// CacheMarks — instructions đứng trước items (như Anthropic, không như OpenAI).
func TestResponsesCacheMarks(t *testing.T) {
	b := responsesBody(t, `{"instructions":"<W.O.N>đất","input":[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"chào"}]},
		{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ừ"}]}
	]}`)
	marks := b.CacheMarks()
	want := []string{"system", "user", "assistant"}
	if len(marks) != len(want) {
		t.Fatalf("muốn %d mắt, got %d: %+v", len(want), len(marks), marks)
	}
	for i, w := range want {
		if marks[i].Slot != w {
			t.Errorf("mắt %d muốn slot %q, got %q", i, w, marks[i].Slot)
		}
	}
	if marks[0].Label != "W.O.N" {
		t.Errorf("nhãn khối đất: %q", marks[0].Label)
	}
}

// StripFrame cắt khung trong instructions.
func TestResponsesStripFrameSystem(t *testing.T) {
	rules := FrameRules{Strip: []string{"system-reminder"}}
	b := responsesBody(t, `{"instructions":"<system-reminder>nhiễu</system-reminder># Tzu","input":[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"chào"}]}
	]}`)
	if !b.StripFrame(b.HostFrame(rules)) {
		t.Fatal("phải cắt được")
	}
	sys := b.SystemText()
	if strings.Contains(sys, "system-reminder") {
		t.Errorf("khung còn: %s", sys)
	}
	if !strings.Contains(sys, "# Tzu") {
		t.Errorf("soul mất: %s", sys)
	}
}

// StripFrame cắt khung trong content blocks của message user.
func TestResponsesStripFrameMessage(t *testing.T) {
	rules := FrameRules{Strip: []string{"system-reminder"}}
	b := responsesBody(t, `{"instructions":"# Tzu","input":[
		{"type":"message","role":"user","content":[
			{"type":"input_text","text":"<system-reminder>nhiễu</system-reminder>"},
			{"type":"input_text","text":"chào"}
		]}
	]}`)
	if !b.StripFrame(b.HostFrame(rules)) {
		t.Fatal("phải cắt được")
	}
	out, _ := b.Marshal()
	if strings.Contains(string(out), "system-reminder") {
		t.Errorf("khung còn: %s", out)
	}
	if !strings.Contains(string(out), "chào") {
		t.Errorf("chữ người mất: %s", out)
	}
}

// MachineReply — text.format.type json_schema.
func TestResponsesMachineReply(t *testing.T) {
	b := responsesBody(t, `{"text":{"format":{"type":"json_schema"}},"input":"hi"}`)
	if !b.MachineReply() {
		t.Error("text.format.type json_schema → MachineReply true")
	}
	plain := responsesBody(t, `{"input":"hi"}`)
	if plain.MachineReply() {
		t.Error("không có text.format → MachineReply false")
	}
}

// AppendMessage không chạm items hiện có — từng byte nguyên.
func TestResponsesAppendMessagePreservesExisting(t *testing.T) {
	existing := `{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}`
	b := responsesBody(t, `{"input":[`+existing+`]}`)
	b.AppendMessage(RoleUser, "tiếng của lượt")

	var got struct {
		Input []json.RawMessage `json:"input"`
	}
	out, _ := b.Marshal()
	_ = json.Unmarshal(out, &got)
	if len(got.Input) != 2 {
		t.Fatalf("phải 2 items, got %d", len(got.Input))
	}
	if !bytes.Equal(got.Input[0], []byte(existing)) {
		t.Fatalf("item cũ bị chạm:\nwant %s\ngot  %s", existing, got.Input[0])
	}
}
