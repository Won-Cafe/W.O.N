// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package request

import (
	"strings"
	"testing"
)

// Cắt theo RUNE, không theo byte: cắt theo byte thì một chữ tiếng Việt hay một emoji bị
// chặt đôi ngay tại mép, và mọi chỗ cắt chữ trong hệ đi qua đúng hàm này.
func TestTruncate(t *testing.T) {
	short := "một hai ba"
	if got := Truncate(short, 20); got != short {
		t.Errorf("ngắn hơn trần thì giữ nguyên, got %q", got)
	}
	long := strings.Repeat("đường", 10) // 50 rune, 100 byte
	got := Truncate(long, 10)
	if r := []rune(strings.TrimSuffix(got, "...")); len(r) != 10 {
		t.Errorf("muốn 10 rune, got %d: %q", len(r), got)
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("phải khai là đã cắt: %q", got)
	}
	if got := Truncate("🌌🚶📖", 2); got != "🌌🚶..." {
		t.Errorf("emoji bị chặt đôi: %q", got)
	}
}

// TrimMarkers bóc dấu trang trí ở ĐẦU dòng — marker của đệ khác đứng đầu dòng là dấu
// không thuộc về dòng ấy. Chữ giữa dòng thì không chạm.
func TestTrimMarkers(t *testing.T) {
	cases := map[string]string{
		"🌌 Tzu: kìa":      "Tzu: kìa",
		"## Tiêu đề":      "Tiêu đề",
		"- gạch đầu dòng": "gạch đầu dòng",
		"  > dẫn lời":     "dẫn lời",
		"chữ thường":      "chữ thường",
		"a - b":           "a - b", // dấu giữa dòng không phải marker
	}
	for in, want := range cases {
		if got := TrimMarkers(in); got != want {
			t.Errorf("TrimMarkers(%q) = %q, muốn %q", in, got, want)
		}
	}
}
