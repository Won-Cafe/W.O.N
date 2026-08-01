// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package plugin

import (
	"fmt"
	"sync"
)

// State — trạng thái một plugin. Note chỉ có mặt ở lời đáp của một lệnh đổi,
// không có trong /status: nó nói về hệ quả của lệnh vừa chạy.
type State struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Note    string `json:"note,omitempty"`
}

// Toggle giữ trạng thái bật/tắt nóng của plugin — control ghi, dòng chính đọc.
// Khởi động = mọi plugin đã dựng đều bật.
type Toggle struct {
	mu      sync.RWMutex
	enabled map[string]bool
}

// NewToggle khởi tạo toggle với mọi plugin truyền vào đều bật.
func NewToggle(plugins []Plugin) *Toggle {
	enabled := make(map[string]bool, len(plugins))
	for _, pl := range plugins {
		enabled[pl.Name()] = true
	}
	return &Toggle{enabled: enabled}
}

// Active trả plugin còn bật, giữ thứ tự dựng để chèn tất định.
func (t *Toggle) Active(plugins []Plugin) []Plugin {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]Plugin, 0, len(plugins))
	for _, pl := range plugins {
		if t.enabled[pl.Name()] {
			out = append(out, pl)
		}
	}
	return out
}

// Set bật/tắt nóng plugin đã dựng lúc khởi động. Tên lạ → lỗi.
func (t *Toggle) Set(name string, on bool) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.enabled[name]; !ok {
		return fmt.Errorf("plugin %q not in this process — to add, declare it in config then restart", name)
	}
	t.enabled[name] = on
	return nil
}

// States trả trạng thái mọi plugin theo thứ tự truyền vào — cho /status.
func (t *Toggle) States(plugins []Plugin) []State {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]State, 0, len(plugins))
	for _, pl := range plugins {
		out = append(out, State{Name: pl.Name(), Enabled: t.enabled[pl.Name()]})
	}
	return out
}
