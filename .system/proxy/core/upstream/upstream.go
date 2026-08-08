// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

// Package upstream giữ chuỗi resolve đích và cache reverse proxy theo host.
// Không biết tên header nào của giao thức W.O.N — người gọi đọc header, đưa
// giá trị vào; gói này chỉ biết URL và vành đai suy ra từ lời khai (#6).
package upstream

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"

	"won/proxy/core/usage"
)

// Upstreams — đích nền cùng cache reverse proxy. def nil chỉ xảy ra khi người dựng lõi
// tự không đưa đích nào; lúc ấy dòng chính bị từ chối rõ thay vì đoán một người nhận (#6).
type Upstreams struct {
	def *url.URL

	// stripHeaders — header nội bộ của W.O.N, gỡ khỏi request trước khi rời hệ.
	// Người gọi khai (proxy protocol sống ở lớp gọi, không phải ở đây).
	stripHeaders []string

	mu       sync.Mutex // bảo vệ override và cache
	cache    map[string]*httputil.ReverseProxy
	override *url.URL // Control API, thắng def, thua header
}

func New(def string, stripHeaders []string) (*Upstreams, error) {
	u := &Upstreams{cache: map[string]*httputil.ReverseProxy{}, stripHeaders: stripHeaders}
	if def != "" {
		t, err := url.Parse(def)
		if err != nil || t.Scheme == "" || t.Host == "" {
			return nil, fmt.Errorf("upstream in config not valid: %q", def)
		}
		u.def = t
	}
	return u, nil
}

// Resolve theo chuỗi: headerOverride (đã đọc từ header bởi người gọi, rỗng nếu
// request không mang) → override runtime (Control API) → def (config) → nil.
// headerOverride ngoài vành đai bị từ chối rõ — proxy forward kèm header xác
// thực của client, nên đích tuỳ ý là open-proxy (#6).
func (u *Upstreams) Resolve(headerOverride string) (*url.URL, error) {
	if headerOverride != "" {
		t, err := url.Parse(headerOverride)
		if err != nil || t.Scheme == "" || t.Host == "" {
			return nil, fmt.Errorf("upstream header not valid: %q", headerOverride)
		}
		if !u.allowed(t.Host) {
			return nil, fmt.Errorf("upstream header points elsewhere than the declared target: %q — reachable hosts are the declared upstream and this machine", t.Host)
		}
		return t, nil
	}
	u.mu.Lock()
	o := u.override
	u.mu.Unlock()
	if o != nil {
		return o, nil
	}
	return u.def, nil
}

// SetOverride đặt đích runtime — cùng vành đai với header. Control bị chiếm
// cũng không thành open-proxy (#6).
func (u *Upstreams) SetOverride(raw string) (string, error) {
	t, err := url.Parse(raw)
	if err != nil || t.Scheme == "" || t.Host == "" {
		return "", fmt.Errorf("upstream not valid: %q", raw)
	}
	if !u.allowed(t.Host) {
		return "", fmt.Errorf("host outside the perimeter: %q — reachable hosts are the declared upstream and this machine", t.Host)
	}
	u.mu.Lock()
	u.override = t
	u.mu.Unlock()
	return t.String(), nil
}

func (u *Upstreams) ClearOverride() {
	u.mu.Lock()
	u.override = nil
	u.mu.Unlock()
}

// View — ảnh chụp cho GET /status.
func (u *Upstreams) View() (def, override string) {
	if u.def != nil {
		def = u.def.String()
	}
	u.mu.Lock()
	if u.override != nil {
		override = u.override.String()
	}
	u.mu.Unlock()
	return
}

// allowed — vành đai KHÔNG phải một danh sách người khai; nó suy ra từ chính lời khai:
// đúng host đã khai, cộng chính máy này. Cả hai đều là chỗ người dùng đã chọn, còn mọi
// host khác là một người nhận chưa ai đồng ý (#6). Không danh sách thì cũng không còn cái
// bẫy so-chuỗi của một danh sách — `localhost` và `127.0.0.1` không phải hai lời khai lệch
// nhau nữa.
func (u *Upstreams) allowed(host string) bool {
	if loopback(host) {
		return true
	}
	return u.def != nil && strings.EqualFold(host, u.def.Host)
}

// ProxyFor trả reverse proxy cho một đích, dựng một lần theo scheme+host.
// FlushInterval âm đẩy từng chunk SSE ngay — response streaming đi nguyên.
func (u *Upstreams) ProxyFor(target *url.URL) *httputil.ReverseProxy {
	key := target.Scheme + "://" + target.Host
	u.mu.Lock()
	defer u.mu.Unlock()
	if rp, ok := u.cache[key]; ok {
		return rp
	}
	t := *target // bản riêng cho closure — target của người gọi không bị giữ
	strip := u.stripHeaders
	rp := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			// SetURL NỐI path của đích với path của request, không thay: đích khai
			// `…/anthropic` + lượt `/v1/messages` → `…/anthropic/v1/messages`.
			pr.SetURL(&t)
			pr.Out.Host = t.Host
			for _, h := range strip {
				pr.Out.Header.Del(h)
			}
		},
		FlushInterval:  -1,
		ModifyResponse: usage.Tap,
		ErrorHandler:   unreachable(t),
		ErrorLog:       slog.NewLogLogger(slog.Default().Handler(), slog.LevelError),
	}
	u.cache[key] = rp
	return rp
}

// unreachable — không ai trả lời ở đích thì nói thành lời: đích nào, lỗi gì, làm gì tiếp.
// Mặc định của ReverseProxy là một 502 rỗng ruột, và khi đích mặc định là chính máy người
// dùng thì đây là lỗi đầu tiên người mới gặp — "chưa ai mở cửa ở đó" đọc lên y như "proxy
// hỏng", nên nó phải nói ra mình là cái nào.
func unreachable(target url.URL) func(http.ResponseWriter, *http.Request, error) {
	return func(w http.ResponseWriter, r *http.Request, err error) {
		// Client tự ngắt giữa dòng là chuyện thường của streaming, không phải lỗi của đích.
		if errors.Is(err, context.Canceled) {
			return
		}
		hint := "check `upstream` in won.conf, and whether that host is reachable"
		if loopback(target.Host) {
			hint = "start the model server on this machine, or declare another `upstream` in won.conf"
		}
		// `hint` đã tính ở trên cho thân 502; log phải mang luôn nó. Người đọc console và
		// người đọc thân lỗi là cùng một người, và chỉ một trong hai chỗ có cách sửa thì
		// chỗ kia là một lời kêu không có lối ra.
		slog.Warn("upstream: unreachable", "target", target.Host, "err", err, "fix", hint)
		http.Error(w, fmt.Sprintf("Proxy Inject: no answer from upstream %s (%v) — %s", target.Host, err, hint), http.StatusBadGateway)
	}
}

// loopback — đích có nằm trên chính máy này không. Quyết lời khuyên nào đi kèm 502, nên
// nó đọc host chứ không đọc lời khai: `localhost` và `127.0.0.1` là cùng một chỗ.
func loopback(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
