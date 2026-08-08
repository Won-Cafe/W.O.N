// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package request

import "strings"

// DispatchMark — mỏ neo mở đầu một lời giao việc trong dòng điều phối. Hai phần, và chỉ
// phần này là mỏ neo: một emoji cộng tên người điều phối. Phần chữ đứng sau nó
// ("gọi — <nhiệm vụ>") là văn, dịch được, và KHÔNG nằm trong phép khớp — đó là lý do mỏ
// neo có emoji: câu đổi sang tiếng khác thì mỏ neo vẫn nguyên.
//
// Cùng hình với mọi tiếng khác trong hệ (`<emoji> <Tên>:`), nên không phải khuôn thứ hai
// ai đó phải nhớ.
//
// HẰNG, không phải núm. Đây là giao thức nội bộ của hệ — cùng một chuỗi ở lõi, ở câu luật
// kênh rót cho đệ, và ở soul file. Nó không đổi theo cài đặt, nên một núm tại đây chỉ mở
// đường bẻ gãy từ vựng chung mà không đổi lại được gì. Khác hẳn chữ của CÔNG CỤ CHỦ: chữ
// ấy lõi không có quyền biết nên phải người khai (#6); chữ này là của chính hệ.
//
// Cũng vì thế nó không có đường tắt: cửa này chỉ khiến lõi bớt đoán, và một công tắc tắt
// nó là công tắc bảo lõi hãy đoán nhiều hơn.
const DispatchMark = "👋 Tzu"

// DispatchMarked — cuộc này có MỞ ĐẦU bằng một lời giao việc không.
//
// Chỉ xét LƯỢT NGƯỜI đầu tiên, và chỉ ở đầu chữ. Hai giới hạn ấy là chủ đích, không phải
// tiện: mỏ neo quét cả thân thì một đệ đọc trúng file có chứa chuỗi ấy — hay đọc chính
// đoạn hội thoại này — sẽ tự nhận mình là lượt giao việc. Lời giao đứng ở chỗ nó đứng:
// đầu cuộc, đầu câu.
//
// "Đầu cuộc" đo bằng LỜI ĐÁP ĐẦU TIÊN, không bằng message số 0 của thân. Một lời giao mở
// một cuộc, nên lúc nó đứng đó chưa ai đáp gì: mọi message vai user TRƯỚC lời đáp đầu tiên
// đều còn là phần mở cuộc, và mỏ neo được phép nằm ở bất cứ cái nào trong số đó. Thấy vai
// assistant là hết phần mở — dừng.
//
// Ranh ấy thay cho "message người đầu tiên", vì phần mở cuộc không phải của hệ: công cụ chủ
// chèn việc nhà của nó vào vai user (`<environment_info>`, `<workspace_info>`), tách thành
// mấy message, và gói câu người vào vỏ riêng (`<userRequest>`). Đo trên đúng message số 0 là
// để khung của người khác quyết định hệ có nhận ra lời giao hay không — mà một lượt được
// giao trượt cửa này thì rơi xuống `default_agent`, tức một tay việc được rót soul của người
// điều phối và đủ sức đóng vai người ấy trọn lượt.
//
// Hai bảo đảm cũ còn nguyên, và chúng là lý do ranh dừng ở lời đáp đầu:
//   - mỏ neo phải ở ĐẦU chữ (sau khi bỏ khung), nên chuỗi nằm giữa câu không tính;
//   - mỏ neo được TRÍCH LẠI về sau không tính, vì trích thì phải có ít nhất một lời đáp
//     đứng trước — đệ đọc trúng file chứa chuỗi ấy đang ở giữa cuộc, không ở đầu.
//
// Chưa khai vỏ nào ở `won.conf` thì `clean` không bóc gì và hàm trở về đúng lối cũ — cửa để
// BỚT việc đoán, không phải để thêm một phép đoán (#6).
//
// Đi qua `messages`/`contentOf`/`flatten` nên đúng ở cả ba định dạng — mỏ neo nằm trong
// chữ hội thoại, không nằm trong trường riêng của nhà nào.
func (b *Body) DispatchMarked(rules FrameRules) bool {
	for _, msg := range b.messages() {
		switch b.normRole(b.roleOfMsg(msg)) {
		case RoleAssistant:
			return false // đã có lời đáp: từ đây trở đi mỏ neo là chữ được nhắc lại
		case RoleUser:
			if strings.HasPrefix(rules.clean(b.flatten(b.contentOf(msg))), DispatchMark) {
				return true
			}
		}
	}
	return false
}
