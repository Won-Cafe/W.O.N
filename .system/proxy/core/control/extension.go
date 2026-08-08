// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package control

import (
	"net/http"

	"won/proxy/core/plugin"
)

// mountExtensions gắn route riêng của plugin dưới /plugins/{name}/.
// StripPrefix để plugin handler nhận path đã cắt — đăng ký `PUT /update` match.
func (a *API) mountExtensions(mux *http.ServeMux) {
	for _, p := range a.d.Plugins {
		if ext, ok := p.(plugin.ControlExtension); ok {
			mux.Handle("/plugins/"+p.Name()+"/", http.StripPrefix("/plugins/"+p.Name(), ext.ControlHandler()))
		}
	}
}
