// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package request

import (
	"strings"
	"unicode"
)

// Truncate cắt theo rune — an toàn với tiếng Việt và emoji. Mọi chỗ cắt chữ
// trong hệ đi qua đây; cắt theo byte thì một rune bị chặt đôi.
func Truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}

// TrimMarkers bóc dấu trang trí ở ĐẦU dòng: emoji marker, dấu tiêu đề, gạch đầu dòng.
// Một nhà, hai chỗ dùng — bản sao hội thoại đưa cho model nền, và dòng nó trả về. Cả hai
// chỗ đều vì một lý do: marker của đệ khác đứng đầu dòng là dấu không thuộc về dòng ấy
// (§ Marker do lõi đeo).
func TrimMarkers(s string) string {
	return strings.TrimLeftFunc(strings.TrimSpace(s), func(r rune) bool {
		switch r {
		case '#', '>', '*', '-', '+', 0xFE0F, 0x200D: // hai mã cuối: đuôi của emoji ghép
			return true
		}
		return unicode.IsSpace(r) || unicode.Is(unicode.So, r) || unicode.Is(unicode.Sk, r)
	})
}
