// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

// Package cockpit giữ trang điều khiển, nhúng vào binary.
//
// Nhúng chứ không đọc đĩa: trang phải CÙNG GỐC với Control API mới đọc được lời đáp
// (không có CORS, và mở CORS ra thì mọi web người dùng ghé đều vặn được núm — control
// không có auth, loopback là vành đai duy nhất). Nhúng cũng bỏ luôn phụ thuộc đường dẫn:
// chạy từ thư mục nào, hay bản build mang đi đâu, trang vẫn có.
package cockpit

import _ "embed"

//go:embed control.html
var page []byte

// Page — trang điều khiển. Trả bản sao vì []byte nhúng là bộ nhớ chung của tiến trình.
func Page() []byte {
	out := make([]byte, len(page))
	copy(out, page)
	return out
}
