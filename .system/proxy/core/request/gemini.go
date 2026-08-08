// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package request

import (
	"encoding/json"
	"strings"
)

// Đường Gemini native. Nó không chỉ đặt tên khác hai nhà kia, nó khác HÌNH:
//
//	lời hệ thống   `systemInstruction` — MỘT Content object, không phải mảng block,
//	               cũng không phải một message mang vai system
//	hội thoại      `contents`, không phải `messages`
//	chữ            `parts[].text` — không có field `type` nào để lọc theo
//	vai của model  `model`, không phải `assistant`
//	danh mục tool  `tools[].functionDeclarations[]` — lồng thêm một tầng
//	kết quả tool   một `part` mang `functionResponse`, không phải block `tool_result`
//
// Một tin tốt: mảng `parts` chính là đường may sẵn có, nên "mỗi tiếng nói một khối riêng,
// không trộn" (§ Format wire) ở đây rẻ hơn cả hai nhà kia — mỗi khối lõi chèn là đúng một
// `part` mới, không phải một lần nối chữ vào chữ đang có.
const (
	geminiContentsKey = "contents"
	// RoleModel — vai của model trên đường Gemini. Lõi quy về "assistant" khi ĐỌC
	// (normRole), nên mọi chỗ đang so với "assistant" không phải biết tới tên này.
	RoleModel = "model"
)

// geminiSystemKeys — REST của Google nhận cả hai cách viết, và thứ tự đây là thứ tự tra.
// SDK JS (thứ Gemini CLI dùng) gửi camelCase; giữ cả snake để một client viết tay không
// lặng lẽ mất khối đất vì lõi tra sai tên.
var geminiSystemKeys = []string{"systemInstruction", "system_instruction"}

// geminiPart — một `part`, đọc đủ để phân loại. Google nhận cả hai cách viết cho hai khoá
// tool, nên đọc cả hai: nhận sai một `functionResponse` thành chữ của người là mở một phiên
// ma và đếm sai số lượt người.
type geminiPart struct {
	Text              string          `json:"text"`
	FunctionCall      json.RawMessage `json:"functionCall"`
	FunctionCallSnake json.RawMessage `json:"function_call"`
	FunctionResp      json.RawMessage `json:"functionResponse"`
	FunctionRespSnake json.RawMessage `json:"function_response"`
}

func (p geminiPart) isFunctionResponse() bool {
	return len(p.FunctionResp) > 0 || len(p.FunctionRespSnake) > 0
}

func (p geminiPart) call() json.RawMessage {
	if len(p.FunctionCall) > 0 {
		return p.FunctionCall
	}
	return p.FunctionCallSnake
}

// geminiSystemField — khoá `systemInstruction` đang CÓ MẶT trong body, kèm ruột của nó.
// Vắng cả hai cách viết → trả khoá camelCase để bên ghi dùng làm chỗ tạo mới.
func (b *Body) geminiSystemField() (string, json.RawMessage) {
	for _, key := range geminiSystemKeys {
		if raw, ok := b.fields[key]; ok && len(raw) > 0 {
			return key, raw
		}
	}
	return geminiSystemKeys[0], nil
}

// systemTextGemini — lời hệ thống làm phẳng, cho chỗ khớp tiêu đề soul và chỗ kiểm khối
// đã có mặt chưa.
func (b *Body) systemTextGemini() string {
	_, raw := b.geminiSystemField()
	return strings.Join(b.systemPartsGemini(raw), "\n\n")
}

// systemPartsGemini — chữ của từng `part` trong systemInstruction, giữ đúng đường may.
// Đây là thứ SystemBlocks trả về trên đường này, nên `AgentTurnShaped` đo lời dạy của
// Gemini CLI bằng cùng một phép đo nó dùng cho Claude Code.
func (b *Body) systemPartsGemini(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var m struct {
		Parts json.RawMessage `json:"parts"`
	}
	if json.Unmarshal(raw, &m) != nil {
		return nil
	}
	var parts []geminiPart
	if json.Unmarshal(m.Parts, &parts) != nil {
		return nil
	}
	var out []string
	for _, p := range parts {
		if p.Text != "" {
			out = append(out, p.Text)
		}
	}
	return out
}

// appendSystemGemini đặt text thành một `part` MỚI ở cuối `systemInstruction.parts`.
// Vắng systemInstruction thì dựng mới. Mọi field khác của Content object (`role`, …) đi
// qua nguyên từng byte — map[string]json.RawMessage giữ chúng, lõi chỉ chạm `parts` (#7).
func (b *Body) appendSystemGemini(text string) {
	key, raw := b.geminiSystemField()
	if len(raw) == 0 {
		if out := mustJSON(map[string]any{
			"parts": []any{map[string]string{"text": text}},
		}); out != nil {
			b.fields[key] = out
		}
		return
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return // dạng lạ: thà không chèn còn hơn làm mất chữ
	}
	var parts []json.RawMessage
	if p, ok := m["parts"]; ok && len(p) > 0 {
		if json.Unmarshal(p, &parts) != nil {
			return
		}
	}
	np := mustJSON(map[string]string{"text": text})
	if np == nil {
		return
	}
	m["parts"] = mustJSON(append(parts, np))
	if out := mustJSON(m); out != nil {
		b.fields[key] = out
	}
}

// toolInfosGemini — `tools[].functionDeclarations[]`. Một phần tử `tools` chở NHIỀU món,
// nên đây là hai vòng, không một: đọc một tầng như hai nhà kia thì mọi tên đều rỗng.
func (b *Body) toolInfosGemini() []ToolInfo {
	var tools []struct {
		Decls []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"functionDeclarations"`
		DeclsSnake []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"function_declarations"`
	}
	if json.Unmarshal(b.fields["tools"], &tools) != nil {
		return nil
	}
	var out []ToolInfo
	for _, t := range tools {
		decls := t.Decls
		if len(decls) == 0 {
			decls = t.DeclsSnake
		}
		for _, d := range decls {
			if d.Name == "" {
				continue
			}
			out = append(out, ToolInfo{Name: d.Name, Description: d.Description})
		}
	}
	return out
}

// geminiGroupHasTool — phần tử `tools` này có chở món nào không. Gemini khai hai loại tool bằng
// hai cách:
//
//	tool của client  `functionDeclarations` là mảng, có phần tử
//	tool sẵn của nhà `googleSearch`, `urlContext`, `codeExecution` — khai bằng SỰ CÓ MẶT của
//	                 khoá, giá trị là một object cấu hình, rỗng cũng được
//
// Nên phép đo là "có khoá nào mang một object, hoặc một mảng có phần tử". Khoá mang chuỗi không
// phải một món: `{"name": ""}` là nhãn của nhóm, tay vẫn trắng.
func geminiGroupHasTool(group json.RawMessage) bool {
	var m map[string]json.RawMessage
	if json.Unmarshal(group, &m) != nil {
		return false
	}
	for key, raw := range m {
		v := strings.TrimSpace(string(raw))
		if key == "functionDeclarations" || key == "function_declarations" {
			var decls []json.RawMessage
			if json.Unmarshal(raw, &decls) == nil && len(decls) > 0 {
				return true
			}
			continue
		}
		if strings.HasPrefix(v, "{") {
			return true
		}
		if strings.HasPrefix(v, "[") && v != "[]" {
			return true
		}
	}
	return false
}

// geminiParts — ruột `parts` của một Content. Đây là chỗ đường Gemini rẽ khỏi `content`
// của hai nhà kia.
func geminiParts(msg json.RawMessage) json.RawMessage {
	var m struct {
		Parts json.RawMessage `json:"parts"`
	}
	if json.Unmarshal(msg, &m) != nil {
		return nil
	}
	return m.Parts
}

// flattenParts làm phẳng `parts` thành chữ hội thoại. Không có field `type` để lọc, nên
// phép lọc là "part nào có `text`": functionCall, functionResponse, inlineData đều không
// mang `text` nên tự rơi ra ngoài.
func flattenParts(raw json.RawMessage) string {
	var parts []geminiPart
	if len(raw) == 0 || json.Unmarshal(raw, &parts) != nil {
		return ""
	}
	var sb strings.Builder
	for _, p := range parts {
		if p.Text != "" {
			sb.WriteString(p.Text)
			sb.WriteString("\n")
		}
	}
	return strings.TrimSpace(sb.String())
}

// humanPartsGemini — `parts` này là lời người hay kết quả tool mặc vai user. Cùng phép đo
// với đường Anthropic: cần ít nhất một part KHÔNG phải functionResponse. Và phép đo là
// "còn part nào", không phải "còn chữ nào" — một lượt chỉ có ảnh vẫn là một lượt của người.
func humanPartsGemini(raw json.RawMessage) bool {
	var parts []geminiPart
	if len(raw) == 0 || json.Unmarshal(raw, &parts) != nil {
		return false
	}
	for _, p := range parts {
		if p.isFunctionResponse() {
			continue
		}
		// Part có chữ thì chữ phải khác rỗng; part không chữ (ảnh, file) thì tự là nội dung.
		if p.Text == "" || strings.TrimSpace(p.Text) != "" {
			return true
		}
	}
	return false
}

// humanTurnGemini — bản Gemini của humanTurn, cùng hai đường thành lượt người và cùng lý do
// KHÔNG siết khi chưa ai khai vỏ nào: siết mà không biết vỏ tên gì thì mọi lượt hoá việc
// nhà, đếm 0, tệ hơn đếm thừa (#6).
func humanTurnGemini(raw json.RawMessage, rules FrameRules) bool {
	if !humanPartsGemini(raw) {
		return false
	}
	if len(rules.Unwrap) == 0 {
		return true
	}
	// flattenParts đã bỏ part không mang chữ, nên một lượt chỉ có ảnh ra chuỗi rỗng ở đây —
	// mà humanPartsGemini phía trên đã tính nó là lượt người rồi.
	s := flattenParts(raw)
	if strings.TrimSpace(s) == "" {
		return true
	}
	for _, tag := range rules.Unwrap {
		if inner, ok := extractBlock(s, tag); ok && strings.TrimSpace(inner) != "" {
			return true
		}
	}
	return strings.TrimSpace(outsideTags(s)) != ""
}

// geminiToolsCalled — tool một lượt `model` vừa với tới, đọc ở lượt GỌI. `args` là object,
// nên targetOf dùng lại được nguyên.
func geminiToolsCalled(msg json.RawMessage) []ToolCall {
	var parts []geminiPart
	if json.Unmarshal(geminiParts(msg), &parts) != nil {
		return nil
	}
	var out []ToolCall
	for _, p := range parts {
		raw := p.call()
		if len(raw) == 0 {
			continue
		}
		var fc struct {
			Name string          `json:"name"`
			Args json.RawMessage `json:"args"`
		}
		if json.Unmarshal(raw, &fc) != nil || fc.Name == "" {
			continue
		}
		out = append(out, ToolCall{Name: fc.Name, Target: targetOf(fc.Args)})
	}
	return out
}

// stripSystemFrameGemini cắt khung trong `systemInstruction.parts`, từng part một. Hình
// part giữ nguyên là hình part, cùng kỷ luật với đường Anthropic.
//
// Không có khái niệm `cache_control` ở đây: Gemini không có mốc cache nào để cắm, cache
// ngầm của nó khớp theo tiền tố (§ Cache). Nên cả lớp phòng "đừng xoá mốc của công cụ chủ"
// không có việc gì làm trên đường này.
//
// Cắt sạch không còn part nào → BỎ HẲN field, không để lại `parts: []`: một mảng rỗng là
// thứ nhà cung cấp từ chối, y như `content: []` bên kia.
func (b *Body) stripSystemFrameGemini(rules FrameRules) bool {
	key, raw := b.geminiSystemField()
	if len(raw) == 0 {
		return false
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return false
	}
	var parts []map[string]any
	if json.Unmarshal(m["parts"], &parts) != nil {
		return false
	}
	kept := make([]map[string]any, 0, len(parts))
	changed := false
	for _, p := range parts {
		text, isText := p["text"].(string)
		if !isText {
			kept = append(kept, p)
			continue
		}
		cleaned := rules.cleanSystem(text)
		if cleaned != text {
			changed = true
		}
		if cleaned == "" {
			continue
		}
		p["text"] = cleaned
		kept = append(kept, p)
	}
	if !changed {
		return false
	}
	if len(kept) == 0 {
		delete(b.fields, key)
		return true
	}
	m["parts"] = mustJSON(kept)
	if out := mustJSON(m); out != nil {
		b.fields[key] = out
	}
	return true
}

// stripMessageFrameGemini cắt khung trong `contents[].parts`, chỉ ở lượt vai `user` —
// ngoại lệ duy nhất chạm lượt người (#1), và nó chỉ bỏ vỏ của công cụ khác. Không chạm vai
// `model`: sửa chữ model đã nói là viết lại quá khứ.
//
// Bỏ lượt không còn part nào, không phải lượt không còn chữ nào: một lượt chỉ có
// `functionResponse` vẫn phải đi, vì bỏ nó là để `functionCall` trơ trọi.
func (b *Body) stripMessageFrameGemini(rules FrameRules) bool {
	msgs := b.messages()
	if msgs == nil {
		return false
	}
	changed := false
	for i, msg := range msgs {
		if b.roleOfMsg(msg) != RoleUser {
			continue
		}
		var m map[string]json.RawMessage
		if json.Unmarshal(msg, &m) != nil {
			continue
		}
		next, did := cleanParts(m["parts"], rules)
		if !did {
			continue
		}
		m["parts"] = next
		if nm := mustJSON(m); nm != nil {
			msgs[i] = nm
			changed = true
		}
	}
	if !changed {
		return false
	}
	if nb := mustJSON(b.dropEmptiedGemini(msgs)); nb != nil {
		b.fields[geminiContentsKey] = nb
	}
	return true
}

// cleanParts bỏ khung khỏi từng part mang chữ; part không chữ đi qua nguyên vẹn.
func cleanParts(raw json.RawMessage, rules FrameRules) (json.RawMessage, bool) {
	var parts []map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &parts) != nil {
		return nil, false
	}
	kept := make([]map[string]any, 0, len(parts))
	changed := false
	for _, p := range parts {
		text, isText := p["text"].(string)
		if !isText {
			kept = append(kept, p)
			continue
		}
		cleaned := rules.clean(text)
		if cleaned != text {
			changed = true
		}
		if cleaned == "" {
			continue
		}
		p["text"] = cleaned
		kept = append(kept, p)
	}
	if !changed {
		return nil, false
	}
	return mustJSON(kept), true
}

// dropEmptiedGemini bỏ lượt `user` không còn part nào sau khi cắt. Vai khác thì không xét:
// vai nào ta không chạm thì ta không bỏ (#7).
func (b *Body) dropEmptiedGemini(msgs []json.RawMessage) []json.RawMessage {
	out := msgs[:0]
	for _, msg := range msgs {
		if b.roleOfMsg(msg) == RoleUser && geminiPartsEmpty(msg) {
			continue
		}
		out = append(out, msg)
	}
	return out
}

func geminiPartsEmpty(msg json.RawMessage) bool {
	var parts []json.RawMessage
	raw := geminiParts(msg)
	if len(raw) == 0 || json.Unmarshal(raw, &parts) != nil {
		return true
	}
	return len(parts) == 0
}
