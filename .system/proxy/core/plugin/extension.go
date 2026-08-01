// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package plugin

import "net/http"

// ControlExtension — plugin mở rộng Control API bằng route riêng dưới
// /plugins/{name}/. Cùng pattern với SystemOwner/TurnVoice: plugin tự khai
// qua interface, lõi không biết plugin nào có route.
type ControlExtension interface {
	ControlHandler() http.Handler
}

// AgentCtxKey — khoá context cho agent đã resolve, gắn bởi control middleware.
// Cùng pattern với owner.go: type rỗng làm khoá, không xung đột string.
type AgentCtxKey struct{}
