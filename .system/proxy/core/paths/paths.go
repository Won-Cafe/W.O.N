// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

// Package paths gọi tên chỗ mọi thứ nằm trong cây W.O.N. Một bố cục một nhà:
// đường dẫn chép ở nhiều package thì đổi bố cục là đi sửa từng chỗ, và chỗ nào
// quên là chỗ đó đọc vào thư mục không có.
package paths

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Marker — thư mục nhận ra gốc cây. Dò ngược tới nó là dò được gốc.
const Marker = ".system"

// Bốn vùng của kho ký ức. Ai cần gọi tên một vùng cụ thể thì đọc ở đây, không gõ lại
// chuỗi: gõ lại là đổi tên vùng xong vẫn biên dịch được mà đọc vào thư mục không có.
const (
	ZoneWorking    = "working"
	ZoneProcedural = "procedural"
	ZonePersonal   = "personal"
	ZoneMoments    = "moments"
)

// Zones — bốn vùng theo thứ tự đọc. Luật ở cửa kho nằm trong README của kho, không ở
// đây: đây chỉ nói vùng nằm đâu.
var Zones = []string{ZoneWorking, ZoneMoments, ZoneProcedural, ZonePersonal}

// TemplatePrefix — file mẫu nằm cùng thư mục với trang thật nhưng không phải trang.
const TemplatePrefix = "template-"

// IsPage — tên tệp này có phải một trang thật của kho không. Thuần tên, không chạm đĩa:
// người gọi đã có DirEntry nên nó tự biết cái gì là thư mục.
//
// Luật này ở đây vì nó là hình của kho, và nó có HAI chỗ đọc — bộ dựng index và kẻ đo
// đường. Chép sang nhà thứ hai là hai bản lệch được: một bên bỏ file mẫu, bên kia khai
// `template-working.md` là một việc đang mở.
func IsPage(name string) bool {
	return strings.HasSuffix(name, ".md") && !strings.HasPrefix(name, TemplatePrefix)
}

// Axis — ba trụ của W.O.N, mỗi trụ một thư mục dưới gốc. Region đọc chúng, và
// bất kỳ ai cần gọi tên chúng cũng đọc ở đây: chép tay lần thứ hai là bản thứ hai.
var Axis = []string{"What", "Own", "Need"}

// Readme — đất mặc định mọi đệ đứng trên khi cấu hình không khai gì khác. Cùng nhà với
// Axis vì cùng một vai: hai thứ hợp thành đất mặc định.
const Readme = "README.md"

// Tree neo mọi đường dẫn vào một gốc. Root rỗng = chưa biết gốc; người gọi kiểm
// bằng Known() trước khi dựng thứ cần đọc đĩa.
type Tree struct{ Root string }

func (t Tree) Known() bool { return t.Root != "" }

// Agents — soul file của mọi đệ, và bản đồ hệ.
func (t Tree) Agents() string { return filepath.Join(t.Root, Marker, "agents") }

// Memory — kho ký ức; Zone là một vùng trong kho.
func (t Tree) Memory() string          { return filepath.Join(t.Root, Marker, "memory") }
func (t Tree) Zone(zone string) string { return filepath.Join(t.Memory(), zone) }

// Proxy — nhà của tầng cơ học: won.conf và thư mục runtime.
func (t Tree) Proxy() string { return filepath.Join(t.Root, Marker, "proxy") }
func (t Tree) Conf() string  { return filepath.Join(t.Proxy(), "won.conf") }
func (t Tree) Run() string   { return filepath.Join(t.Proxy(), "run") }
func (t Tree) State() string { return filepath.Join(t.Run(), "state.json") }

// Threshold — ngưỡng của hệ, Wayfarer đọc để biết cái gì đáng thành mốc.
func (t Tree) Threshold() string { return filepath.Join(t.Root, "What", "Threshold", "Threshold.md") }

// Vùng của cây — tên chỗ, không phải lời phán về chỗ. Ai đọc ra nghĩa "chạm vào
// đây là làm thay người" thì đó là việc của soul; đây chỉ nói đường dẫn nằm ở đâu.
const (
	RegionAxis    = "trục W.O.N"   // What/ Own/ Need/ — bút của đệ Shu
	RegionMemory  = "kho ký ức"    // .system/memory/ — cũng bút của Shu
	RegionSystem  = "tầng cơ học"  // .system/ còn lại
	RegionOutside = "ngoài cây"    // không nằm dưới gốc
	RegionUnknown = "không rõ chỗ" // đường dẫn rỗng hoặc không đọc ra được
)

// Region gọi tên vùng mà một đường dẫn rơi vào. Ngoài các vùng có tên thì trả về
// thư mục đầu tiên dưới gốc — tên thật của chỗ đó, không gán nghĩa gì thêm.
func (t Tree) Region(p string) string {
	if strings.TrimSpace(p) == "" {
		return RegionUnknown
	}
	abs := p
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(t.Root, p)
	}
	r, err := filepath.Rel(t.Root, abs)
	if err != nil || strings.HasPrefix(r, "..") {
		return RegionOutside
	}
	parts := strings.Split(filepath.ToSlash(r), "/")
	for _, a := range Axis {
		if parts[0] == a {
			return RegionAxis
		}
	}
	switch parts[0] {
	case Marker:
		if len(parts) > 1 && parts[1] == "memory" {
			return RegionMemory
		}
		return RegionSystem
	case ".":
		return RegionUnknown
	default:
		return parts[0] + "/"
	}
}

// Expand nở một mẫu thành các file có thật, neo vào gốc cây. `**` khớp số đoạn bất kỳ,
// kể cả không đoạn nào: `What/**/*.md` lấy cả `What/a.md` lẫn `What/World/World.md`.
// Chỉ trả file. Thứ tự bảng chữ cái — tất định, vì tiền tố cache cần thế.
func Expand(tree Tree, pattern string) []string {
	pattern = strings.TrimPrefix(filepath.ToSlash(pattern), "./")
	if !filepath.IsAbs(filepath.FromSlash(pattern)) {
		pattern = filepath.ToSlash(filepath.Join(tree.Root, pattern))
	}
	if strings.Contains(pattern, "**") {
		return expandDeep(pattern)
	}
	matches, err := filepath.Glob(filepath.FromSlash(pattern))
	if err != nil {
		return nil // mẫu sai cú pháp — người gọi khai ra như một mẫu không khớp
	}
	return onlyFiles(matches)
}

// expandDeep đi bộ từ phần chữ cố định trước `**` rồi khớp từng đường dẫn thật.
func expandDeep(pattern string) []string {
	segs := strings.Split(pattern, "/")
	root := ""
	for _, s := range segs {
		if strings.ContainsAny(s, "*?[") {
			break
		}
		root = path.Join(root, s)
	}
	if root == "" {
		return nil
	}
	if strings.HasPrefix(pattern, "/") {
		root = "/" + root
	}
	var out []string
	filepath.WalkDir(filepath.FromSlash(root), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil // thư mục không đọc được thì bỏ qua nhánh đó, không chặn cả mẫu
		}
		if matchSegs(segs, strings.Split(filepath.ToSlash(p), "/")) {
			out = append(out, p)
		}
		return nil
	})
	return out
}

// matchSegs khớp theo từng đoạn; `**` ăn 0 hoặc nhiều đoạn, thử dần từ ăn-không-đoạn.
func matchSegs(pat, p []string) bool {
	switch {
	case len(pat) == 0:
		return len(p) == 0
	case pat[0] == "**":
		for i := 0; i <= len(p); i++ {
			if matchSegs(pat[1:], p[i:]) {
				return true
			}
		}
		return false
	case len(p) == 0:
		return false
	default:
		ok, err := path.Match(pat[0], p[0])
		return err == nil && ok && matchSegs(pat[1:], p[1:])
	}
}

func onlyFiles(in []string) []string {
	out := make([]string, 0, len(in))
	for _, m := range in {
		if fi, err := os.Stat(m); err == nil && !fi.IsDir() {
			out = append(out, m)
		}
	}
	return out
}

// Find dò gốc: đi ngược từ dir tới thư mục chứa Marker. Không thấy → dir.
func Find(dir string) string {
	for d := dir; ; {
		if fi, err := os.Stat(filepath.Join(d, Marker)); err == nil && fi.IsDir() {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			return dir
		}
		d = parent
	}
}
