// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package paths

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Mọi đường dẫn neo vào một gốc — đó là lý do package này tồn tại.
func TestEverythingUnderRoot(t *testing.T) {
	tr := Tree{Root: filepath.Join("x", "won")}
	for name, got := range map[string]string{
		"Agents":    tr.Agents(),
		"Memory":    tr.Memory(),
		"Zone":      tr.Zone("working"),
		"Conf":      tr.Conf(),
		"Run":       tr.Run(),
		"State":     tr.State(),
		"Threshold": tr.Threshold(),
	} {
		if !strings.HasPrefix(got, tr.Root) {
			t.Errorf("%s() = %q, phải neo vào gốc %q", name, got, tr.Root)
		}
	}
	if !tr.Known() || (Tree{}).Known() {
		t.Error("Known() phải nói đúng chuyện đã biết gốc hay chưa")
	}
}

// Find đi ngược tới thư mục chứa `.system/` — chạy binary từ đâu cũng ra cùng gốc.
func TestFindWalksUp(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(filepath.Join(root, Marker), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := Find(deep); got != root {
		t.Errorf("Find(%q) = %q, muốn %q", deep, got, root)
	}
}

// Không có `.system/` ở đâu cả → trả về chính chỗ đang đứng, không leo tới ổ đĩa.
func TestFindNoMarkerStaysPut(t *testing.T) {
	dir := t.TempDir()
	if got := Find(dir); got != dir {
		t.Errorf("Find(%q) = %q, muốn giữ nguyên", dir, got)
	}
}

// Region gọi tên CHỖ, không phán về chỗ. Đây là dữ kiện Outfitter cần để phân biệt
// ghi trục với làm thay người — nhưng phân biệt là việc của soul.
func TestRegion(t *testing.T) {
	tr := Tree{Root: t.TempDir()} // gốc phải tuyệt đối, đúng như lúc chạy thật
	cases := map[string]string{
		"":                            RegionUnknown,
		"What/Threshold/Threshold.md": RegionAxis,
		"Own/a.md":                    RegionAxis,
		"Need/b.md":                   RegionAxis,
		".system/memory/working/x.md": RegionMemory,
		".system/proxy/main.go":       RegionSystem,
		".system/agents/Tzu.agent.md": RegionSystem,
		"Stories/Adventure.md":        "Stories/",
		// Ngoài gốc — đường dẫn tuyệt đối ở thư mục cha.
		filepath.Join(filepath.Dir(tr.Root), "khac.md"): RegionOutside,
		// Trong gốc, dạng tuyệt đối: vẫn phải đọc ra đúng vùng.
		filepath.Join(tr.Root, ".system", "memory", "personal", "a.md"): RegionMemory,
	}
	// `\` là dấu tách đoạn trên Windows, nhưng trên Unix nó là một BYTE HỢP LỆ trong tên
	// file. Region cắt theo `filepath` của hệ ĐANG CHẠY, nên cùng một chuỗi ra hai kết quả
	// và cả hai đều đúng ở chỗ nó chạy — hệ này là hai máy, Windows và Mac. Cái KHÔNG được
	// làm là tự cắt thêm theo backslash trên Unix: khi ấy một file tên thật là `Kho\a.md`
	// bị đọc thành hai đoạn và gán sai vùng.
	if filepath.Separator == '\\' {
		cases[`Stories\Adventure.md`] = "Stories/"
	} else {
		cases[`Stories\Adventure.md`] = `Stories\Adventure.md/`
	}
	for in, want := range cases {
		if got := tr.Region(in); got != want {
			t.Errorf("Region(%q) = %q, muốn %q", in, got, want)
		}
	}
}

// Expand nở mẫu tương đối từ gốc, bỏ thư mục, và tất định.
func TestExpand(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{"README.md", "Need/a.md", "Need/b.md"} {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tr := Tree{Root: root}
	if got := Expand(tr, "./README.md"); len(got) != 1 {
		t.Errorf("mẫu `./` phải tính từ gốc: %v", got)
	}
	if got := Expand(tr, "Need/*.md"); len(got) != 2 {
		t.Errorf("glob phải nở: %v", got)
	}
	if got := Expand(tr, "không-có/*.md"); len(got) != 0 {
		t.Errorf("không khớp → rỗng: %v", got)
	}
	if got := Expand(tr, "["); len(got) != 0 {
		t.Errorf("mẫu sai cú pháp → rỗng, không panic: %v", got)
	}
}

// File mẫu nằm cùng thư mục với trang thật nhưng không phải trang. Luật ở đây vì nó có
// hai chỗ đọc — bộ dựng index và kẻ đo đường — và lần lệch đầu đã xảy ra thật.
func TestIsPage(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"2026-07-19-promise.md", true},
		{"self.md", true},
		{"template-working.md", false},
		{"template-procedural.md", false},
		{"README", false},
		{"notes.txt", false},
	}
	for _, c := range cases {
		if got := IsPage(c.name); got != c.want {
			t.Errorf("IsPage(%q) = %v, muốn %v", c.name, got, c.want)
		}
	}
}
