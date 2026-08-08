// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package request

import "testing"

// Thân gói trong PHONG BÌ — mọi trường của định dạng nằm lồng dưới một khoá, nên đọc ở
// tầng top-level là đọc trượt. Từng có một hằng chép path riêng của một dịch vụ trong
// `format.go` để ép những lượt ấy về passthrough; test này là lý do cái hằng ấy không cần
// tồn tại, và là lưới giữ cho nó đừng quay lại.
//
// Cơ chế chung đã đủ: không đọc ra hội thoại thì mọi cửa của `agentFor` cùng đóng, và lượt
// ấy đi qua nguyên bản. Đo HÌNH thì đúng cho mọi cửa gói phong bì, kể cả cửa chưa ai gặp;
// đọc TÊN thì chỉ đúng cho đúng một dịch vụ, và im lặng sai cho phần còn lại.
func TestEnvelopedBodyStaysUntouched(t *testing.T) {
	raw := `{"request":{"contents":[{"role":"user","parts":[{"text":"chào"}]}],` +
		`"systemInstruction":{"parts":[{"text":"# Tzu"}]},` +
		`"tools":[{"functionDeclarations":[{"name":"Read"}]}]}}`
	b := mustBody(t, raw, FormatGemini)
	rules := FrameRules{Strip: []string{"system-reminder"}, Unwrap: []string{"userRequest"}}

	if b.MessageCount() != 0 {
		t.Errorf("thân phong bì không có hội thoại đọc được, got %d message", b.MessageCount())
	}
	if b.SystemText() != "" {
		t.Errorf("lời hệ thống cũng nằm trong phong bì, got %q", b.SystemText())
	}
	// Hai cửa này cùng đóng là đủ để `agentFor` trả lượt về chuyển tiếp nguyên bản.
	if b.AgentTurnShaped(rules) {
		t.Error("thân phong bì không được tính là có hình một lượt của đệ")
	}
	if b.ConversationShaped() {
		t.Error("thân phong bì không được tính là một cuộc hội thoại")
	}
}

// Hậu tố của định dạng vẫn nhận ra bình thường — bỏ ca đặc biệt không được làm hỏng
// đường thường, kể cả với path có tiền tố lạ.
func TestFormatFromPathKeepsSuffixMatching(t *testing.T) {
	cases := []struct {
		path string
		want Format
	}{
		{"/v1/messages", FormatAnthropic},
		{"/v1/chat/completions", FormatOpenAI},
		{"/v1beta/models/gemini-3-pro:generateContent", FormatGemini},
		{"/v1beta/models/gemini-3-pro:streamGenerateContent", FormatGemini},
		{"/anything/else", FormatUnknown},
	}
	for _, c := range cases {
		if got := FormatFromPath(c.path); got != c.want {
			t.Errorf("%s: muốn %v, got %v", c.path, c.want, got)
		}
	}
}
