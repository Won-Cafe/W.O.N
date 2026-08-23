// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package request

import (
	"encoding/json"
	"strings"
)

// Đường OpenAI Responses. Nó khác HÌNH cả hai nhà kia lẫn Chat Completions:
//
//	lời hệ thống   `instructions` — MỘT chuỗi top-level, không phải mảng block,
//	               cũng không phải message mang vai system
//	hội thoại      `input`, không phải `messages` — và có thể là CHUỖI (one-shot)
//	               hoặc MẢNG items
//	chữ            content blocks dùng `input_text`/`output_text`, không phải `text`
//	item hội thoại  mỗi item có `type`: `message`, `function_call`,
//	               `function_call_output`, `reasoning`…
//	tool call       item riêng `function_call` ở tầng `input`, không phải
//	               `tool_calls` trong message — và KHÔNG có `role`
//	kết quả tool   item riêng `function_call_output`, cũng không có `role`
//	danh mục tool   `name` top-level (như Anthropic), không lồng `function`
//
// Một tin tốt: `content` của message items vẫn là mảng block có `text`, nên đường
// chèn tiếng của lượt (AppendMessage) và đường cắt khung (stripMessageFrame) tái dùng
// được — chỉ cần đổi key field và nhận diện content type.
const (
	responsesInputKey        = "input"
	responsesInstructionsKey = "instructions"
)

// systemTextResponses — lời hệ thống của Responses đến từ HAI nguồn:
//  1. field `instructions` (top-level string) — chuẩn Responses
//  2. items `role: "system"` trong mảng `input` — tương thích ngược với Chat Completions;
//     một số client (DeepSeek Harness) gửi lời hệ thống theo cách này thay vì dùng `instructions`
//
// Nối cả hai theo thứ tự: instructions trước, rồi system items.
func (b *Body) systemTextResponses() string {
	parts := b.systemPartsResponses()
	return strings.Join(parts, "\n\n")
}

// systemPartsResponses — từng khối lời hệ thống: instructions (một khối) + mỗi system item
// trong input một khối. SystemBlocks trả về từng khối, nên AgentTurnShaped đo đúng.
func (b *Body) systemPartsResponses() []string {
	var out []string
	// 1. instructions field (top-level string).
	if raw, ok := b.fields[responsesInstructionsKey]; ok && len(raw) > 0 {
		var s string
		if json.Unmarshal(raw, &s) == nil && strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	// 2. items role:system trong mảng input — tương thích ngược Chat Completions.
	for _, msg := range b.messages() {
		if b.roleOfMsg(msg) != RoleSystem {
			continue
		}
		var m struct{ Content json.RawMessage }
		if json.Unmarshal(msg, &m) != nil {
			continue
		}
		if t := flattenResponses(m.Content); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// appendSystemResponses — chèn chữ thành system item MỚI ở CUỐI vùng system trong
// mảng `input`, giống đường OpenAI (appendSystemOpenAI). Không tạo field `instructions`:
// upstream API (GLM) không nhận field đó, nó chỉ đọc `input`. Vùng system là dãy items
// `role: "system"` ở đầu mảng; item system đặt sau lượt hội thoại đầu tiên thì không còn
// là lời hệ thống nữa.
func (b *Body) appendSystemResponses(text string) {
	msgs, ok := b.messageList()
	if !ok {
		return
	}
	nm := mustJSON(map[string]any{"role": RoleSystem, "content": text})
	if nm == nil {
		return
	}
	at := 0
	for at < len(msgs) && b.roleOfMsg(msgs[at]) == RoleSystem {
		at++
	}
	if out := mustJSON(insertAt(msgs, at, nm)); out != nil {
		b.fields[responsesInputKey] = out
	}
}

// replaceSystemResponses — gộp mọi system item trong input thành một ở vị trí đầu.
// Cắt hết chỉ còn rỗng → bỏ luôn item, không đẩy vỏ trống lên đích.
func (b *Body) replaceSystemResponses(text string) {
	msgs, ok := b.messageList()
	if !ok {
		return
	}
	kept := make([]json.RawMessage, 0, len(msgs))
	placed := false
	for _, msg := range msgs {
		if b.roleOfMsg(msg) != RoleSystem {
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
		b.fields[responsesInputKey] = nb
	}
}

// flattenResponses làm phẳng content blocks của Responses: lọc `input_text` và
// `output_text` (hai type mà Responses dùng thay vì `text` của hai nhà kia).
func flattenResponses(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}
	var sb strings.Builder
	for _, blk := range blocks {
		if (blk.Type == "input_text" || blk.Type == "output_text") && blk.Text != "" {
			sb.WriteString(blk.Text)
			sb.WriteString("\n")
		}
	}
	return strings.TrimSpace(sb.String())
}

// coreMessageResponses dựng một message item của lõi theo hình Responses:
// `{type: "message", role, content: [{type: "input_text", text}]}`.
func (b *Body) coreMessageResponses(role, text string) json.RawMessage {
	if role != RoleUser && role != RoleSystem {
		role = RoleUser
	}
	return mustJSON(map[string]any{
		"type": "message",
		"role": role,
		"content": []any{
			map[string]string{"type": "input_text", "text": text},
		},
	})
}

// toolsCalledResponses — tool call là item riêng `function_call`, không nằm trong
// message. Đọc `name` và `arguments` trực tiếp từ item.
func toolsCalledResponses(msg json.RawMessage) []ToolCall {
	var item struct {
		Type      string `json:"type"`
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}
	if json.Unmarshal(msg, &item) != nil || item.Type != "function_call" || item.Name == "" {
		return nil
	}
	target, kind := targetOf([]byte(item.Arguments))
	return []ToolCall{{Name: item.Name, Target: target, Kind: kind}}
}

// responsesRoleOf — suy vai cho item không mang `role`. Responses có item `function_call`
// (model gọi tool) và `function_call_output` (kết quả tool) đứng riêng ở tầng `input`,
// không có field `role`. Suy từ `type`:
//
//	function_call         → assistant (model với tay)
//	function_call_output  → user      (kết quả tool, cùng vai user như tool_result)
//
// Message items có `role` thật → đọc trực tiếp, không suy.
func responsesRoleOf(msg json.RawMessage) string {
	role := roleOf(msg)
	if role != "" {
		return role
	}
	var m struct{ Type string }
	_ = json.Unmarshal(msg, &m)
	switch m.Type {
	case "function_call":
		return RoleAssistant
	case "function_call_output":
		return RoleUser
	}
	return ""
}

// stripSystemFrameResponses cắt khung trong `instructions` (nếu có) và trong system
// items của mảng `input` (nếu có). `instructions` là chuỗi: cắt rồi đặt lại. System items
// trong input: cắt content bằng cleanContent (luật lời hệ thống), như đường OpenAI.
func (b *Body) stripSystemFrameResponses(rules FrameRules) bool {
	changed := false
	// 1. instructions field (chuỗi).
	if raw, ok := b.fields[responsesInstructionsKey]; ok && len(raw) > 0 {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			cleaned := rules.cleanSystem(s)
			if cleaned != s {
				b.fields[responsesInstructionsKey] = mustJSON(cleaned)
				changed = true
			}
		}
	}
	// 2. system items trong input — cắt content như đường OpenAI.
	for i, msg := range b.messages() {
		if b.roleOfMsg(msg) != RoleSystem {
			continue
		}
		var m map[string]json.RawMessage
		if json.Unmarshal(msg, &m) != nil {
			continue
		}
		content, ok := m["content"]
		if !ok {
			continue
		}
		next, did := cleanContent(content, rules, true)
		if !did {
			continue
		}
		m["content"] = next
		if nm := mustJSON(m); nm != nil {
			b.fields[responsesInputKey] = mustJSON(replaceAt(b.messages(), i, nm))
			changed = true
		}
	}
	return changed
}
