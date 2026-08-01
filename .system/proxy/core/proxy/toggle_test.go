// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package proxy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"won/proxy/core/plugin"
	"won/proxy/core/request"
	"won/proxy/core/session"
	"won/proxy/core/upstream"
)

type fakeID struct{}

func (fakeID) Resolve(string, string) string { return "Tzu" }

// markerPlugin chèn một marker mang tên mình — đủ căn cước để qua Apply.
type markerPlugin struct{ name string }

func (m markerPlugin) Name() string { return m.name }
func (m markerPlugin) Contribute(context.Context, *request.Snapshot, *session.Session) ([]*plugin.Contribution, error) {
	return plugin.One(&plugin.Contribution{Kind: plugin.KindMarker, Text: "🛣️ " + m.name + ": present"}), nil
}

func newTestProxy(t *testing.T, upstreamURL string) *Proxy {
	t.Helper()
	store := session.NewStore(filepath.Join(t.TempDir(), "state.json"))
	plugins := []plugin.Plugin{markerPlugin{name: "horn"}}
	up, err := upstream.New(upstreamURL, request.InternalHeaders())
	if err != nil {
		t.Fatal(err)
	}
	p, err := New(Deps{
		Identity:    fakeID{},
		Store:       store,
		Plugins:     plugins,
		TotalBudget: time.Second,
		Up:          up,
		Toggle:      plugin.NewToggle(plugins),
	})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func call(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// Toggle phải chạm dòng chính thật: plugin tắt qua Toggle.Set thì marker không tới upstream.
func TestPluginToggleDataPlane(t *testing.T) {
	var got string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = string(b)
		w.Write([]byte(`{}`))
	}))
	defer ts.Close()

	p := newTestProxy(t, ts.URL)
	body := `{"model":"m","system":"s","messages":[{"role":"user","content":"hi"}]}`
	send := func() {
		r := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(body))
		p.ServeHTTP(httptest.NewRecorder(), r)
	}

	send()
	if !strings.Contains(got, "horn") {
		t.Fatalf("enabled plugin must insert marker: %s", got)
	}

	if err := p.toggle.Set("horn", false); err != nil {
		t.Fatalf("toggle off must succeed: %v", err)
	}
	send()
	if strings.Contains(got, "horn") {
		t.Fatalf("disabled plugin must not be inserted: %s", got)
	}

	if err := p.toggle.Set("horn", true); err != nil {
		t.Fatalf("toggle on must succeed: %v", err)
	}
	send()
	if !strings.Contains(got, "horn") {
		t.Fatalf("re-enabled plugin must insert marker: %s", got)
	}
}

// Proxy dựng với 0 plugin vẫn đứng — không panic, không Fatalf: lõi chạy như
// reverse proxy trong suốt. Tầng nền chết thì dòng chính vẫn chảy (#2), và
// "không plugin nào" là ca cực đoan của chính câu đó.
func TestProxyZeroPlugins(t *testing.T) {
	store := session.NewStore(filepath.Join(t.TempDir(), "state.json"))
	up, err := upstream.New("https://api.anthropic.com", request.InternalHeaders())
	if err != nil {
		t.Fatal(err)
	}
	p, err := New(Deps{
		Identity:    fakeID{},
		Store:       store,
		Plugins:     nil, // 0 plugin — wiring rỗng hợp lệ
		TotalBudget: time.Second,
		Up:          up,
		Toggle:      plugin.NewToggle(nil),
	})
	if err != nil {
		t.Fatalf("0 plugins must build without error: %v", err)
	}
	if got := p.activePlugins(); len(got) != 0 {
		t.Fatalf("0 plugins → activePlugins empty, got %d", len(got))
	}
}
