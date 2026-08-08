// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

// Package request giữ biểu diễn lossless của request dòng chính
// và bản sao chỉ-đọc (Snapshot) đưa cho plugin.
package request

import (
	"bytes"
	"encoding/json"
	"strings"
)

// Body bọc request dòng chính đã parse. Mỗi field giữ nguyên byte (json.RawMessage);
// lõi chỉ vật chất hoá đúng phần nó chạm (#7). Format quyết định nhánh logic —
// router gán từ path, không sniff body (#6). Zero-value Unknown ép tường minh.
type Body struct {
	fields map[string]json.RawMessage
	format Format
}

// Vai của message chèn thêm. Hai giá trị, không ba — xem AppendMessage.
const (
	RoleUser   = "user"
	RoleSystem = "system"
	// RoleAssistant — tên CHUNG của lõi cho lượt model trả lời. Gemini gọi nó là `model`;
	// normRole quy về đây, nên chỗ đọc không phải biết mỗi nhà đặt tên gì.
	RoleAssistant = "assistant"
)

// normRole quy vai về tên chung của lõi. Chỉ đường Gemini cần: `model` → `assistant`.
func (b *Body) normRole(role string) string {
	if b.format == FormatGemini && role == RoleModel {
		return RoleAssistant
	}
	return role
}

// roleOfMsg — vai của một item trong mảng hội thoại. Responses có item không mang `role`
// (function_call, function_call_output), nên phải suy từ `type` khi field `role` vắng.
func (b *Body) roleOfMsg(msg json.RawMessage) string {
	if b.format == FormatOpenAIResponses {
		return responsesRoleOf(msg)
	}
	return roleOf(msg)
}

func ParseBody(b []byte, format Format) (*Body, error) {
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(b, &fields); err != nil {
		return nil, err
	}
	return &Body{fields: fields, format: format}, nil
}

func (b *Body) Marshal() ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(b.fields); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// Format — nhánh giao thức của request. Chỗ nào phải rẽ theo luật riêng của nhà
// cung cấp thì đọc đây, không đoán từ hình dạng body (#6).
func (b *Body) Format() Format { return b.format }

func (b *Body) Model() string {
	var s string
	_ = json.Unmarshal(b.fields["model"], &s)
	return s
}

// SystemText — Anthropic: field top-level "system" — string hoặc mảng block.
// OpenAI: quét messages, lọc role=="system", flatten. Không có → rỗng.
func (b *Body) SystemText() string {
	switch b.format {
	case FormatOpenAI:
		return b.systemTextOpenAI()
	case FormatGemini:
		return b.systemTextGemini()
	case FormatOpenAIResponses:
		return b.systemTextResponses()
	}
	// Anthropic (và Unknown — không đoán, không chèn).
	raw, ok := b.fields["system"]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var blocks []json.RawMessage
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}
	var parts []string
	for _, blk := range blocks {
		var m struct{ Text string }
		if err := json.Unmarshal(blk, &m); err != nil {
			continue
		}
		if m.Text != "" {
			parts = append(parts, m.Text)
		}
	}
	return strings.Join(parts, "\n\n")
}

func (b *Body) systemTextOpenAI() string {
	var msgs []json.RawMessage
	if err := json.Unmarshal(b.fields["messages"], &msgs); err != nil {
		return ""
	}
	var parts []string
	for _, msg := range msgs {
		if roleOf(msg) != RoleSystem {
			continue
		}
		var m struct {
			Content json.RawMessage
		}
		if err := json.Unmarshal(msg, &m); err != nil {
			continue
		}
		if t := flattenContent(m.Content); t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, "\n\n")
}

// SystemBlocks — chữ từng khối lời hệ thống, cho nhật ký thấy đường may.
// OpenAI: đọc vùng system ở đầu mảng messages, dừng ở lượt hội thoại đầu tiên.
func (b *Body) SystemBlocks() []string {
	// Gemini: từng `part` của systemInstruction là một khối. Phải đúng ở đây, vì
	// AgentTurnShaped đo "còn lời dạy nào" bằng chính hàm này — rỗng thì mọi lượt của Gemini
	// CLI bị tính là lượt việc nhà và không lượt nào được chèn đất.
	if b.format == FormatGemini {
		_, raw := b.geminiSystemField()
		return b.systemPartsGemini(raw)
	}
	if b.format == FormatOpenAIResponses {
		return b.systemPartsResponses()
	}
	if b.format == FormatOpenAI {
		var out []string
		for _, msg := range b.messages() {
			if roleOf(msg) != RoleSystem {
				break
			}
			var m struct{ Content json.RawMessage }
			if err := json.Unmarshal(msg, &m); err != nil {
				continue
			}
			if t := contentText(m.Content); t != "" {
				out = append(out, t)
			}
		}
		return out
	}
	blocks, ok := b.systemBlocks()
	if !ok {
		return nil
	}
	out := make([]string, 0, len(blocks))
	for _, blk := range blocks {
		var m struct{ Text string }
		if err := json.Unmarshal(blk, &m); err == nil && m.Text != "" {
			out = append(out, m.Text)
		}
	}
	return out
}

// contentText làm phẳng content: chuỗi thì lấy nguyên, dạng khác thì JSON thô.
func contentText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return string(raw)
}

// AppendSystem đặt text thành một khối system MỚI ở CUỐI vùng lời hệ thống, sau mọi khối
// công cụ chủ. Khối RIÊNG, không trộn: bên nhận thấy đường may, chữ client không bị chạm.
// Vì sao SAU, và cái giá cache của nó: § Format wire.
//
// Anthropic: system đổi sang mảng block (chính sách). OpenAI: message role:system mới,
// đặt CUỐI vùng system ở đầu mảng messages. Gemini: một `part` mới ở cuối
// `systemInstruction.parts`. Unknown: không chạm (#6).
func (b *Body) AppendSystem(text string) {
	switch b.format {
	case FormatOpenAI:
		b.appendSystemOpenAI(text)
	case FormatAnthropic:
		b.appendSystemAnthropic(text)
	case FormatGemini:
		b.appendSystemGemini(text)
	case FormatOpenAIResponses:
		b.appendSystemResponses(text)
	}
}

func (b *Body) appendSystemAnthropic(text string) {
	blocks, ok := b.systemBlocks()
	if !ok {
		return // dạng lạ: thà không chèn còn hơn làm mất chữ
	}
	if out := mustJSON(append(blocks, textBlock(text))); out != nil {
		b.fields["system"] = out
	}
}

// appendSystemOpenAI — cuối VÙNG system, không cuối mảng: lời hệ thống của đường
// OpenAI là các message `system` ở đầu mảng, và một message `system` đặt sau lượt hội
// thoại đầu tiên thì không còn là lời hệ thống nữa.
func (b *Body) appendSystemOpenAI(text string) {
	var msgs []json.RawMessage
	if err := json.Unmarshal(b.fields["messages"], &msgs); err != nil {
		return
	}
	nm := mustJSON(map[string]any{"role": RoleSystem, "content": text})
	if nm == nil {
		return
	}
	at := 0
	for at < len(msgs) && roleOf(msgs[at]) == RoleSystem {
		at++
	}
	if out := mustJSON(insertAt(msgs, at, nm)); out != nil {
		b.fields["messages"] = out
	}
}

// systemBlocks — chuỗi → một block bọc nguyên chữ; vắng → mảng rỗng; dạng lạ →
// ok=false và người gọi không chạm.
func (b *Body) systemBlocks() ([]json.RawMessage, bool) {
	raw, ok := b.fields["system"]
	if !ok || len(raw) == 0 {
		return nil, true
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if strings.TrimSpace(s) == "" {
			return nil, true
		}
		return []json.RawMessage{textBlock(s)}, true
	}
	var blocks []json.RawMessage
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil, false
	}
	return blocks, true
}

func insertAt(msgs []json.RawMessage, at int, nm json.RawMessage) []json.RawMessage {
	out := make([]json.RawMessage, 0, len(msgs)+1)
	out = append(out, msgs[:at]...)
	out = append(out, nm)
	return append(out, msgs[at:]...)
}

// replaceAt thay message thứ i bằng bản đã sửa, giữ nguyên các message khác.
func replaceAt(msgs []json.RawMessage, i int, nm json.RawMessage) []json.RawMessage {
	out := make([]json.RawMessage, len(msgs))
	copy(out, msgs)
	out[i] = nm
	return out
}

// ReplaceSystem thay thế toàn bộ system prompt — chính sách, không lossless:
// nhiều system message OpenAI được gộp thành một. Chỉ dùng cho StripFrame.
// Unknown → no-op (#6).
//
// Gemini KHÔNG có nhánh ở đây, và đó là chủ ý: `systemInstruction` là một mảng `parts`, nên
// đường cắt của nó (stripSystemFrameGemini) sửa được TỪNG part tại chỗ và không bao giờ cần
// gộp. Đây là đường chính sách duy nhất của lõi; format nào không cần thì không mở.
func (b *Body) ReplaceSystem(text string) {
	switch b.format {
	case FormatOpenAI:
		b.replaceSystemOpenAI(text)
	case FormatAnthropic:
		b.fields["system"] = mustJSON(text)
	case FormatOpenAIResponses:
		b.replaceSystemResponses(text)
	}
}

// replaceSystemOpenAI gộp mọi system message thành một ở vị trí đầu. Cắt hết
// chỉ còn rỗng → bỏ luôn message, không đẩy vỏ trống lên đích.
func (b *Body) replaceSystemOpenAI(text string) {
	var msgs []json.RawMessage
	if err := json.Unmarshal(b.fields["messages"], &msgs); err != nil {
		return
	}
	kept := make([]json.RawMessage, 0, len(msgs))
	placed := false
	for _, msg := range msgs {
		if roleOf(msg) != RoleSystem {
			kept = append(kept, msg)
			continue
		}
		if placed || strings.TrimSpace(text) == "" {
			continue
		}
		if nm := mustJSON(map[string]any{"role": RoleSystem, "content": text}); nm != nil {
			kept = append(kept, nm)
			placed = true
		}
	}
	if !placed && strings.TrimSpace(text) != "" {
		if nm := mustJSON(map[string]any{"role": RoleSystem, "content": text}); nm != nil {
			kept = append([]json.RawMessage{nm}, kept...)
		}
	}
	if nb := mustJSON(kept); nb != nil {
		b.fields["messages"] = nb
	}
}

// AppendMessage đặt chữ hệ thành message MỚI ở cuối, không vào trong lượt người
// (#1). role: user chạy mọi nhà, system là kênh vận hành (nhà cũ trả 400).
// assistant bị chặn — message cuối mang vai assistant là lệnh "viết tiếp", model
// sẽ nối vào lời agent bờ như lời của chính nó.
func (b *Body) AppendMessage(role, text string) {
	if text == "" {
		return
	}
	msgs, ok := b.messageList()
	if !ok {
		return
	}
	nm := b.coreMessage(role, text)
	if nm == nil {
		return
	}
	if nb := mustJSON(append(msgs, nm)); nb != nil {
		b.fields[b.messagesKey()] = nb
	}
}

// coreMessage dựng một message của lõi theo hình của format. Một chỗ dựng cho cả đường nối
// và đường xen (PlaceMessages): hai chỗ dựng là hai hình message có thể lệch nhau.
//
// Gemini: `contents` chỉ nhận `user` và `model`, không có kênh `system` nào ở tầng này, nên
// vai người gọi khai không dùng được và hình cũng khác (`parts`).
func (b *Body) coreMessage(role, text string) json.RawMessage {
	if b.format == FormatGemini {
		return mustJSON(map[string]any{
			"role":  RoleUser,
			"parts": []any{map[string]string{"text": text}},
		})
	}
	if b.format == FormatOpenAIResponses {
		return b.coreMessageResponses(role, text)
	}
	if role != RoleUser && role != RoleSystem {
		role = RoleUser
	}
	return mustJSON(map[string]any{"role": role, "content": text})
}

// ToolInfo — tên tool kèm hướng dẫn. Chỉ đọc — không có đường ghi.
type ToolInfo struct {
	Name        string
	Description string
}

// ToolInfos — Unknown → rỗng, không đoán schema (#6).
func (b *Body) ToolInfos() []ToolInfo {
	if b.format == FormatUnknown {
		return nil
	}
	// Gemini lồng thêm một tầng: `tools[].functionDeclarations[]`. Một phần tử chở nhiều món,
	// nên nó là hai vòng chứ không một — không rẽ được bằng nameOf/descriptionOf.
	if b.format == FormatGemini {
		return b.toolInfosGemini()
	}
	var tools []json.RawMessage
	if err := json.Unmarshal(b.fields["tools"], &tools); err != nil {
		return nil
	}
	var out []ToolInfo
	for _, t := range tools {
		n := b.nameOf(t)
		if n == "" {
			continue
		}
		out = append(out, ToolInfo{Name: n, Description: b.descriptionOf(t)})
	}
	return out
}

// nameOf / descriptionOf trích tên và hướng dẫn theo nhánh format. Unknown: rỗng.
func (b *Body) nameOf(tool json.RawMessage) string {
	switch b.format {
	case FormatOpenAI:
		var m struct {
			Function struct{ Name string }
		}
		_ = json.Unmarshal(tool, &m)
		return m.Function.Name
	case FormatAnthropic, FormatOpenAIResponses:
		var m struct{ Name string }
		_ = json.Unmarshal(tool, &m)
		return m.Name
	default:
		return ""
	}
}

func (b *Body) descriptionOf(tool json.RawMessage) string {
	if b.format == FormatOpenAI {
		var t struct {
			Function struct {
				Description string `json:"description"`
			} `json:"function"`
		}
		_ = json.Unmarshal(tool, &t)
		return t.Function.Description
	}
	var t struct {
		Description string `json:"description"`
	}
	_ = json.Unmarshal(tool, &t)
	return t.Description
}

// ── Bên đọc hội thoại, rẽ theo format ────────────────────────────────────────────
// Bốn hàm dưới là toàn bộ chỗ Gemini khác HÌNH chứ không chỉ khác tên. Gom lại đây để
// Snapshot và digest đọc qua một cửa, thay vì mỗi bên tự rẽ nhánh và rẽ lệch nhau.

// contentOf — ruột chở chữ của một message: `content` ở hai nhà kia, `parts` ở Gemini.
func (b *Body) contentOf(msg json.RawMessage) json.RawMessage {
	if b.format == FormatGemini {
		return geminiParts(msg)
	}
	var m struct{ Content json.RawMessage }
	if err := json.Unmarshal(msg, &m); err != nil {
		return nil
	}
	return m.Content
}

// flatten — làm phẳng ruột đó thành chữ hội thoại.
func (b *Body) flatten(raw json.RawMessage) string {
	if b.format == FormatGemini {
		return flattenParts(raw)
	}
	if b.format == FormatOpenAIResponses {
		return flattenResponses(raw)
	}
	return flattenContent(raw)
}

// toolsCalled — tool một lượt trả lời vừa với tới.
func (b *Body) toolsCalled(msg json.RawMessage) []ToolCall {
	if b.format == FormatGemini {
		return geminiToolsCalled(msg)
	}
	if b.format == FormatOpenAIResponses {
		return toolsCalledResponses(msg)
	}
	return toolsCalled(msg)
}

// humanTurn — message vai user này có phải MỘT LƯỢT của người không.
func (b *Body) humanTurn(raw json.RawMessage, rules FrameRules) bool {
	if b.format == FormatGemini {
		return humanTurnGemini(raw, rules)
	}
	return humanTurn(raw, rules)
}

// messagesKey — tên field chở hội thoại. Gemini gọi nó là `contents`. Một chỗ tra tên, nên
// mọi bên đọc hội thoại tự đúng theo format mà không phải rẽ nhánh.
func (b *Body) messagesKey() string {
	if b.format == FormatGemini {
		return geminiContentsKey
	}
	if b.format == FormatOpenAIResponses {
		return responsesInputKey
	}
	return "messages"
}

// messages — mảng messages đã tách, hoặc nil khi không đọc được.
func (b *Body) messages() []json.RawMessage {
	msgs, _ := b.messageList()
	return msgs
}

// messageList — như messages nhưng nói thêm ĐỌC ĐƯỢC hay không. Bên GHI cần biết: mảng
// rỗng hợp lệ cũng cho nil, mà bỏ qua nó thì request đầu tiên của một hội thoại không
// chèn được gì.
func (b *Body) messageList() ([]json.RawMessage, bool) {
	var msgs []json.RawMessage
	if err := json.Unmarshal(b.fields[b.messagesKey()], &msgs); err != nil {
		return nil, false
	}
	return msgs, true
}

func roleOf(msg json.RawMessage) string {
	var m struct{ Role string }
	_ = json.Unmarshal(msg, &m)
	return m.Role
}

func textBlock(text string) json.RawMessage {
	return mustJSON(map[string]string{"type": "text", "text": text})
}

// mustJSON marshal giá trị lõi tự dựng. Không escape HTML: chữ chèn bọc trong
// khối <Soul>, <W.O.N>, mà Marshal mặc định đổi mỗi dấu ngoặc thành cụm escape
// sáu byte — đúng JSON nhưng không ai đọc nổi khi soi nhật ký.
func mustJSON(v any) json.RawMessage {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil
	}
	return bytes.TrimRight(buf.Bytes(), "\n")
}
