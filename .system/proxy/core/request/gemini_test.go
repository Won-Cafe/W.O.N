// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package request

import (
	"encoding/json"
	"strings"
	"testing"
)

func geminiBody(t *testing.T, raw string) *Body {
	t.Helper()
	b, err := ParseBody([]byte(raw), FormatGemini)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// Cửa vào. `:streamGenerateContent` KHÔNG có hậu tố `:generateContent` (dấu hai chấm nằm
// sai chỗ), nên phải là hai nhánh, không một.
func TestFormatFromPathGemini(t *testing.T) {
	cases := map[string]Format{
		"/v1beta/models/gemini-3-pro:generateContent":       FormatGemini,
		"/v1beta/models/gemini-3-pro:streamGenerateContent": FormatGemini,
		"/v1/messages":                    FormatAnthropic,
		"/v1beta/openai/chat/completions": FormatOpenAI,
		"/v1beta/models":                  FormatUnknown,
		// Cửa gói phong bì (`/v1internal:generateContent` và bạn của nó) KHÔNG còn nhánh
		// riêng ở đây: hậu tố của chúng đúng là hậu tố Gemini, nên router trả Gemini. Cái
		// giữ cho thân ấy không bị chạm đã dời xuống các cửa đo HÌNH — xem
		// TestEnvelopedBodyStaysUntouched. Đo hình thì đúng cho mọi cửa phong bì; đọc tên
		// path thì chỉ đúng cho một dịch vụ.
		"/v1internal:generateContent":       FormatGemini,
		"/v1internal:streamGenerateContent": FormatGemini,
	}
	for path, want := range cases {
		if got := FormatFromPath(path); got != want {
			t.Errorf("%s → %v, muốn %v", path, got, want)
		}
	}
}

// Lời hệ thống của Gemini là MỘT Content object, không phải mảng block. Cả hai cách viết
// khoá đều phải đọc được: SDK JS gửi camelCase, một client viết tay có thể gửi snake.
func TestGeminiSystemText(t *testing.T) {
	for _, key := range []string{"systemInstruction", "system_instruction"} {
		b := geminiBody(t, `{"`+key+`":{"parts":[{"text":"đất"},{"text":"nhà"}]},"contents":[]}`)
		if got := b.SystemText(); got != "đất\n\nnhà" {
			t.Errorf("%s: SystemText = %q", key, got)
		}
		if blocks := b.SystemBlocks(); len(blocks) != 2 {
			t.Errorf("%s: phải thấy 2 khối, got %d", key, len(blocks))
		}
	}
}

// Vắng systemInstruction thì AppendSystem phải DỰNG MỚI — Gemini CLI gửi request không có
// field này là chuyện thường, và không dựng thì không lượt nào nhận được đất.
func TestGeminiAppendSystemCreatesField(t *testing.T) {
	b := geminiBody(t, `{"contents":[{"role":"user","parts":[{"text":"chào"}]}]}`)
	b.AppendSystem("<W.O.N>\nđất\n</W.O.N>")
	if !strings.Contains(b.SystemText(), "<W.O.N>") {
		t.Fatalf("không dựng được systemInstruction: %s", b.SystemText())
	}
}

// Mỗi khối lõi chèn là đúng MỘT part mới, không nối chữ vào part đang có (§ Format wire:
// "mỗi tiếng nói một khối riêng, không trộn"). Và mọi field khác của Content object phải
// đi qua nguyên byte (#7).
func TestGeminiAppendSystemKeepsSeamAndSiblings(t *testing.T) {
	b := geminiBody(t, `{"systemInstruction":{"role":"system","parts":[{"text":"của Gemini CLI"}]},"contents":[]}`)
	b.AppendSystem("đất")
	b.AppendSystem("nhà")

	blocks := b.SystemBlocks()
	want := []string{"của Gemini CLI", "đất", "nhà"}
	if len(blocks) != len(want) {
		t.Fatalf("phải là 3 part rời, got %d: %q", len(blocks), blocks)
	}
	for i := range want {
		if blocks[i] != want[i] {
			t.Errorf("part %d = %q, muốn %q", i, blocks[i], want[i])
		}
	}

	out, err := b.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"role":"system"`) {
		t.Errorf("field `role` của systemInstruction bị mất: %s", out)
	}
}

// Hội thoại nằm ở `contents`, vai model tên là `model`, và tool gọi bằng part
// `functionCall`. Không quy vai về tên chung thì Reached rỗng và khoá phiên không thăng cấp.
func TestGeminiSnapshotReadsContentsAndNormalizesRole(t *testing.T) {
	b := geminiBody(t, `{"contents":[
		{"role":"user","parts":[{"text":"đọc file đi"}]},
		{"role":"model","parts":[{"functionCall":{"name":"read_file","args":{"path":"Own/a.md"}}}]},
		{"role":"user","parts":[{"functionResponse":{"name":"read_file","response":{"x":1}}}]},
		{"role":"model","parts":[{"text":"xong rồi"}]},
		{"role":"user","parts":[{"text":"tốt"}]}
	]}`)
	snap := b.Snapshot(FrameRules{})

	if snap.FirstUser != "đọc file đi" {
		t.Errorf("FirstUser = %q", snap.FirstUser)
	}
	if snap.FirstAssistant != "xong rồi" {
		t.Errorf("FirstAssistant = %q — vai `model` chưa quy về `assistant`", snap.FirstAssistant)
	}
	if len(snap.Reached) != 1 || snap.Reached[0].Name != "read_file" {
		t.Fatalf("Reached = %+v", snap.Reached)
	}
	if snap.Reached[0].Target != "Own/a.md" {
		t.Errorf("Target = %q, muốn Own/a.md", snap.Reached[0].Target)
	}
	// Lượt chở functionResponse KHÔNG phải một lượt của người: hai lượt người, không ba.
	if snap.HumanTurns != 2 {
		t.Errorf("HumanTurns = %d, muốn 2 — functionResponse bị tính là lời người", snap.HumanTurns)
	}
	if snap.Anchor != "tốt" {
		t.Errorf("Anchor = %q", snap.Anchor)
	}
}

// Cùng cái bẫy của `tool_result`, khác cái tên: Gemini chở kết quả tool dưới vai user. Chèn
// tiếng của lượt sau một lượt như thế là chen vào giữa cặp functionCall/functionResponse.
func TestGeminiHumanSpokeLast(t *testing.T) {
	human := geminiBody(t, `{"contents":[{"role":"user","parts":[{"text":"ừ"}]}]}`)
	if !human.HumanSpokeLast() {
		t.Error("lượt chữ thật phải tính là người vừa nói")
	}
	tool := geminiBody(t, `{"contents":[{"role":"user","parts":[{"functionResponse":{"name":"ls","response":{}}}]}]}`)
	if tool.HumanSpokeLast() {
		t.Error("lượt functionResponse KHÔNG phải người vừa nói")
	}
	model := geminiBody(t, `{"contents":[{"role":"model","parts":[{"text":"..."}]}]}`)
	if model.HumanSpokeLast() {
		t.Error("lượt model không phải người vừa nói")
	}
}

// Một phần tử `tools` chở NHIỀU món. Đọc một tầng như hai nhà kia thì mọi tên đều rỗng, và
// Outfitter không có gì để nói.
func TestGeminiToolInfosFlattensDeclarations(t *testing.T) {
	b := geminiBody(t, `{"tools":[{"functionDeclarations":[
		{"name":"read_file","description":"đọc một file"},
		{"name":"run_shell_command","description":"chạy lệnh"}
	]}],"contents":[]}`)
	got := b.ToolInfos()
	if len(got) != 2 {
		t.Fatalf("phải rút được 2 món, got %d: %+v", len(got), got)
	}
	if got[0].Name != "read_file" || got[1].Name != "run_shell_command" {
		t.Errorf("tên sai: %+v", got)
	}
	if got[0].Description != "đọc một file" {
		t.Errorf("hướng dẫn bị mất: %q", got[0].Description)
	}
	if !b.hasTools() {
		t.Error("hasTools phải thấy danh mục — AgentTurnShaped dựa vào nó")
	}
}

// Tiếng của lượt đi thành một Content MỚI ở cuối, vai `user` với hình `parts`. Vai
// `system` người gọi khai không dùng được ở tầng này: `contents` chỉ nhận user/model.
func TestGeminiAppendMessage(t *testing.T) {
	b := geminiBody(t, `{"contents":[{"role":"user","parts":[{"text":"chào"}]}]}`)
	b.AppendMessage(RoleSystem, "<Wayfarer>\nba mươi phút\n</Wayfarer>")

	var got struct {
		Contents []struct {
			Role  string `json:"role"`
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"contents"`
	}
	out, _ := b.Marshal()
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Contents) != 2 {
		t.Fatalf("phải có 2 content, got %d", len(got.Contents))
	}
	last := got.Contents[1]
	if last.Role != RoleUser {
		t.Errorf("vai = %q, phải quy về user", last.Role)
	}
	if len(last.Parts) != 1 || !strings.Contains(last.Parts[0].Text, "Wayfarer") {
		t.Errorf("parts sai: %+v", last.Parts)
	}
}

// Lossless: field lõi không đụng phải đi qua nguyên từng byte (#7). `generationConfig` và
// `safetySettings` là hai thứ Gemini CLI gửi mà lõi không có việc gì với chúng.
func TestGeminiMarshalKeepsUntouchedFields(t *testing.T) {
	raw := `{"contents":[{"role":"user","parts":[{"text":"chào"}]}],` +
		`"generationConfig":{"temperature":0.7,"thinkingConfig":{"thinkingBudget":-1}},` +
		`"safetySettings":[{"category":"HARM_CATEGORY_HARASSMENT","threshold":"BLOCK_NONE"}]}`
	b := geminiBody(t, raw)
	b.AppendSystem("đất")
	out, err := b.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"thinkingBudget":-1`,
		`"temperature":0.7`,
		`"HARM_CATEGORY_HARASSMENT"`,
		`"BLOCK_NONE"`,
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("mất %s trong: %s", want, out)
		}
	}
}

// Hai cửa đầu của lượt việc nhà phải đọc được hình Gemini. Cửa một đo lời dạy qua SystemBlocks,
// cửa hai đo hội thoại qua `contents` — cả hai đều là chỗ tên field khác nhau.
func TestGeminiShapedGates(t *testing.T) {
	rules := FrameRules{Identity: []string{"You are "}}

	chore := geminiBody(t, `{"contents":[{"role":"user","parts":[{"text":"đặt tiêu đề"}]}]}`)
	if chore.AgentTurnShaped(rules) {
		t.Error("không lời dạy, không tools → không phải lượt của đệ")
	}

	turn := geminiBody(t, `{"systemInstruction":{"parts":[{"text":"Hướng dẫn dùng đồ nghề."}]},
		"contents":[{"role":"user","parts":[{"text":"chào"}]},{"role":"model","parts":[{"text":"ừ"}]}]}`)
	if !turn.AgentTurnShaped(rules) {
		t.Error("có lời dạy → phải là lượt của đệ")
	}
	if !turn.ConversationShaped() {
		t.Error("hai lượt hội thoại → phải có hình hội thoại")
	}

	// Cửa một trừ đúng cái đã khai: khối chỉ có lời khẳng định căn
	// cước không phải lời dạy.
	ident := geminiBody(t, `{"systemInstruction":{"parts":[{"text":"You are Gemini CLI."}]},"contents":[]}`)
	if ident.AgentTurnShaped(rules) {
		t.Error("khối chỉ có câu khẳng định vai thì không tính là lời dạy")
	}
}

// Đo được trên lượt thật: Gemini CLI gửi TRỌN sách hướng dẫn trong MỘT part 25.257 ký tự mở
// đầu bằng "You are ", với 16 tiêu đề Markdown. Bỏ cả khối là đệ
// mất sạch `# Tool Usage`, `# Primary Workflows`, `# Security and Safety Rules` — đúng cái
// đánh đổi § Format wire đã bác: "Bỏ 17.427 để gỡ 150 không phải đánh đổi."
func TestPrefixDropSparesBlocksCarryingAManual(t *testing.T) {
	rules := FrameRules{Identity: []string{"You are "}, Sections: []string{"Tone and Style"}}
	manual := "You are an interactive CLI agent specializing in software engineering tasks.\n\n" +
		"# Core Mandates\ntuân thủ quy ước của repo\n\n" +
		"## Tone and Style (CLI Interaction)\nngắn gọn\n\n" +
		"## Tool Usage\ndùng replace để sửa file\n"
	b := geminiBody(t, `{"systemInstruction":{"parts":[{"text":`+mustQuote(manual)+`}]},"contents":[]}`)

	b.StripFrame(b.HostFrame(rules))
	got := b.SystemText()

	if got == "" {
		t.Fatal("bỏ cả khối sách hướng dẫn — bẻ tay đệ")
	}
	if !strings.Contains(got, "Tool Usage") || !strings.Contains(got, "Core Mandates") {
		t.Errorf("mục dạy dùng đồ nghề phải còn:\n%s", got)
	}
	// Mục đã khai thì vẫn phải cắt: gỡ câu căn cước không được làm `strip_sections` mất việc.
	if strings.Contains(got, "Tone and Style") {
		t.Errorf("mục đã khai vẫn phải bị cắt:\n%s", got)
	}
	// Và một khối mang sách thì PHẢI tính là có lời dạy — cùng một định nghĩa, chỗ đọc cũng đúng.
	if !b.AgentTurnShaped(rules) {
		t.Error("khối mang sách hướng dẫn phải tính là lời dạy")
	}
}

// Khối căn cước THẬT — không tiêu đề nào — vẫn phải bị bỏ cả khối, đúng như trước. Siết luật
// mà làm mất tác dụng gốc của nó thì là đổi một bug lấy một bug.
func TestPrefixDropStillDropsHeadlessIdentityBlock(t *testing.T) {
	rules := FrameRules{Identity: []string{"You are "}}
	b := geminiBody(t, `{"systemInstruction":{"parts":[{"text":"You are an interactive agent that helps users with software engineering tasks."}]},"contents":[]}`)
	b.StripFrame(b.HostFrame(rules))
	if got := b.SystemText(); got != "" {
		t.Errorf("khối căn cước không tiêu đề vẫn phải bỏ cả khối, còn lại: %q", got)
	}
}

// Đo được trên lượt thật: Gemini CLI gửi `tools: [{"name": ""}]` — một nhóm, KHÔNG món nào.
// Đếm phần tử là đếm cái vỏ, và tay trắng hoá tay đầy.
func TestGeminiHasToolsCountsToolsNotGroups(t *testing.T) {
	empty := geminiBody(t, `{"tools":[{"name":""}],"contents":[]}`)
	if empty.hasTools() {
		t.Error("một nhóm không món nào thì tay vẫn trắng")
	}
	declared := geminiBody(t, `{"tools":[{"name":"","functionDeclarations":[{"name":"read_file"}]}],"contents":[]}`)
	if !declared.hasTools() {
		t.Error("có functionDeclarations thì phải là có món")
	}
	// Tool sẵn của nhà khai bằng SỰ CÓ MẶT của khoá, giá trị là object cấu hình rỗng.
	builtin := geminiBody(t, `{"tools":[{"googleSearch":{}}],"contents":[]}`)
	if !builtin.hasTools() {
		t.Error("googleSearch là một món, dù cấu hình rỗng")
	}
	if geminiBody(t, `{"tools":[],"contents":[]}`).hasTools() {
		t.Error("mảng rỗng thì không có món")
	}
}

// Cắt khung trên đường Gemini: từng part một, giữ hình part. Cắt sạch không còn part nào thì
// BỎ HẲN field — `parts: []` là thứ nhà cung cấp từ chối.
func TestGeminiStripFrame(t *testing.T) {
	rules := FrameRules{Identity: []string{"You are "}, Sections: []string{"Tone and style"}}
	b := geminiBody(t, `{"systemInstruction":{"parts":[
		{"text":"You are Gemini CLI, bỏ cả khối này."},
		{"text":"# Using your tools\ngiữ\n\n# Tone and style\ncắt mục này"}
	]},"contents":[]}`)

	f := b.HostFrame(rules)
	if !f.Present {
		t.Fatal("phải thấy khung")
	}
	if !b.StripFrame(f) {
		t.Fatal("phải cắt được")
	}
	got := b.SystemText()
	if strings.Contains(got, "You are Gemini CLI") {
		t.Error("khối căn cước chưa bị bỏ")
	}
	if strings.Contains(got, "Tone and style") {
		t.Error("mục Tone and style chưa bị cắt")
	}
	if !strings.Contains(got, "Using your tools") {
		t.Error("cắt lố — mục dạy dùng đồ nghề là tay của đệ, phải giữ")
	}
}

func TestGeminiStripFrameDropsFieldWhenEmptied(t *testing.T) {
	rules := FrameRules{Identity: []string{"You are "}}
	b := geminiBody(t, `{"systemInstruction":{"parts":[{"text":"You are Gemini CLI."}]},"contents":[]}`)
	b.StripFrame(b.HostFrame(rules))
	out, _ := b.Marshal()
	if strings.Contains(string(out), "systemInstruction") {
		t.Errorf("cắt sạch thì phải bỏ hẳn field, không để `parts: []`: %s", out)
	}
}

// Nguyên văn phần đầu khối của Gemini CLI: một câu khẳng định vai, một câu vận hành, rồi sách
// bắt đầu ở `# Core Mandates`. Chỉ câu đầu bị gỡ.
const geminiPreamble = "You are Gemini CLI, an interactive CLI agent specializing in software " +
	"engineering tasks. You are currently operating in **Default** mode. Your primary goal is " +
	"to help users safely and effectively."

func TestGeminiIdentityClaimRemovedManualKept(t *testing.T) {
	rules := FrameRules{Identity: []string{"You are "}}
	manual := geminiPreamble + "\n\n# Core Mandates\ntuân thủ quy ước\n\n## Tool Usage\ndùng replace\n"
	b := geminiBody(t, `{"systemInstruction":{"parts":[{"text":`+mustQuote(manual)+`}]},"contents":[]}`)

	if !b.StripFrame(b.HostFrame(rules)) {
		t.Fatal("phải cắt được — phần đầu khối là thứ có thể cắt")
	}
	got := b.SystemText()
	if strings.Contains(got, "You are Gemini CLI") {
		t.Errorf("lời khẳng định vai chưa bị gỡ:\n%s", got)
	}
	// Câu vận hành cùng dòng KHÔNG phải lời khẳng định vai, nên nó phải sống.
	if !strings.Contains(got, "Your primary goal is to help users") {
		t.Errorf("gỡ lố sang câu vận hành:\n%s", got)
	}
	for _, want := range []string{"Core Mandates", "Tool Usage", "dùng replace"} {
		if !strings.Contains(got, want) {
			t.Errorf("mất %q — sách phải còn nguyên", want)
		}
	}
}

// Ca thật đã lọt CẢ HAI cửa cũ: Gemini CLI gọi một "Task Routing AI" chấm điểm phức tạp. Nó mang
// 2.175 ký tự lời dạy CÓ tiêu đề (`# Complexity Rubric`) nên AgentTurnShaped cho qua, và mang
// trọn 9 lượt hội thoại nên ConversationShaped cũng cho qua. Giá thật: ~58KB đất chèn vào một
// lượt chỉ trả về `{"complexity_score": N}`, cộng một phiên rác trong sổ.
func TestMachineReplyCatchesStructuredHousekeeping(t *testing.T) {
	rules := FrameRules{Identity: []string{"You are "}}
	routing := geminiBody(t, `{
		"systemInstruction":{"parts":[{"text":"\nYou are a specialized Task Routing AI. Your sole function is to analyze the user's request.\n\n# Complexity Rubric\n**1-20: Trivial**\n*   Simple, read-only\n"}]},
		"contents":[
			{"role":"user","parts":[{"text":"câu một"}]},
			{"role":"model","parts":[{"text":"đáp một"}]},
			{"role":"user","parts":[{"text":"câu hai"}]}
		],
		"generationConfig":{"responseMimeType":"application/json","responseJsonSchema":{"type":"OBJECT","properties":{"complexity_score":{"type":"INTEGER"}}}}
	}`)

	// Hai cửa đầu THẬT SỰ cho qua ca này: đó là việc của cửa ba, không phải vì chúng hỏng.
	if !routing.AgentTurnShaped(rules) || !routing.ConversationShaped() {
		t.Fatal("tiền đề đổi: hai cửa đầu đã tự bắt được, cửa ba không còn cần")
	}
	if !routing.MachineReply() {
		t.Error("lượt xin về object khớp schema phải bị cửa ba chặn")
	}

	// Lượt thật của đệ thì không xin cấu trúc, và phải đi qua được cả ba cửa.
	real := geminiBody(t, `{
		"systemInstruction":{"parts":[{"text":"You are Gemini CLI.\n\n# Core Mandates\ntuân thủ"}]},
		"contents":[{"role":"user","parts":[{"text":"chào"}]},{"role":"model","parts":[{"text":"ừ"}]}],
		"tools":[{"functionDeclarations":[{"name":"read_file"}]}],
		"generationConfig":{"temperature":1}
	}`)
	if real.MachineReply() {
		t.Error("lượt thật không xin cấu trúc, không được chặn")
	}
	if !real.AgentTurnShaped(rules) || !real.ConversationShaped() {
		t.Error("lượt thật phải qua được hai cửa đầu")
	}
}

// Ba đường khai cấu trúc của Gemini, và cửa phải bắt cả ba. `text/plain` là mặc định, không tính.
func TestMachineReplyReadsAllGeminiSignals(t *testing.T) {
	cases := map[string]bool{
		`{"generationConfig":{"responseMimeType":"application/json"}}`:  true,
		`{"generationConfig":{"responseSchema":{"type":"OBJECT"}}}`:     true,
		`{"generationConfig":{"responseJsonSchema":{"type":"OBJECT"}}}`: true,
		`{"generationConfig":{"responseMimeType":"text/plain"}}`:        false,
		`{"generationConfig":{"temperature":1,"maxOutputTokens":8192}}`: false,
		`{"contents":[]}`: false,
	}
	for body, want := range cases {
		if got := geminiBody(t, body).MachineReply(); got != want {
			t.Errorf("%s → %v, muốn %v", body, got, want)
		}
	}
}

// Đường Anthropic không có field nào cho việc này (nó ép cấu trúc bằng tool_choice), nên không
// đoán thay họ. Đường OpenAI thì có `response_format`.
func TestMachineReplyPerFormat(t *testing.T) {
	anth, _ := ParseBody([]byte(`{"system":"x","messages":[]}`), FormatAnthropic)
	if anth.MachineReply() {
		t.Error("Anthropic không có field khai cấu trúc — không được đoán")
	}
	for _, tc := range []struct {
		body string
		want bool
	}{
		{`{"response_format":{"type":"json_schema"}}`, true},
		{`{"response_format":{"type":"json_object"}}`, true},
		{`{"response_format":{"type":"text"}}`, false},
		{`{"messages":[]}`, false},
	} {
		b, _ := ParseBody([]byte(tc.body), FormatOpenAI)
		if got := b.MachineReply(); got != tc.want {
			t.Errorf("openai %s → %v, muốn %v", tc.body, got, tc.want)
		}
	}
}

// Bản đọc của nhật ký đọc `content`, mà Gemini chở chữ ở `parts` — đọc thẳng thì mọi lượt
// ra rỗng và phiên không có lượt người nào. Đo được trên lượt thật: 15 message, tất cả mù.
func TestGeminiHumanTextsIsNotBlind(t *testing.T) {
	b := geminiBody(t, `{"contents":[
		{"role":"user","parts":[{"text":"đọc file đi"}]},
		{"role":"model","parts":[{"functionCall":{"name":"read_file","args":{"path":"Own/a.md"}}}]},
		{"role":"user","parts":[{"functionResponse":{"name":"read_file","response":{"x":1}}}]},
		{"role":"model","parts":[{"text":"xong"}]},
		{"role":"user","parts":[{"text":"tiếp đi"}]}
	]}`)
	// Hai lượt người, không bốn: lượt `model` không mở lượt, và lượt chở functionResponse
	// KHÔNG phải lời người — cùng luật với tool_result của Anthropic.
	got := b.HumanTexts(nil, FrameRules{})
	if len(got) != 2 {
		t.Fatalf("muốn 2 lượt người, got %d: %q", len(got), got)
	}
	if got[0] != "đọc file đi" || got[1] != "tiếp đi" {
		t.Errorf("chữ của lượt người bị mất: %q", got)
	}
}
