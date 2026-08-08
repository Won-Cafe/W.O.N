// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package usage

import (
	"bytes"
	"encoding/json"
)

// forEachEvent gọi fn cho từng "cục" JSON trong thân response — một cục nếu
// JSON thường, hoặc từng dòng `data: {...}` nếu SSE. AssistantCalls và
// AssistantText đều cần đi qua đúng bốn hình dáng như nhau (JSON/SSE ×
// Anthropic/OpenAI), nên chung một đường quét.
func forEachEvent(body []byte, fn func(json.RawMessage)) {
	fn(body)
	for _, line := range bytes.Split(body, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		if p := bytes.TrimSpace(line[len("data:"):]); len(p) > 0 && p[0] == '{' {
			fn(p)
		}
	}
}

// AssistantCalls trả tên tool đệ vừa gọi. Lời đệ trả quay lại nguyên vẹn ở mảng
// messages lượt sau, nên giữ ở đây là in hai lần. Tên tool thì không quay lại được:
// lượt assistant gọi tool có content rỗng. Hai hình dáng: JSON một cục và dòng SSE.
func AssistantCalls(body []byte) []string {
	if len(body) == 0 {
		return nil
	}
	var out []string
	forEachEvent(body, func(b json.RawMessage) {
		out = addCalls(out, eventCalls(b))
	})
	return out
}

// addCalls nối tên mới vào danh sách, bỏ tên rỗng và tên đã có. Hai đường đọc tool —
// một cục và rút dọc đường — cùng gọi nó, nên phép trùng chỉ có một bản.
func addCalls(out, names []string) []string {
	for _, n := range names {
		if n == "" {
			continue
		}
		dup := false
		for _, x := range out {
			if x == n {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, n)
		}
	}
	return out
}

// eventCalls — tên tool trong MỘT cục thân. Bốn hình: Anthropic một cục, Anthropic delta,
// OpenAI (message/delta), Gemini.
func eventCalls(b json.RawMessage) []string {
	var v struct {
		Content []struct {
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"content"`
		ContentBlock struct {
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"content_block"`
		Choices []struct {
			Message struct {
				ToolCalls []struct {
					Function struct{ Name string } `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			Delta struct {
				ToolCalls []struct {
					Function struct{ Name string } `json:"function"`
				} `json:"tool_calls"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if json.Unmarshal(b, &v) != nil {
		return nil
	}
	var out []string
	for _, c := range v.Content {
		if c.Type == "tool_use" {
			out = append(out, c.Name)
		}
	}
	if v.ContentBlock.Type == "tool_use" {
		out = append(out, v.ContentBlock.Name)
	}
	for _, ch := range v.Choices {
		for _, tc := range ch.Message.ToolCalls {
			out = append(out, tc.Function.Name)
		}
		for _, tc := range ch.Delta.ToolCalls {
			out = append(out, tc.Function.Name)
		}
	}
	// Gemini: candidates[].content.parts[].functionCall. Không có hình delta riêng — thân
	// SSE của nó là cùng một hình candidates, gửi từng cục.
	return append(out, geminiCallNames(b)...)
}

// geminiPart — một part trong lời đáp. `Thought` là cờ Gemini gắn cho part chở chữ SUY NGHĨ.
// Nó không có trong tài liệu công khai tôi đọc được, nhưng `ThoughtSignature` thì ĐO ĐƯỢC trên
// dây (7 chỗ, tới 3.904 ký tự), nên tài liệu đi sau thực tế ở chỗ này. Vì vậy lõi không đoán:
// nó tách hai luồng chữ ra hai field của nhật ký, và chính nhật ký nói cờ nào có thật.
type geminiRespPart struct {
	Text             string `json:"text"`
	Thought          bool   `json:"thought"`
	ThoughtSignature string `json:"thoughtSignature"`
	FunctionCall     struct {
		Name string `json:"name"`
	} `json:"functionCall"`
}

// geminiCandidateParts đọc `candidates[].content.parts` của một cục thân Gemini. Ba đường dùng
// nó (tên tool, lời đáp, chữ suy nghĩ) nên tách riêng, khỏi parse ba lần ba chỗ lệch nhau.
func geminiCandidateParts(b json.RawMessage) []geminiRespPart {
	var v struct {
		Candidates []struct {
			Content struct {
				Parts []geminiRespPart `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if json.Unmarshal(b, &v) != nil {
		return nil
	}
	var out []geminiRespPart
	for _, c := range v.Candidates {
		out = append(out, c.Content.Parts...)
	}
	return out
}

func geminiCallNames(b json.RawMessage) []string {
	var out []string
	for _, p := range geminiCandidateParts(b) {
		if p.FunctionCall.Name != "" {
			out = append(out, p.FunctionCall.Name)
		}
	}
	return out
}

// AssistantText rút chữ đệ đã trả — cho nhật ký thấy NGAY trong lượt đó, không
// phải đợi lượt sau mới thấy lại qua lịch sử hội thoại client gửi lên (và nếu
// đây là lượt cuối của phiên, "lượt sau" không bao giờ tới). Chỉ rút khối
// `text`/`text_delta` — bỏ qua thinking/signature, đó không phải lời đáp.
func AssistantText(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var sb bytes.Buffer
	forEachEvent(body, func(b json.RawMessage) { sb.WriteString(eventText(b)) })
	return sb.String()
}

// eventText — chữ trả lời trong MỘT cục thân.
func eventText(b json.RawMessage) string {
	var v struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Delta struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"delta"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if json.Unmarshal(b, &v) != nil {
		return ""
	}
	var sb bytes.Buffer
	for _, c := range v.Content {
		if c.Type == "text" {
			sb.WriteString(c.Text)
		}
	}
	if v.Delta.Type == "text_delta" {
		sb.WriteString(v.Delta.Text)
	}
	for _, ch := range v.Choices {
		sb.WriteString(ch.Message.Content)
		sb.WriteString(ch.Delta.Content)
	}
	// Gemini: chữ nằm ở candidates[].content.parts[].text. Part mang functionCall không có
	// `text` nên tự rơi ra. Part chở chữ SUY NGHĨ thì phải trừ tường minh — cùng luật với
	// nhánh Anthropic bỏ khối `thinking`, vì suy nghĩ không phải lời đáp.
	for _, p := range geminiCandidateParts(b) {
		if p.Thought {
			continue
		}
		sb.WriteString(p.Text)
	}
	return sb.String()
}

// AssistantThinking rút chữ SUY NGHĨ, thành một luồng RIÊNG của nhật ký. Trộn chung thì suy
// nghĩ ăn hết trần 1500 ký tự của `reply` và lời đáp thật bị cắt — đo trên đường Gemini: hai
// lượt liền `reply` dài 1503/1504, đứt giữa câu, không lượt nào thấy lời đáp.
//
// Tách hai luồng thay vì chỉ lọc bỏ, vì hai lý do: suy nghĩ CÓ ích khi soi bệnh, và nếu cờ
// `thought` không có thật thì `reply` sẽ rỗng còn `thinking` sẽ đầy — nhật ký tự tố giác chỗ
// lõi đoán sai, thay vì im lặng mất chữ.
func AssistantThinking(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var sb bytes.Buffer
	forEachEvent(body, func(b json.RawMessage) { sb.WriteString(eventThinking(b)) })
	return sb.String()
}

// eventThinking — chữ suy nghĩ trong MỘT cục thân.
//
// Đường OpenAI chở nó ở `reasoning_content` (GLM, DeepSeek, Ollama) hoặc `reasoning`, cạnh
// `content` trong cùng một delta. Không đọc trường ấy thì chữ suy nghĩ không mất — nó chảy
// qua như byte vô danh và ăn hết trần bản sao thân, rồi câu trả lời tới sau không còn chỗ.
func eventThinking(b json.RawMessage) string {
	var v struct {
		Content []struct {
			Type     string `json:"type"`
			Thinking string `json:"thinking"`
		} `json:"content"`
		Delta struct {
			Type     string `json:"type"`
			Thinking string `json:"thinking"`
		} `json:"delta"`
		Choices []struct {
			Message struct {
				ReasoningContent string `json:"reasoning_content"`
				Reasoning        string `json:"reasoning"`
			} `json:"message"`
			Delta struct {
				ReasoningContent string `json:"reasoning_content"`
				Reasoning        string `json:"reasoning"`
			} `json:"delta"`
		} `json:"choices"`
	}
	var sb bytes.Buffer
	if json.Unmarshal(b, &v) == nil {
		for _, c := range v.Content {
			if c.Type == "thinking" {
				sb.WriteString(c.Thinking)
			}
		}
		if v.Delta.Type == "thinking_delta" {
			sb.WriteString(v.Delta.Thinking)
		}
		for _, ch := range v.Choices {
			sb.WriteString(ch.Message.ReasoningContent)
			sb.WriteString(ch.Message.Reasoning)
			sb.WriteString(ch.Delta.ReasoningContent)
			sb.WriteString(ch.Delta.Reasoning)
		}
	}
	for _, p := range geminiCandidateParts(b) {
		if p.Thought {
			sb.WriteString(p.Text)
		}
	}
	return sb.String()
}
