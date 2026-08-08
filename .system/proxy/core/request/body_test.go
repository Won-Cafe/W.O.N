// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package request

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func mustParse(t *testing.T, s string) *Body {
	t.Helper()
	b, err := ParseBody([]byte(s), FormatAnthropic)
	if err != nil {
		t.Fatalf("ParseBody: %v", err)
	}
	return b
}

// mustParseFmt — parse với format tường minh (cho test OpenAI).
func mustParseFmt(t *testing.T, s string, fmt Format) *Body {
	t.Helper()
	b, err := ParseBody([]byte(s), fmt)
	if err != nil {
		t.Fatalf("ParseBody: %v", err)
	}
	return b
}

func topFields(t *testing.T, b *Body) map[string]json.RawMessage {
	t.Helper()
	out, err := b.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	m := map[string]json.RawMessage{}
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	return m
}

func TestParseBodyInvalid(t *testing.T) {
	if _, err := ParseBody([]byte("not json"), FormatAnthropic); err == nil {
		t.Fatal("want parse error, got nil")
	}
}

// Bất biến #7: field không bị đụng giữ nguyên từng byte — số lớn hơn 2^53,
// định dạng số lạ, field chưa biết đều sống sót qua parse → marshal.
func TestLosslessUntouchedFields(t *testing.T) {
	meta := `{"big":9007199254740993,"f":1.10,"odd":"kept"}`
	b := mustParse(t, `{"model":"m","metadata":`+meta+`,"messages":[{"role":"user","content":"hi"}]}`)
	got := topFields(t, b)["metadata"]
	if !bytes.Equal(got, []byte(meta)) {
		t.Fatalf("metadata bytes changed:\nwant %s\ngot %s", meta, got)
	}
}

// Tiếng của lượt là một message MỚI ở cuối — không nối vào message nào đang có. Lượt
// người và mọi message trước nó đi qua nguyên TỪNG BYTE (#1): đây là chỗ khoá lại rằng
// lõi không còn đường nào sửa một message của người.
func TestAppendMessageLeavesEveryOtherMessageByteIdentical(t *testing.T) {
	asst := `{"role":"assistant","content":[{"type":"tool_use","id":"t1","input":{"n":9007199254740993}}]}`
	user := `{"role":"user","content":"hi"}`
	b := mustParse(t, `{"messages":[`+asst+`,`+user+`]}`)
	b.AppendMessage(RoleUser, "🛣️ wayfarer: milestone")

	var msgs []json.RawMessage
	if err := json.Unmarshal(topFields(t, b)["messages"], &msgs); err != nil {
		t.Fatalf("messages: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("phải thành 3 message, got %d", len(msgs))
	}
	if !bytes.Equal(msgs[0], []byte(asst)) {
		t.Fatalf("lời assistant bị sửa: %s", msgs[0])
	}
	if !bytes.Equal(msgs[1], []byte(user)) {
		t.Fatalf("lời người bị sửa: %s", msgs[1])
	}
	if !strings.Contains(string(msgs[2]), "wayfarer: milestone") {
		t.Fatalf("tiếng của lượt không tới được cuối mảng: %s", msgs[2])
	}
}

// Lượt người dạng mảng block cũng không bị chạm: content của nó giữ đúng số block.
func TestAppendMessageDoesNotEnterBlockContent(t *testing.T) {
	b := mustParse(t, `{"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`)
	b.AppendMessage(RoleUser, "tiếng của lượt")
	var msgs []json.RawMessage
	_ = json.Unmarshal(topFields(t, b)["messages"], &msgs)
	if len(msgs) != 2 {
		t.Fatalf("phải thành 2 message, got %d", len(msgs))
	}
	var m struct{ Content []struct{ Text string } }
	if err := json.Unmarshal(msgs[0], &m); err != nil || len(m.Content) != 1 {
		t.Fatalf("content của lượt người bị thêm block: %s (err=%v)", msgs[0], err)
	}
}

// Chỉ có đường chèn vào CUỐI: chèn hai lần thì thứ tự chèn là thứ tự đọc, và chữ
// gốc của công cụ chủ luôn đứng TRƯỚC. Đứng sau là chỗ được đọc gần lúc trả lời nhất.
func TestSystemSplice(t *testing.T) {
	b := mustParse(t, `{"system":"root"}`)
	b.AppendSystem("đất")
	b.AppendSystem("soul")
	if got := b.SystemText(); got != "root\n\nđất\n\nsoul" {
		t.Fatalf("system string: %q", got)
	}

	b = mustParse(t, `{"system":[{"type":"text","text":"root"}]}`)
	b.AppendSystem("đất")
	b.AppendSystem("soul")
	var blocks []json.RawMessage
	if err := json.Unmarshal(topFields(t, b)["system"], &blocks); err != nil || len(blocks) != 3 {
		t.Fatalf("system array: want 3 blocks, got %d (err=%v)", len(blocks), err)
	}
	var first struct{ Text string }
	_ = json.Unmarshal(blocks[0], &first)
	if first.Text != "root" {
		t.Errorf("khối của công cụ chủ phải đứng đầu, got %q", first.Text)
	}

	b = mustParse(t, `{}`)
	b.AppendSystem("new")
	if got := b.SystemText(); got != "new" {
		t.Fatalf("system missing: %q", got)
	}
}

// Danh mục tool đi qua NGUYÊN VẸN. Lõi đọc nó (ToolInfos) để agent bờ nói được
// về đồ nghề, nhưng không có đường nào sửa nó: đổi `tools` giữa phiên làm mọi
// tầng cache của nhà cung cấp dựng lại. Test này khoá cái không-có-đường.
func TestToolsReadOnly(t *testing.T) {
	tools := `[{"name":"a","input_schema":{"max":9007199254740993}},{"name":"b"}]`
	b := mustParse(t, `{"tools":`+tools+`}`)

	_ = b.ToolInfos()
	b.AppendSystem("đất")
	b.AppendMessage(RoleUser, "một tiếng bên lề")

	if got := topFields(t, b)["tools"]; !bytes.Equal(got, []byte(tools)) {
		t.Fatalf("danh mục tool bị chạm:\nwant %s\ngot  %s", tools, got)
	}
	if got := b.ToolInfos(); len(got) != 2 || got[0].Name != "a" || got[1].Name != "b" {
		t.Fatalf("ToolInfos: %v", got)
	}
}

func TestSnapshotAnchors(t *testing.T) {
	b := mustParse(t, `{"model":"m","messages":[
		{"role":"user","content":"greeting"},
		{"role":"assistant","content":[{"type":"text","text":"reply"}]},
		{"role":"user","content":"turn two"}]}`)
	snap := b.Snapshot(FrameRules{})
	if snap.FirstUser != "greeting" || snap.FirstAssistant != "reply" {
		t.Fatalf("anchors wrong: user=%q assistant=%q", snap.FirstUser, snap.FirstAssistant)
	}
	if got := snap.Anchor; got != "turn two" {
		t.Fatalf("Anchor: %q", got)
	}
}

// ===== OpenAI song song =====
// FormatOpenAI quyết từ router, không sniff body (#6) — test gọi thẳng ParseBody.

// --- SystemText (case 1-6) ---

func TestOpenAISystemText(t *testing.T) {
	cases := []struct {
		name string
		json string
		want string
	}{
		// 1: không có system message → rỗng.
		{"no-system", `{"messages":[{"role":"user","content":"hi"}]}`, ""},
		// 2: một system message dạng string.
		{"string", `{"messages":[{"role":"system","content":"you are X"}]}`, "you are X"},
		// 3: nhiều system message → nối \n\n theo thứ tự, không gộp.
		{"many-string", `{"messages":[{"role":"system","content":"a"},{"role":"system","content":"b"}]}`, "a\n\nb"},
		// 4: system dạng mảng text block.
		{"blocks", `{"messages":[{"role":"system","content":[{"type":"text","text":"x"},{"type":"text","text":"y"}]}]}`, "x\ny"},
		// 5: system message xen giữa user — vẫn lấy, không sửa thứ tự.
		{"interleaved", `{"messages":[{"role":"system","content":"s1"},{"role":"user","content":"q"},{"role":"system","content":"s2"}]}`, "s1\n\ns2"},
		// 6: field top-level "system" (kiểu Anthropic) trong OpenAI → bỏ qua,
		// chỉ quét messages.
		{"anthropic-field-ignored", `{"system":"STALE","messages":[{"role":"system","content":"REAL"}]}`, "REAL"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b := mustParseFmt(t, c.json, FormatOpenAI)
			if got := b.SystemText(); got != c.want {
				t.Fatalf("SystemText: want %q got %q", c.want, got)
			}
		})
	}
}

// --- spliceSystem (case 7-12) ---

func TestOpenAISpliceSystemPrependString(t *testing.T) {
	// 7: Append → message system MỚI SAU message system đang có; content của
	// message cũ nguyên từng byte.
	root := `{"role":"system","content":"root"}`
	b := mustParseFmt(t, `{"messages":[`+root+`,{"role":"user","content":"hi"}]}`, FormatOpenAI)
	b.AppendSystem("head")
	if got := b.SystemText(); got != "root\n\nhead" {
		t.Fatalf("append string: %q", got)
	}
	var msgs []json.RawMessage
	_ = json.Unmarshal(topFields(t, b)["messages"], &msgs)
	if len(msgs) != 3 {
		t.Fatalf("want 3 messages, got %d", len(msgs))
	}
	if !bytes.Equal(msgs[0], []byte(root)) {
		t.Fatalf("message cũ bị chạm: %s", msgs[0])
	}
}

func TestOpenAIAppendSystemStaysInSystemRegion(t *testing.T) {
	// 8: vùng system là DÃY system ở ĐẦU mảng. Một message system nằm sau lượt
	// người thì không còn là lời hệ thống, nên khối mới không được xếp sau nó —
	// đi sau là lạc xuống giữa hội thoại. Dãy đầu rỗng → khối mới đứng đầu mảng.
	b := mustParseFmt(t, `{"messages":[{"role":"user","content":"hi"},{"role":"system","content":"root"}]}`, FormatOpenAI)
	b.AppendSystem("head")
	var msgs []struct{ Role, Content string }
	_ = json.Unmarshal(topFields(t, b)["messages"], &msgs)
	if len(msgs) != 3 || msgs[0].Role != "system" || msgs[0].Content != "head" || msgs[1].Role != "user" {
		t.Fatalf("khối mới phải ở đầu vùng system, không rơi xuống sau lượt người: %+v", msgs)
	}
}

func TestOpenAISpliceSystemPrependNoSystem(t *testing.T) {
	// 9: không có system message nào → chèn message system mới ở đầu mảng.
	b := mustParseFmt(t, `{"messages":[{"role":"user","content":"hi"}]}`, FormatOpenAI)
	b.AppendSystem("head")
	var msgs []struct {
		Role    string
		Content string
	}
	_ = json.Unmarshal(topFields(t, b)["messages"], &msgs)
	if len(msgs) != 2 || msgs[0].Role != "system" || msgs[0].Content != "head" {
		t.Fatalf("no-system: %+v", msgs)
	}
}

func TestOpenAIAppendSystemNoSystemGoesFirst(t *testing.T) {
	// 10: chưa có system message nào → khối mới đứng đầu mảng, trước lượt người.
	b := mustParseFmt(t, `{"messages":[{"role":"user","content":"hi"},{"role":"assistant","content":"r"}]}`, FormatOpenAI)
	b.AppendSystem("head")
	var msgs []struct {
		Role    string
		Content string
	}
	_ = json.Unmarshal(topFields(t, b)["messages"], &msgs)
	if len(msgs) != 3 || msgs[0].Role != "system" || msgs[0].Content != "head" || msgs[1].Role != "user" {
		t.Fatalf("khối mới phải đứng đầu: %+v", msgs)
	}
}

func TestOpenAISpliceSystemManyMessagesNoGobble(t *testing.T) {
	// 11: Nhiều system message — không gộp, không sửa content nào. Khối mới đứng
	// riêng ở CUỐI dãy system đầu mảng; ba message cũ giữ nguyên chỗ và nguyên chữ.
	// `s2` nằm sau lượt người nên nó ngoài vùng system, và khối mới không nhảy qua nó.
	b := mustParseFmt(t, `{"messages":[{"role":"system","content":"s1"},{"role":"user","content":"q"},{"role":"system","content":"s2"}]}`, FormatOpenAI)
	b.AppendSystem("HEAD")
	if got := b.SystemText(); got != "s1\n\nHEAD\n\ns2" {
		t.Fatalf("no-gobble: %q", got)
	}
	var msgs []struct {
		Role    string
		Content string
	}
	_ = json.Unmarshal(topFields(t, b)["messages"], &msgs)
	if len(msgs) != 4 {
		t.Fatalf("want 4 messages (3 cũ + 1 khối mới), got %d: %+v", len(msgs), msgs)
	}
	got := make([]string, len(msgs))
	for i, m := range msgs {
		got[i] = m.Role + ":" + m.Content
	}
	want := []string{"system:s1", "system:HEAD", "user:q", "system:s2"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("thứ tự message sai:\nwant %v\ngot  %v", want, got)
		}
	}
}

func TestOpenAISpliceSystemBlocks(t *testing.T) {
	// 12: system content dạng mảng block — message cũ không bị chạm một byte,
	// khối mới là message riêng.
	root := `{"role":"system","content":[{"type":"text","text":"root"}]}`
	b := mustParseFmt(t, `{"messages":[`+root+`]}`, FormatOpenAI)
	b.AppendSystem("head")
	var msgs []json.RawMessage
	_ = json.Unmarshal(topFields(t, b)["messages"], &msgs)
	if len(msgs) != 2 {
		t.Fatalf("want 2 messages, got %d", len(msgs))
	}
	if !bytes.Equal(msgs[0], []byte(root)) {
		t.Fatalf("message dạng block bị chạm: %s", msgs[0])
	}
	if got := b.SystemText(); got != "root\n\nhead" {
		t.Fatalf("SystemText: %q", got)
	}
}

// --- ToolInfos / nameOf theo nhánh format ---

func TestOpenAIToolNames(t *testing.T) {
	// OpenAI: tên tool nằm trong function.name.
	b := mustParseFmt(t, `{"tools":[{"type":"function","function":{"name":"a","description":"x"}},{"type":"function","function":{"name":"b"}}]}`, FormatOpenAI)
	got := b.ToolInfos()
	if len(got) != 2 || got[0].Name != "a" || got[1].Name != "b" {
		t.Fatalf("ToolInfos: %v", got)
	}
	if got[0].Description != "x" {
		t.Fatalf("hướng dẫn phải đọc cùng nhánh với tên: %q", got[0].Description)
	}
}

func TestOpenAIToolNamesTopLevelIgnored(t *testing.T) {
	// Field top-level "name" (kiểu Anthropic) trong OpenAI → bỏ qua.
	b := mustParseFmt(t, `{"tools":[{"name":"STALE","function":{"name":"real"}}]}`, FormatOpenAI)
	got := b.ToolInfos()
	if len(got) != 1 || got[0].Name != "real" {
		t.Fatalf("want real only, got %v", got)
	}
}

func TestOpenAIToolNamesEmptyWhenUnknown(t *testing.T) {
	// Unknown format → không đoán schema → rỗng.
	b := mustParseFmt(t, `{"tools":[{"name":"a"}]}`, FormatUnknown)
	if got := b.ToolInfos(); len(got) != 0 {
		t.Fatalf("Unknown: want empty, got %v", got)
	}
}

// --- Snapshot (case 19-21) ---

func TestOpenAISnapshotAnchors(t *testing.T) {
	// 19: anchors từ message role user/assistant OpenAI.
	b := mustParseFmt(t, `{"model":"gpt","messages":[
		{"role":"system","content":"sys"},
		{"role":"user","content":"greeting"},
		{"role":"assistant","content":"reply"},
		{"role":"user","content":"turn two"}]}`, FormatOpenAI)
	snap := b.Snapshot(FrameRules{})
	if snap.System != "sys" || snap.FirstUser != "greeting" || snap.FirstAssistant != "reply" {
		t.Fatalf("anchors: sys=%q user=%q asst=%q", snap.System, snap.FirstUser, snap.FirstAssistant)
	}
	if got := snap.Anchor; got != "turn two" {
		t.Fatalf("Anchor: %q", got)
	}
}

// Mỏ neo sống sót qua cửa cắt cửa sổ. Turns cắt còn snapMaxTurns lượt cuối, mà
// giữa một vòng tool dài, lời người nằm cách đuôi hàng trăm message — cắt xong là
// mất. Đọc ra từ một phiên thật: 128 lượt máy liền nhau, câu người hỏi không có
// mặt trong prompt của một agent bờ nào.
func TestSnapshotKeepsAnchorBeyondWindow(t *testing.T) {
	var sb strings.Builder
	sb.WriteString(`{"messages":[{"role":"user","content":"viết lại README.md"}`)
	for i := 0; i < snapMaxTurns+5; i++ {
		sb.WriteString(`,{"role":"assistant","content":"giờ sửa tiếp"}`)
		sb.WriteString(`,{"role":"tool","tool_call_id":"c1","content":"kết quả"}`)
	}
	sb.WriteString("]}")
	snap := mustParseFmt(t, sb.String(), FormatOpenAI).Snapshot(FrameRules{})

	if snap.Anchor != "viết lại README.md" {
		t.Fatalf("mỏ neo mất khi cửa sổ cắt: %q", snap.Anchor)
	}
	for _, turn := range snap.Turns {
		if turn.Role == "user" {
			t.Fatalf("lượt người vốn đã rơi khỏi cửa sổ — nếu còn thì test không đo đúng thứ định đo: %+v", turn)
		}
	}
	// Người chưa nói lại: mỏ neo là câu cũ, không phải dấu hiệu người vừa lên tiếng.
	if snap.HumanSpokeLast {
		t.Error("giữa vòng tool, người chưa nói lại")
	}
}

func TestSnapshotSkipsRoleTool(t *testing.T) {
	// 20: role:tool = kết quả công cụ, không phải lượt — Snapshot bỏ qua (chốt #6).
	b := mustParse(t, `{"messages":[
		{"role":"user","content":"q"},
		{"role":"assistant","content":[{"type":"tool_use","id":"t1","input":{"n":1}}]},
		{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"result"}]},
		{"role":"assistant","content":"answer"}]}`)
	snap := b.Snapshot(FrameRules{})
	for _, turn := range snap.Turns {
		if turn.Role == "tool" || turn.Role == "function" {
			t.Fatalf("role:tool must be filtered: %+v", turn)
		}
	}
	if snap.FirstAssistant != "answer" {
		t.Fatalf("FirstAssistant: %q (want answer, tool_use text empty should not be anchor)", snap.FirstAssistant)
	}
}

func TestOpenAISnapshotSkipsRoleTool(t *testing.T) {
	// 21: OpenAI role:tool (kết quả function call) — Snapshot bỏ qua.
	b := mustParseFmt(t, `{"messages":[
		{"role":"user","content":"q"},
		{"role":"assistant","content":null,"tool_calls":[{"id":"c1","type":"function","function":{"name":"f"}}]},
		{"role":"tool","tool_call_id":"c1","content":"result"},
		{"role":"assistant","content":"answer"}]}`, FormatOpenAI)
	snap := b.Snapshot(FrameRules{})
	for _, turn := range snap.Turns {
		if turn.Role == "tool" || turn.Role == "function" {
			t.Fatalf("role:tool must be filtered: %+v", turn)
		}
	}
	if snap.FirstAssistant != "answer" {
		t.Fatalf("FirstAssistant: %q", snap.FirstAssistant)
	}
}

// --- Edge / regression ---

func TestParseBodyUnknownZeroValue(t *testing.T) {
	// Unknown là zero-value: không gọi Format gì thì Unknown.
	b, err := ParseBody([]byte(`{"messages":[]}`), FormatUnknown)
	if err != nil || b == nil {
		t.Fatalf("parse unknown: %v %v", b, err)
	}
	// Unknown: SystemText rỗng (không đoán Anthropic).
	if got := b.SystemText(); got != "" {
		t.Fatalf("Unknown SystemText must be empty, got %q", got)
	}
	// Unknown: nameOf rỗng.
	if got := b.ToolInfos(); len(got) != 0 {
		t.Fatalf("Unknown ToolInfos must be empty, got %v", got)
	}
}

func TestFormatFromPath(t *testing.T) {
	cases := []struct {
		path string
		fmt  Format
	}{
		{"/v1/messages", FormatAnthropic},
		{"/v1/chat/completions", FormatOpenAI},
		{"/something/else", FormatUnknown},
		{"/v1/messages/count", FormatUnknown}, // suffix phải khớp đúng
		{"/foo/chat/completions", FormatOpenAI},
		{"", FormatUnknown},
	}
	for _, c := range cases {
		if got := FormatFromPath(c.path); got != c.fmt {
			t.Fatalf("FormatFromPath(%q) = %v, want %v", c.path, got, c.fmt)
		}
	}
}

func TestFormatString(t *testing.T) {
	if FormatAnthropic.String() != "anthropic" || FormatOpenAI.String() != "openai" || FormatUnknown.String() != "unknown" {
		t.Fatalf("Format.String wrong: %s %s %s", FormatAnthropic, FormatOpenAI, FormatUnknown)
	}
}

func TestOpenAIParseMalformed(t *testing.T) {
	if _, err := ParseBody([]byte("not json"), FormatOpenAI); err == nil {
		t.Fatal("want parse error for OpenAI")
	}
}

func TestOpenAISystemTextEmptyContent(t *testing.T) {
	// system content rỗng / null → bỏ qua.
	b := mustParseFmt(t, `{"messages":[{"role":"system","content":""},{"role":"system","content":null}]}`, FormatOpenAI)
	if got := b.SystemText(); got != "" {
		t.Fatalf("empty system content: %q", got)
	}
}
