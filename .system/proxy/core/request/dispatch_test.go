// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package request

import "testing"

// Lời giao mở đầu cuộc → nhận ra. Đây là ca thường của một đệ được giao việc.
func TestDispatchMarkedAtOpening(t *testing.T) {
	b := mustBody(t, `{"messages":[{"role":"user","content":"`+DispatchMark+` gọi — dò địa hình"}]}`, FormatAnthropic)
	if !b.DispatchMarked(FrameRules{}) {
		t.Error("lời giao mở đầu cuộc phải được nhận ra")
	}
}

// Cuộc do người mở → không mang mỏ neo → lối cũ giữ nguyên.
func TestDispatchNotMarkedForHumanOpening(t *testing.T) {
	b := mustBody(t, `{"messages":[{"role":"user","content":"chào, xem giúp tôi chỗ này"}]}`, FormatAnthropic)
	if b.DispatchMarked(FrameRules{}) {
		t.Error("cuộc người mở không được tính là lượt giao việc")
	}
}

// Mỏ neo bị NHẮC LẠI ở lượt sau không phải một cuộc được giao. Đây là chỗ một phép khớp
// quét cả thân sẽ hỏng: đệ đọc trúng file có chứa chuỗi ấy — hay đọc chính đoạn hội thoại
// đang bàn về chuỗi ấy — rồi tự nhận mình là lượt giao việc.
func TestDispatchMarkQuotedLaterDoesNotCount(t *testing.T) {
	b := mustBody(t, `{"messages":[
		{"role":"user","content":"cuộc này người mở"},
		{"role":"assistant","content":"ừ"},
		{"role":"user","content":"`+DispatchMark+` gọi — chuỗi này chỉ đang được trích"}]}`, FormatAnthropic)
	if b.DispatchMarked(FrameRules{}) {
		t.Error("mỏ neo nhắc lại ở lượt sau không được tính")
	}
}

// Mỏ neo phải đứng ĐẦU chữ. Nằm giữa câu là chữ được kể, không phải lời giao.
func TestDispatchMarkMidTextDoesNotCount(t *testing.T) {
	b := mustBody(t, `{"messages":[{"role":"user","content":"tôi định viết `+DispatchMark+` gọi — vào đây"}]}`, FormatAnthropic)
	if b.DispatchMarked(FrameRules{}) {
		t.Error("mỏ neo giữa câu không được tính")
	}
}

// Mỏ neo nằm trong CHỮ hội thoại, không trong trường riêng của nhà nào — nên nó phải đúng
// ở cả ba định dạng. Gemini chở chữ ở `contents[].parts[]`, đọc thẳng `content` ra rỗng.
func TestDispatchMarkedAcrossFormats(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		f    Format
	}{
		{"anthropic", `{"messages":[{"role":"user","content":"` + DispatchMark + ` gọi — dò"}]}`, FormatAnthropic},
		{"openai", `{"messages":[{"role":"user","content":"` + DispatchMark + ` gọi — dò"}]}`, FormatOpenAI},
		{"gemini", `{"contents":[{"role":"user","parts":[{"text":"` + DispatchMark + ` gọi — dò"}]}]}`, FormatGemini},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !mustBody(t, c.raw, c.f).DispatchMarked(FrameRules{}) {
				t.Errorf("%s: lời giao phải được nhận ra", c.name)
			}
		})
	}
}

// Khung Antigravity thật, rút gọn: công cụ chủ chèn việc nhà của nó vào vai user TRƯỚC câu
// người, và gói câu ấy vào `<userRequest>`. Đo mỏ neo thẳng trên message số 0 thì lượt được
// giao trượt cửa, rơi xuống `default_agent`, và một tay việc được dạy rằng nó là người điều
// phối — đúng ca đã hỏng thật (phiên 20260814-152537, head/agent=Tzu cho lượt giao cho Han).
func TestDispatchMarkedBehindHostFraming(t *testing.T) {
	raw := `{"messages":[
		{"role":"user","content":"<environment_info>\nmacOS\n</environment_info>\n<workspace_info>\n/W.O.N\n</workspace_info>"},
		{"role":"user","content":"<context>\nhôm nay\n</context>\n<userRequest>` + DispatchMark + ` gọi — kiểm chất</userRequest>"}]}`
	if !mustBody(t, raw, FormatOpenAI).DispatchMarked(rules) {
		t.Error("lời giao nằm sau việc nhà của công cụ chủ vẫn phải được nhận ra")
	}
}

// Bỏ qua việc nhà không được thành bỏ qua tất: lượt người ĐẦU TIÊN không mang mỏ neo thì
// cuộc này là cuộc người mở, dù có bao nhiêu khối việc nhà đứng trước.
func TestDispatchNotMarkedWhenFirstHumanTurnIsPlain(t *testing.T) {
	raw := `{"messages":[
		{"role":"user","content":"<environment_info>\nmacOS\n</environment_info>"},
		{"role":"user","content":"<userRequest>chào, xem giúp tôi chỗ này</userRequest>"},
		{"role":"assistant","content":"ừ"},
		{"role":"user","content":"<userRequest>` + DispatchMark + ` gọi — chuỗi này chỉ đang được trích</userRequest>"}]}`
	if mustBody(t, raw, FormatOpenAI).DispatchMarked(rules) {
		t.Error("cuộc người mở không được tính là lượt giao việc")
	}
}

// Chưa khai vỏ nào thì lõi không siết: `humanTurn` cho mọi lời người qua và `clean` không
// bóc gì, nên hàm trở về đúng lối cũ. Siết mà không có lời khai là đoán (#6).
func TestDispatchUndeclaredFramingKeepsOldReading(t *testing.T) {
	raw := `{"messages":[{"role":"user","content":"<userRequest>` + DispatchMark + ` gọi — dò</userRequest>"}]}`
	if mustBody(t, raw, FormatOpenAI).DispatchMarked(FrameRules{}) {
		t.Error("chưa khai vỏ thì mỏ neo trong vỏ không được nhận ra")
	}
	if !mustBody(t, raw, FormatOpenAI).DispatchMarked(rules) {
		t.Error("khai rồi thì phải nhận ra")
	}
}

// Phần mở cuộc là của CÔNG CỤ CHỦ, không của hệ: nó tách bao nhiêu message vai user tuỳ nó,
// và có thể để chữ trần lọt ra ngoài vỏ. Mỏ neo được phép nằm ở bất cứ message nào trước lời
// đáp đầu tiên — bám vào hình của phần mở cuộc là để khung người khác quyết định hộ.
func TestDispatchMarkedAnywhereBeforeFirstReply(t *testing.T) {
	raw := `{"messages":[
		{"role":"user","content":"<environment_info>\nmacOS\n</environment_info>"},
		{"role":"user","content":"IDE context follows\n<workspace_info>\n/W.O.N\n</workspace_info>"},
		{"role":"user","content":"<userRequest>` + DispatchMark + ` gọi — kiểm chất</userRequest>"}]}`
	if !mustBody(t, raw, FormatOpenAI).DispatchMarked(rules) {
		t.Error("mỏ neo trong phần mở cuộc phải được nhận ra, dù công cụ chủ tách mấy message")
	}
}

// Ranh dừng ở lời đáp đầu tiên: sau đó mỏ neo chỉ có thể là chữ được trích lại. Đây là cái
// giữ cửa khỏi nhận nhầm khi một đệ đọc trúng file có chứa chuỗi ấy giữa cuộc.
func TestDispatchStopsAtFirstAssistantAcrossFormats(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		f    Format
	}{
		{"openai", `{"messages":[
			{"role":"user","content":"cuộc này người mở"},
			{"role":"assistant","content":"ừ"},
			{"role":"user","content":"<userRequest>` + DispatchMark + ` gọi — chỉ đang trích</userRequest>"}]}`, FormatOpenAI},
		{"gemini", `{"contents":[
			{"role":"user","parts":[{"text":"cuộc này người mở"}]},
			{"role":"model","parts":[{"text":"ừ"}]},
			{"role":"user","parts":[{"text":"` + DispatchMark + ` gọi — chỉ đang trích"}]}]}`, FormatGemini},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if mustBody(t, c.raw, c.f).DispatchMarked(rules) {
				t.Errorf("%s: mỏ neo sau lời đáp đầu không được tính", c.name)
			}
		})
	}
}
