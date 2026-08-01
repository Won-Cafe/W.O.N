// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package proxy

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
)

// Hai đường chuyển tiếp KHÔNG chạm body, gom một nhà vì cùng một bất biến: byte ra khỏi
// hệ đúng như byte vào (#1). Khác nhau ở chỗ vào — một đường chưa đọc thân, một đường đã
// đọc rồi phải đặt lại.

// servePassthrough — chưa đọc thân, chuyển tiếp thẳng. Dùng cho GET, count_tokens,
// models, path lạ. Đường thường nên chỉ nói ở mức debug.
func (p *Proxy) servePassthrough(w http.ResponseWriter, r *http.Request, rp *httputil.ReverseProxy, target *url.URL) {
	slog.Debug("proxy: passthrough", "method", r.Method, "path", r.URL.Path, "target", target.Host)
	rp.ServeHTTP(w, r)
}

// forwardRaw — đã đọc thân nên phải đặt lại nguyên bản. Ba đường tới: vắng đệ, parse
// hỏng, marshal hỏng (#2). reason rỗng = đường thường, không kêu lên.
func (p *Proxy) forwardRaw(w http.ResponseWriter, r *http.Request, rp *httputil.ReverseProxy, raw []byte, reason string, cause error) {
	// msg là hằng, lý do đi vào attr: msg dựng từ biến thì không grep và không gom nhóm được.
	if reason != "" {
		slog.Warn("proxy: forwarding raw", "reason", reason, "err", cause)
	}
	r.Body = io.NopCloser(bytes.NewReader(raw))
	r.ContentLength = int64(len(raw))
	r.Header.Del("Content-Length")
	rp.ServeHTTP(w, r)
}
