// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

// Package proxy là lớp duy nhất ngồi giữa các đệ và Model API: nhận request,
// nhận diện căn cước, cho plugin đóng góp, chèn theo thứ tự bất biến, rồi chuyển
// tiếp — response đi qua nguyên vẹn.
package proxy

import (
	"time"

	"won/proxy/core/plugin"
	"won/proxy/core/proxy/debug"
	"won/proxy/core/request"
	"won/proxy/core/session"
	"won/proxy/core/soul"
	"won/proxy/core/upstream"
)

// Cấu hình luôn có một đích — mặc định là chính máy này — nên dòng này chỉ tới được khi
// lõi bị dựng mà không ai đưa đích nào. Lúc ấy vẫn từ chối rõ thay vì đoán (#6).
const msgNoUpstream = "Proxy Inject: no target given. Declare one via header X-WON-Upstream (per request), flag -upstream or won.conf (startup), or PUT /upstream via Control API."

// Deps là mọi thứ Proxy cần, dựng sẵn từ main — Proxy không tự đọc config.
type Deps struct {
	// Chữ chèn vào lời hệ thống, theo thứ tự đọc: đất → nhà → (bản sắc do plugin).
	Ground    string // đất mọi đệ đứng, đã gộp từ các file khai trong `ground`. Rỗng → không chèn
	House     string // bản đồ hệ — giống mọi đệ, đứng giữa đất và bản sắc
	Workspace string // gốc W.O.N, đi cùng House — đệ không tự biết thư mục

	DebugDir string // rỗng → nhật ký chẩn bệnh tắt

	Store   *session.Store
	Plugins []plugin.Plugin
	// Identity — lõi chỉ cần: request này của đệ nào. Không nhận ra → rỗng (#6).
	Identity soul.IdentityResolver
	// DefaultAgent — đệ dùng khi không nhận ra ai. Rỗng = chuyển tiếp nguyên bản như cũ.
	DefaultAgent string
	TotalBudget  time.Duration

	// Up — chuỗi resolve đích, dựng sẵn từ main.
	Up *upstream.Upstreams
	// Toggle — trạng thái bật/tắt nóng, dựng sẵn từ main.
	Toggle *plugin.Toggle

	// FrameRules — khung công cụ chủ nào được cắt. Rỗng = không cắt gì.
	FrameRules request.FrameRules
}

type Proxy struct {
	d     Deps
	up    *upstream.Upstreams
	start time.Time
	debug *debug.Log // nil = nhật ký chẩn bệnh tắt

	toggle *plugin.Toggle
}

func New(d Deps) (*Proxy, error) {
	return &Proxy{d: d, up: d.Up, start: time.Now(), debug: debug.New(d.DebugDir), toggle: d.Toggle}, nil
}

// activePlugins — plugin còn bật, giữ thứ tự dựng để chèn tất định.
func (p *Proxy) activePlugins() []plugin.Plugin {
	return p.toggle.Active(p.d.Plugins)
}
