// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package request

import "strings"

// Format là căn cước định dạng của request — router quyết từ path, không sniff
// body (#6). Zero-value Unknown: ép tường minh, không có Anthropic ngầm định.
type Format int

const (
	FormatUnknown         Format = 0 // zero-value — chưa được gán, không chèn gì
	FormatAnthropic       Format = 1 // /v1/messages — field top-level system, name tool
	FormatOpenAI          Format = 2 // /v1/chat/completions — message system, tool.function.name
	FormatGemini          Format = 3 // /v1beta/models/{model}:generateContent — systemInstruction, contents
	FormatOpenAIResponses Format = 4 // /v1/responses — instructions, input (string hoặc mảng items)
)

// FormatFromPath quyết định format từ hậu tố path. Không khớp → Unknown → passthrough.
//
// KHÔNG có ca đặc biệt cho cửa gói request vào phong bì (thân thật nằm lồng dưới một khoá,
// nên mọi method đọc ở tầng top-level đọc trượt). Từng có một hằng chép path riêng của một
// dịch vụ ở đây; nó thừa. Thân phong bì tự rơi khỏi dòng chính bằng cơ chế chung: không
// `messages`/`contents` đọc được thì `ConversationShaped` và `AgentTurnShaped` cùng false,
// và `agentFor` trả lượt ấy về chuyển tiếp nguyên bản. Một luật đo HÌNH đã có sẵn thì
// không cần thêm một luật đọc TÊN — xem TestEnvelopedBodyStaysUntouched.
func FormatFromPath(path string) Format {
	switch {
	case strings.HasSuffix(path, "/messages"):
		return FormatAnthropic
	case strings.HasSuffix(path, "/chat/completions"):
		return FormatOpenAI
	case strings.HasSuffix(path, "/responses"):
		return FormatOpenAIResponses
	case strings.HasSuffix(path, ":generateContent"),
		strings.HasSuffix(path, ":streamGenerateContent"):
		return FormatGemini
	default:
		return FormatUnknown
	}
}

func (f Format) String() string {
	switch f {
	case FormatAnthropic:
		return "anthropic"
	case FormatOpenAI:
		return "openai"
	case FormatGemini:
		return "gemini"
	case FormatOpenAIResponses:
		return "openai-responses"
	default:
		return "unknown"
	}
}
