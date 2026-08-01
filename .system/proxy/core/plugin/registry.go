// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package plugin

import (
	"fmt"
	"sort"
)

// Factory dựng một plugin từ Env. Lỗi dựng là lỗi cấu hình, chặn ở khởi động.
type Factory func(env Env) (Plugin, error)

// factories — ngoại lệ có chủ đích: init() ghi đúng một lần lúc nạp, sau đó chỉ đọc.
var factories = map[string]Factory{}

// Register — plugin tự đăng ký trong init(). Trùng tên → panic, không ghi đè.
func Register(name string, f Factory) {
	if _, dup := factories[name]; dup {
		panic(fmt.Sprintf("plugin %q registered twice", name))
	}
	factories[name] = f
}

// Registered — mọi plugin biên dịch được vào binary này, theo thứ tự tên. Đây chính là
// bản quét thư mục `plugins/`: genplugins đọc thư mục ấy và chỉ sinh import cho package
// nào có gọi Register, nên `base` không có mặt vì nó không tự đăng ký. Đọc sau init là
// chỉ-đọc, cùng lý do với chính `factories`.
func Registered() []string {
	out := make([]string, 0, len(factories))
	for name := range factories {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func Build(name string, env Env) (Plugin, error) {
	f, ok := factories[name]
	if !ok {
		return nil, fmt.Errorf("plugin %q not registered — missing blank import in main?", name)
	}
	env.Name = name // tên đến từ chỗ dựng, không từ chỗ được dựng
	return f(env)
}
