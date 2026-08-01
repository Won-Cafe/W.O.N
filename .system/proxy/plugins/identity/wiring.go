// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package identity

import (
	"strings"

	"won/proxy/core/request"
)

// Luật kênh — chính sách của identity, không phải hạ tầng: nó nói một đệ Circle
// phải xử sự thế nào trong hai kênh, và mở lời trả về bằng nhãn nào. Ai giữ vai
// gì thì nằm ở bản đồ hệ (House.md) — chép lại đây là hai bản có thể lệch.

// labels — nhãn đầu ra của từng đệ Circle. Chỉ nhãn: vai đọc ở bản đồ hệ. Tên
// không có trong bảng = không thuộc Circle → không có luật kênh.
var labels = map[string]string{
	"Tzu": "", // điều phối: không có nhãn riêng, cửa vào là cửa của người
	"Sun": "🗺️ Báo cáo trinh sát",
	"Mo":  "🏛️ Báo cáo kiến trúc",
	"Fan": "♟️ Kế hoạch chiến lược",
	"Han": "⚖️ Phán quyết",
	"Shu": "📜 Bản ghi",
}

// wiring dựng luật kênh rót vào phiên của một đệ. Ngoài Circle hoặc không nhận ra
// căn cước → rỗng (#6).
func wiring(agent string) string {
	name, label, ok := lookup(agent)
	if !ok {
		return "" // agent nền hoặc tên lạ — không thuộc Circle
	}

	var sb strings.Builder
	sb.WriteString("---\n\n## Luật kênh — tầng cơ học rót vào, không thuộc soul\n\n")
	sb.WriteString("Bản đồ hệ đứng ở khối trên; đây là phần của riêng đệ " + name + ".\n")

	// Mỏ neo dựng TỪ hằng của lõi, không chép tay: lõi khớp đúng chuỗi này để biết một cuộc
	// là cuộc được giao, nên hai bên lệch nhau là đệ được dạy một dấu mà lõi không đọc.
	// Phần chữ đứng sau mỏ neo là văn — dịch được, và không nằm trong phép khớp.
	sb.WriteString("\n**Hai kênh**, phân biệt bằng dấu ở dòng đầu:\n")
	sb.WriteString("- `" + request.DispatchMark + " gọi — <nhiệm vụ>`: đang đứng trong dòng điều phối. Trả **dữ liệu nghề** về Tzu, không diễn lời với người. Liều nằm trong lời giao — dò nhanh hay dò kỹ, phạm vi tới đâu; đệ không tự nới liều, thấy liều chọi với ý định thì hỏi lại Tzu.\n")
	sb.WriteString("- Không dấu: đang đứng trước người thật. Nói tiếng của họ, lời đẩy áp toàn phần; không thay Tzu điều phối.\n")

	if label != "" {
		sb.WriteString("\n**Nhãn đầu ra của đệ " + name + "**: mở phần trả về bằng `" + label + "` để Tzu gom được mà không phải đoán nguồn.\n")
	} else {
		sb.WriteString("\n**Điều phối một tay đệ Tzu**: đệ không gọi nhau; việc nào người nấy, và Tzu không tự thi hành chuyên môn khi có đệ phù hợp.\n")
	}

	sb.WriteString("\n**Ghi — luôn qua đệ Shu**: mọi chữ đặt vào trục (`What/` `Own/` `Need/`) hay Memory đều do Shu cầm bút. Cần đệ khác thì gọi tên và chỉ đường, không gọi thay Tzu. Không gọi được đệ thì nói rõ, rồi trao chữ cho người tự đặt bút — trục là text của người.\n")
	return sb.String()
}

// lookup khớp tên không phân biệt hoa thường, trả lại tên đúng chính tả của bảng.
func lookup(agent string) (name, label string, ok bool) {
	if agent == "" {
		return "", "", false
	}
	for n, l := range labels {
		if strings.EqualFold(n, agent) {
			return n, l, true
		}
	}
	return "", "", false
}
