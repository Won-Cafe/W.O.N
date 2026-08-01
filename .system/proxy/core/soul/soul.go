// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

// Package soul nạp soul file của các đệ và nhận ra request này của ai.
// Chỉ đọc file và so khớp — không bảng vai, không luật kênh: những thứ đó là
// chính sách, sống ở plugin (identity) hoặc ở file (House.md).
package soul

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ServiceName — khoá trong Hub để plugin lấy sổ ra.
const ServiceName = "soul"

const (
	suffix    = ".agent.md" // soul file của một đệ
	houseFile = "House.md"  // bản đồ hệ: ai giữ vai gì, giống mọi đệ
)

// Book — soul đã nạp, tiêu đề của từng soul, và bản đồ hệ. Rỗng là trạng thái
// bình thường: không có file thì không nhận ra ai, và không khối nào được chèn.
type Book struct {
	house  string
	names  []string
	souls  map[string]string
	titles map[string]string
}

func Empty() *Book {
	return &Book{souls: map[string]string{}, titles: map[string]string{}}
}

// Load đọc thư mục agents: mỗi `*.agent.md` là một đệ, `House.md` là bản đồ hệ.
// Một file hỏng không chặn cả sổ (#2).
func Load(dir string) (*Book, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	b := Empty()
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, suffix) {
			continue
		}
		base := strings.TrimSuffix(name, suffix)
		if strings.EqualFold(base, "template") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		text := string(content)
		b.souls[base] = text
		if i := strings.IndexByte(text, '\n'); i > 0 && strings.HasPrefix(text, "# ") {
			b.titles[base] = strings.TrimSpace(text[:i])
		}
		b.names = append(b.names, base)
	}
	sort.Strings(b.names)
	if h, err := os.ReadFile(filepath.Join(dir, houseFile)); err == nil {
		b.house = strings.TrimSpace(string(h))
	}
	return b, nil
}

func (b *Book) Names() []string          { return append([]string(nil), b.names...) }
func (b *Book) Soul(name string) string  { return b.souls[name] }
func (b *Book) Title(name string) string { return b.titles[name] }

// House — bản đồ hệ, nguyên văn từ House.md. Vắng file → rỗng, và lõi không chèn
// khối nào: bản đồ là lời khai của người giữ hệ, không phải hằng số trong code (#6).
func (b *Book) House() string { return b.house }

// Resolve — request này của đệ nào. Header là nguồn chắc nhất; không có thì nhận
// ra bằng tiêu đề soul nằm trong lời hệ thống. Không khớp → rỗng, không đoán (#6).
func (b *Book) Resolve(headerAgent, systemText string) string {
	if headerAgent != "" {
		for _, n := range b.names {
			if strings.EqualFold(n, headerAgent) {
				return n
			}
		}
	}
	for _, n := range b.names {
		if t := b.titles[n]; t != "" && strings.Contains(systemText, t) {
			return n
		}
	}
	return ""
}
