// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package soul

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeAgents(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// Nạp: mỗi `*.agent.md` một đệ, tiêu đề là dòng `# …` đầu, template bị bỏ.
func TestLoadReadsSoulsAndTitles(t *testing.T) {
	dir := writeAgents(t, map[string]string{
		"Tzu.agent.md":      "# Tzu - 🌌 The Orchestrator\nKênh của tôi có hai đầu.",
		"Sun.agent.md":      "# Sun - 🗺️ The Scout\nTôi đo đất.",
		"template.agent.md": "# template\nkhông phải một đệ",
		"House.md":          "Bản đồ hệ W.O.N — ai giữ vai gì.",
		"README.md":         "không phải soul",
	})
	b, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := b.Names(); len(got) != 2 || got[0] != "Sun" || got[1] != "Tzu" {
		t.Fatalf("names = %v (phải sắp ổn định, bỏ template)", got)
	}
	if !strings.HasPrefix(b.Soul("Tzu"), "# Tzu") {
		t.Errorf("soul Tzu = %q", b.Soul("Tzu"))
	}
	if b.Title("Tzu") != "# Tzu - 🌌 The Orchestrator" {
		t.Errorf("title Tzu = %q", b.Title("Tzu"))
	}
	if !strings.Contains(b.House(), "Bản đồ hệ") {
		t.Errorf("bản đồ hệ phải đọc từ House.md, got %q", b.House())
	}
}

// Vắng House.md → bản đồ rỗng, và lõi sẽ không chèn khối nào. Bản đồ là lời khai
// của người giữ hệ; hằng số trong code thì vẫn khai kể cả khi không còn ai (#6).
func TestHouseEmptyWithoutFile(t *testing.T) {
	b, err := Load(writeAgents(t, map[string]string{"Tzu.agent.md": "# Tzu\nx"}))
	if err != nil {
		t.Fatal(err)
	}
	if b.House() != "" {
		t.Errorf("vắng House.md phải trả rỗng, got %q", b.House())
	}
	if Empty().House() != "" {
		t.Error("sổ rỗng cũng không được có bản đồ")
	}
}

// Nhận diện: header thắng, không có thì khớp tiêu đề trong lời hệ thống, không
// khớp thì rỗng — không đoán (#6).
func TestResolve(t *testing.T) {
	b, err := Load(writeAgents(t, map[string]string{
		"Tzu.agent.md": "# Tzu - 🌌 The Orchestrator\nx",
		"Sun.agent.md": "# Sun - 🗺️ The Scout\ny",
	}))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct{ header, system, want string }{
		{"Sun", "", "Sun"},
		{"sun", "", "Sun"}, // hoa thường không phải căn cước khác
		{"", "lời dặn\n# Tzu - 🌌 The Orchestrator\n...", "Tzu"}, // khớp tiêu đề
		{"", "một system prompt không mang tiêu đề nào", ""},    // không đoán
		{"tên lạ", "# Tzu - 🌌 The Orchestrator", "Tzu"},         // header sai → còn đường tiêu đề
	}
	for _, c := range cases {
		if got := b.Resolve(c.header, c.system); got != c.want {
			t.Errorf("Resolve(%q, …) = %q, muốn %q", c.header, got, c.want)
		}
	}
}

// Thư mục agents không có → lỗi, và main chạy tiếp không căn cước (#2).
func TestLoadMissingDir(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "không-có")); err == nil {
		t.Error("thư mục vắng phải trả lỗi để main tự quyết")
	}
}
