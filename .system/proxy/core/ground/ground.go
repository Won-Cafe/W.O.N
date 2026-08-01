// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

// Package ground dựng khối đất — chữ phải CÓ MẶT ở mọi request, ở cuối lời hệ thống,
// trước cả bản sắc (#0). Nội dung từ file người giữ hệ khai, không từ hằng trong code.
package ground

import (
	"os"
	"path/filepath"
	"strings"

	"won/proxy/core/paths"
)

// File — một nguồn đã đọc được. Rel thành nhãn nguồn trong khối đất, nên bên nhận
// truy được chữ này ở đâu ra (#4).
type File struct {
	Rel  string
	Text string
}

// Load nở mẫu, đọc file, trả về theo ĐÚNG thứ tự khai — thứ tự ấy nằm trong tiền tố
// cache. File vắng hoặc lỗi → vào miss, không chặn gì (#2).
func Load(tree paths.Tree, patterns []string) (files []File, miss []string) {
	seen := map[string]bool{}
	for _, pattern := range patterns {
		matches := paths.Expand(tree, pattern)
		if len(matches) == 0 {
			miss = append(miss, pattern)
			continue
		}
		for _, abs := range matches {
			if seen[abs] {
				continue // cùng một file khai hai lần vẫn chỉ vào ngữ cảnh một lần
			}
			seen[abs] = true
			b, err := os.ReadFile(abs)
			if err != nil {
				miss = append(miss, abs)
				continue
			}
			text := strings.TrimSpace(string(b))
			if text == "" {
				continue // file rỗng không phải lỗi, nhưng cũng không phải đất
			}
			files = append(files, File{Rel: rel(tree, abs), Text: text})
		}
	}
	return files, miss
}

// Text gộp các nguồn, mỗi nguồn một nhãn đường dẫn — kể cả khi chỉ có một nguồn, để
// hình dạng khối không đổi theo số file.
func Text(files []File) string {
	var sb strings.Builder
	for i, f := range files {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString("[" + f.Rel + "]\n" + f.Text)
	}
	return sb.String()
}

func rel(tree paths.Tree, abs string) string {
	r, err := filepath.Rel(tree.Root, abs)
	if err != nil {
		return abs // ngoài cây W.O.N — nói thẳng đường dẫn đầy đủ
	}
	return filepath.ToSlash(r)
}
