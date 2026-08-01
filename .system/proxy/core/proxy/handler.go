// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package proxy

import (
	"net/http"

	"won/proxy/core/request"
)

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	target, err := p.up.Resolve(r.Header.Get(request.HeaderUpstream))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if target == nil {
		http.Error(w, msgNoUpstream, http.StatusBadGateway)
		return
	}
	rp := p.up.ProxyFor(target)

	// Format do router quyết từ path — không sniff body (#6).
	format := request.FormatFromPath(r.URL.Path)
	if r.Method != http.MethodPost || format == request.FormatUnknown {
		p.servePassthrough(w, r, rp, target)
		return
	}
	p.serveMainline(w, r, rp, target, format)
}
