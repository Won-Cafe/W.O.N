// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package request

// Header điều khiển nội bộ — bị gỡ trước khi request rời hệ.
//
// Ba cái đầu thuộc mặt DỮ LIỆU: chúng nói về lượt đang đi qua — ai đứng lên nó, nó thuộc
// cuộc nào, nó đi tới đâu. Cái thứ tư thuộc mặt CONTROL và không bao giờ chạm một lượt nào.
const (
	HeaderAgent    = "X-WON-Agent"
	HeaderSession  = "X-WON-Session"
	HeaderUpstream = "X-WON-Upstream"
	// HeaderControl — mặt control tự khai nó là MỤC nào (`upstream`, `plugin`, `config`…).
	// Chỉ Control API đọc, và chỉ để nhật ký nói được ai vừa vặn núm: control không có auth,
	// vành đai duy nhất là loopback, nên "lệnh này đến từ đâu" là câu duy nhất còn trả lời
	// được. KHÔNG phải căn cước và không cấp quyền gì — vắng nó thì mọi núm vẫn vặn được.
	HeaderControl = "X-WON-Control"
)

// InternalHeaders — trọn danh sách trên, cho chỗ GỠ chúng khỏi request đi ra. Một nhà, vì
// chép danh sách ở chỗ khai và chỗ gỡ là thêm một header rồi quên gỡ nó — và quên gỡ nghĩa
// là header nội bộ của hệ đi tới nhà cung cấp.
//
// Trả BẢN SAO: người gọi giữ nó suốt đời tiến trình, và một slice dùng chung thì một lần
// append của họ ghi vào hằng số của lõi.
func InternalHeaders() []string {
	return []string{HeaderAgent, HeaderSession, HeaderUpstream, HeaderControl}
}

// AgentOrUnknown — căn cước rỗng thì NÓI là rỗng, không đoán (#6). Một nhãn, một nhà:
// nhật ký chẩn bệnh và prompt của model nền đều gọi tên đệ, và hai nhãn khác nhau cho
// cùng một chuyện rỗng thì đọc ra hai chuyện.
func AgentOrUnknown(s string) string {
	if s == "" {
		return "(unknown)"
	}
	return s
}
