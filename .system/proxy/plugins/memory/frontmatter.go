// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package memory

import (
	"strconv"
	"strings"
)

// splitFrontmatter cắt khối `---\n...\n---\n` đầu file. Không có hoặc không đóng
// khớp → trả nguyên văn, frontmatter rỗng. Chuẩn hoá CRLF → LF trước khi cắt.
func splitFrontmatter(text string) (body, frontmatter string) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return text, ""
	}
	rest := text[len("---\n"):]
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		return text, ""
	}
	return rest[end+len("\n---\n"):], rest[:end]
}

// parseFrontmatter rút s và f từ khối frontmatter. Dòng lạ bỏ qua, số sai → 0.
// Frontmatter rỗng → (0, 0).
func parseFrontmatter(fm string) (s, f int) {
	for _, ln := range strings.Split(fm, "\n") {
		ln = strings.TrimSpace(ln)
		key, val, ok := strings.Cut(ln, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		n, err := strconv.Atoi(val)
		if err != nil {
			continue
		}
		switch key {
		case "s":
			s = n
		case "f":
			f = n
		}
	}
	return s, f
}

// stone tính độ sỏi: (s−f)×weight, kẹp trong [0, 100]. Pure function.
func stone(s, f, weight int) int {
	v := (s - f) * weight
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}
