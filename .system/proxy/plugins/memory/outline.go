// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package memory

import (
	"strings"

	"won/proxy/core/request"
)

// outlineFallbackChars — trang không có dàn ý thì cắt chừng này chữ đầu. Chữ thật cắt
// ngang câu vẫn hơn một bản kê không nói gì.
const outlineFallbackChars = 400

// outline rút DÀN Ý của trang: mọi dòng heading Markdown, giữ nguyên bậc.
//
// Vì sao heading chứ không phải một dòng model viết: heading do NGƯỜI đặt, nên nó không
// bịa được và không diễn giải lệch được. Một bản tóm tắt sai đắt hơn một dàn ý thô.
//
// Trang chỉ có một heading thì dàn ý bằng đúng dòng đã nằm ở index — không thêm gì cho
// ai. Lúc ấy cắt phần đầu. Hàng rào ``` phải nhảy qua: `# ` trong khối mã là chú thích
// của một ngôn ngữ khác, không phải mục của trang.
func outline(text string) string {
	var heads []string
	inFence := false
	for _, ln := range strings.Split(text, "\n") {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "```") {
			inFence = !inFence
			continue
		}
		if inFence || !strings.HasPrefix(t, "#") {
			continue
		}
		if i := strings.IndexByte(t, ' '); i > 0 && strings.Trim(t[:i], "#") == "" {
			heads = append(heads, t)
		}
	}
	if len(heads) < 2 {
		return request.Truncate(text, outlineFallbackChars)
	}
	return strings.Join(heads, "\n")
}
