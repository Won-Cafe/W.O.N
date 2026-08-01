// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package ground

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"won/proxy/core/paths"
)

func writeTree(t *testing.T, files map[string]string) paths.Tree {
	t.Helper()
	root := t.TempDir()
	for rel, body := range files {
		abs := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return paths.Tree{Root: root}
}

// Thứ tự khai là thứ tự đọc, và nó nằm trong tiền tố cache — nên nó phải bị khoá
// bằng test, không phải bằng lời hứa.
func TestLoadKeepsDeclaredOrder(t *testing.T) {
	tree := writeTree(t, map[string]string{
		"README.md":   "# đất",
		"Need/b.md":   "cần b",
		"Need/a.md":   "cần a",
		"Own/lost.md": "không ai gọi tên",
	})
	files, miss := Load(tree, []string{"./README.md", "Need/*.md"})
	if len(miss) != 0 {
		t.Fatalf("không thiếu gì mà báo thiếu: %v", miss)
	}
	want := []string{"README.md", "Need/a.md", "Need/b.md"} // trong một mẫu: bảng chữ cái
	if len(files) != len(want) {
		t.Fatalf("đọc %d file, muốn %d: %+v", len(files), len(want), files)
	}
	for i, w := range want {
		if files[i].Rel != w {
			t.Errorf("thứ tự [%d] = %q, muốn %q", i, files[i].Rel, w)
		}
	}
	if strings.Contains(Text(files), "không ai gọi tên") {
		t.Error("file không khớp mẫu nào không được vào đất")
	}
}

// Mỗi nguồn mang nhãn đường dẫn, kể cả khi chỉ có một nguồn: bên nhận truy được
// chữ này ở file nào (#4), và hình dạng khối không đổi theo số file.
func TestTextLabelsEverySource(t *testing.T) {
	one := Text([]File{{Rel: "README.md", Text: "# đất"}})
	if !strings.HasPrefix(one, "[README.md]\n# đất") {
		t.Errorf("một nguồn vẫn phải có nhãn: %q", one)
	}
	two := Text([]File{{Rel: "a.md", Text: "A"}, {Rel: "b.md", Text: "B"}})
	if !strings.Contains(two, "[a.md]\nA\n\n[b.md]\nB") {
		t.Errorf("hai nguồn, hai nhãn, cách nhau một dòng trống: %q", two)
	}
	if Text(nil) != "" {
		t.Error("không nguồn nào → rỗng, và lõi sẽ không chèn khối")
	}
}

// Mẫu không khớp gì thì khai ra rồi chạy tiếp — dòng chính không đứng vì một file
// thiếu (#2). File rỗng ruột không phải lỗi, nhưng cũng không phải đất.
func TestLoadFailsOpen(t *testing.T) {
	tree := writeTree(t, map[string]string{"README.md": "# đất", "Own/empty.md": "   \n"})
	files, miss := Load(tree, []string{"./README.md", "Need/*.md", "./không-có.md", "Own/empty.md"})
	if len(files) != 1 || files[0].Rel != "README.md" {
		t.Fatalf("chỉ README thành đất, got %+v", files)
	}
	if len(miss) != 2 {
		t.Errorf("hai mẫu không khớp phải được khai, got %v", miss)
	}
}

// Cùng một file khai hai lần vẫn chỉ vào ngữ cảnh một lần: trả tiền token hai lần
// cho một thứ là cái giá không mua gì.
func TestLoadDeduplicates(t *testing.T) {
	tree := writeTree(t, map[string]string{"README.md": "# đất"})
	files, _ := Load(tree, []string{"./README.md", "README.md", "*.md"})
	if len(files) != 1 {
		t.Errorf("muốn 1 file, got %d: %+v", len(files), files)
	}
}

// Thư mục khớp mẫu thì bỏ qua — Expand chỉ trả file.
func TestLoadSkipsDirectories(t *testing.T) {
	tree := writeTree(t, map[string]string{"Need/a.md": "x"})
	if files, _ := Load(tree, []string{"*"}); len(files) != 0 {
		t.Errorf("thư mục không phải nguồn đọc được: %+v", files)
	}
}

// `**` khớp số đoạn bất kỳ, kể cả không đoạn nào — đó là điều kiện để mặc định
// `What/**/*.md` lấy được cả trang nằm ngay dưới trục lẫn trang nằm trong cửa.
func TestLoadDeepGlob(t *testing.T) {
	tree := writeTree(t, map[string]string{
		"What/ngay-duoi.md":            "A",
		"What/World/World.md":          "B",
		"What/Threshold/sau/nua.md":    "C",
		"What/World/khong-phai-md.txt": "D",
		"Own/o.md":                     "E",
	})
	files, miss := Load(tree, []string{"What/**/*.md"})
	if len(miss) != 0 {
		t.Fatalf("mẫu khớp mà báo thiếu: %v", miss)
	}
	want := []string{"What/Threshold/sau/nua.md", "What/World/World.md", "What/ngay-duoi.md"}
	got := make([]string, len(files))
	for i, f := range files {
		got[i] = f.Rel
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %v, muốn %v (bảng chữ cái, chỉ .md, không ra ngoài trục)", got, want)
	}
}
