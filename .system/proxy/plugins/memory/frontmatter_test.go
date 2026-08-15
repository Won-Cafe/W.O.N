// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package memory

import "testing"

func TestSplitFrontmatter(t *testing.T) {
	// Có frontmatter → cắt đúng.
	in := "---\ns: 3\nf: 1\n---\n# Title\n\nBody.\n"
	body, fm := splitFrontmatter(in)
	if body != "# Title\n\nBody.\n" {
		t.Errorf("body sai: %q", body)
	}
	if fm != "s: 3\nf: 1" {
		t.Errorf("frontmatter sai: %q", fm)
	}
	// Không có → nguyên văn.
	plain := "# No frontmatter\n\nBody.\n"
	body, fm = splitFrontmatter(plain)
	if body != plain || fm != "" {
		t.Errorf("không có frontmatter phải trả nguyên văn: body=%q fm=%q", body, fm)
	}
	// Không đóng → trả nguyên văn.
	unclosed := "---\ns: 3\nf: 1\n# Title\n"
	body, fm = splitFrontmatter(unclosed)
	if body != unclosed || fm != "" {
		t.Errorf("frontmatter không đóng phải trả nguyên văn: body=%q fm=%q", body, fm)
	}
	// CRLF → chuẩn hoá LF rồi cắt đúng.
	crlf := "---\r\ns: 3\r\nf: 1\r\n---\r\n# Title\r\n\r\nBody.\r\n"
	body, fm = splitFrontmatter(crlf)
	if body != "# Title\n\nBody.\n" {
		t.Errorf("CRLF body sai: %q", body)
	}
	if fm != "s: 3\nf: 1" {
		t.Errorf("CRLF frontmatter sai: %q", fm)
	}
}

func TestParseFrontmatter(t *testing.T) {
	s, f := parseFrontmatter("s: 3\nf: 1")
	if s != 3 || f != 1 {
		t.Errorf("rút sai: s=%d f=%d", s, f)
	}
	// Rỗng → (0, 0).
	s, f = parseFrontmatter("")
	if s != 0 || f != 0 {
		t.Errorf("rỗng phải (0,0): s=%d f=%d", s, f)
	}
	// Dòng lạ bỏ qua, số sai → 0.
	s, f = parseFrontmatter("s: abc\nf: 2\nweird: 5")
	if s != 0 || f != 2 {
		t.Errorf("dòng lạ phải bỏ qua: s=%d f=%d", s, f)
	}
}

func TestStone(t *testing.T) {
	cases := []struct{ s, f, w, want int }{
		{3, 1, 10, 20},
		{0, 0, 10, 0},
		{10, 0, 10, 100},
		{0, 10, 10, 0},
		{1, 5, 10, 0},    // f > s → 0
		{20, 0, 10, 100}, // > 100 → kẹp
	}
	for _, c := range cases {
		if got := stone(c.s, c.f, c.w); got != c.want {
			t.Errorf("stone(%d,%d,%d) = %d, want %d", c.s, c.f, c.w, got, c.want)
		}
	}
}
