// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package usage

import (
	"strings"
	"testing"
)

// Gọi tool thì không có chữ. Không đọc ra tên tool thì `assistant: ""` của một
// lượt gọi tool trông giống hệt một lượt im — đúng chỗ đang cần phân biệt.
func TestAssistantCalls(t *testing.T) {
	cases := []struct {
		name, body string
		want       []string
	}{
		{"anthropic json", `{"content":[{"type":"tool_use","name":"read_file"}]}`, []string{"read_file"}},
		{"anthropic sse",
			`data: {"type":"content_block_start","content_block":{"type":"tool_use","name":"grep"}}` + "\n\n",
			[]string{"grep"}},
		{"openai json",
			`{"choices":[{"message":{"tool_calls":[{"function":{"name":"read_file"}}]}}]}`,
			[]string{"read_file"}},
		{"openai sse",
			`data: {"choices":[{"delta":{"tool_calls":[{"function":{"name":"list_dir"}}]}}]}` + "\n\n",
			[]string{"list_dir"}},
		{"chữ thường, không tool", `{"content":[{"type":"text","text":"chào"}]}`, nil},
		{"rỗng", "", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := AssistantCalls([]byte(c.body))
			if len(got) != len(c.want) {
				t.Fatalf("muốn %v, got %v", c.want, got)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("muốn %v, got %v", c.want, got)
				}
			}
		})
	}
}

// Nhật ký cần thấy chữ đệ trả NGAY trong lượt đó — không phải đợi lượt sau mới
// thấy lại qua lịch sử. Bốn hình dáng như AssistantCalls, và thinking/signature
// không được lẫn vào coi như lời đáp.
func TestAssistantText(t *testing.T) {
	cases := []struct{ name, body, want string }{
		{"anthropic json", `{"content":[{"type":"text","text":"chào bạn"}]}`, "chào bạn"},
		{"anthropic sse — dồn nhiều delta",
			`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"chào "}}` + "\n\n" +
				`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"bạn"}}` + "\n\n",
			"chào bạn"},
		{"anthropic sse — bỏ qua thinking_delta",
			`data: {"type":"content_block_delta","delta":{"type":"thinking_delta","text":"suy nghĩ riêng"}}` + "\n\n" +
				`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"lời đáp"}}` + "\n\n",
			"lời đáp"},
		{"openai json", `{"choices":[{"message":{"content":"chào bạn"}}]}`, "chào bạn"},
		{"openai sse — dồn nhiều delta",
			`data: {"choices":[{"delta":{"content":"chào "}}]}` + "\n\n" +
				`data: {"choices":[{"delta":{"content":"bạn"}}]}` + "\n\n",
			"chào bạn"},
		{"chỉ gọi tool, không có chữ", `{"content":[{"type":"tool_use","name":"read_file"}]}`, ""},
		{"rỗng", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := AssistantText([]byte(c.body)); got != c.want {
				t.Errorf("muốn %q, got %q", c.want, got)
			}
		})
	}
}

// Gemini gắn cờ `thought` cho part chở chữ SUY NGHĨ. Trước đây lõi không trừ nó, nên trên một
// lượt thật `reply` dài 1503 ký tự toàn narration và đứt giữa câu, còn lời đáp bị cắt mất.
func TestGeminiSplitsThinkingFromReply(t *testing.T) {
	body := `{"candidates":[{"content":{"role":"model","parts":[
		{"text":"**Initiating System Awareness**\nTôi đang cân nhắc.","thought":true},
		{"text":"Chào anh, tôi đã đọc đất."},
		{"functionCall":{"name":"list_directory"}}
	]}}]}`
	if got := AssistantText([]byte(body)); got != "Chào anh, tôi đã đọc đất." {
		t.Errorf("reply = %q — chữ suy nghĩ chưa bị trừ", got)
	}
	if got := AssistantThinking([]byte(body)); !strings.Contains(got, "Initiating System Awareness") {
		t.Errorf("thinking = %q — phải giữ riêng, không bỏ mất", got)
	}
	if got := AssistantCalls([]byte(body)); len(got) != 1 || got[0] != "list_directory" {
		t.Errorf("tool_calls = %v", got)
	}
}

// Lượt CHỈ có suy nghĩ và gọi tool thì `reply` phải RỖNG, không phải narration.
func TestGeminiToolOnlyTurnHasEmptyReply(t *testing.T) {
	body := `{"candidates":[{"content":{"parts":[
		{"text":"Tôi sẽ liệt kê thư mục.","thought":true},
		{"functionCall":{"name":"update_topic"},"thoughtSignature":"AAAA"}
	]}}]}`
	if got := AssistantText([]byte(body)); got != "" {
		t.Errorf("reply phải rỗng, got %q", got)
	}
}

// Nhánh Anthropic: khối `thinking` giờ có chỗ riêng thay vì bị bỏ hẳn.
func TestAnthropicThinkingCaptured(t *testing.T) {
	body := `{"content":[{"type":"thinking","thinking":"cân nhắc đã"},{"type":"text","text":"xong"}]}`
	if got := AssistantText([]byte(body)); got != "xong" {
		t.Errorf("reply = %q", got)
	}
	if got := AssistantThinking([]byte(body)); got != "cân nhắc đã" {
		t.Errorf("thinking = %q", got)
	}
}
