// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package control

import (
	"context"

	"won/proxy/core/plugin"
	"won/proxy/core/request"
	"won/proxy/core/session"
)

// markerPlugin chèn một marker mang tên mình — đủ căn cước để qua Apply.
type markerPlugin struct{ name string }

func (m markerPlugin) Name() string { return m.name }
func (m markerPlugin) Contribute(context.Context, *request.Snapshot, *session.Session) ([]*plugin.Contribution, error) {
	return plugin.One(&plugin.Contribution{Kind: plugin.KindMarker, Text: "🛣️ " + m.name + ": present"}), nil
}
