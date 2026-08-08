// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package control

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"won/proxy/core/config"
	"won/proxy/core/plugin"
	"won/proxy/core/request"
	"won/proxy/core/session"
	"won/proxy/core/upstream"
)

type fakeID struct{}

func (fakeID) Resolve(string, string) string { return "Tzu" }

func newTestAPI(t *testing.T, upstreamURL string) *API {
	t.Helper()
	store := session.NewStore(filepath.Join(t.TempDir(), "state.json"))
	plugins := []plugin.Plugin{markerPlugin{name: "horn"}}
	up, err := upstream.New(upstreamURL, request.InternalHeaders())
	if err != nil {
		t.Fatal(err)
	}
	return New(Deps{
		Identity: fakeID{},
		Plugins:  plugins,
		Store:    store,
		Up:       up,
		Toggle:   plugin.NewToggle(plugins),
		Start:    time.Now(),
	})
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

func TestStatusShape(t *testing.T) {
	h := newTestAPI(t, "https://api.anthropic.com").Handler()
	w := call(t, h, "GET", "/status", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status must be 200, got %d: %s", w.Code, w.Body)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type must be application/json, got %q", ct)
	}
	var v statusView
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatalf("status must be valid JSON: %v", err)
	}
	if v.Upstream.Default != "https://api.anthropic.com" {
		t.Fatalf("default must be config: %+v", v.Upstream)
	}
	if len(v.Plugins) != 1 || v.Plugins[0].Name != "horn" || !v.Plugins[0].Enabled {
		t.Fatalf("plugin built at startup must be enabled: %+v", v.Plugins)
	}
}

// `X-WON-Control` là lời buồng lái TỰ KHAI, và nó đi thẳng vào một dòng nhật ký. Chữ ấy là
// của người ngoài: phải có trần và phải rửa — một dấu xuống dòng trong đó là một dòng nhật ký
// GIẢ do client viết. Vắng lời khai thì nói "gọi thẳng", không để trống: một trường vắng đọc
// như nhật ký quên ghi.
func TestControlFromIsWashedBeforeItReachesTheLog(t *testing.T) {
	bare := httptest.NewRequest("GET", "/", nil)
	if got := controlFrom(bare); got != fromDirect {
		t.Errorf("vắng lời khai phải nói là gọi thẳng, got %q", got)
	}

	clean := httptest.NewRequest("GET", "/", nil)
	clean.Header.Set(request.HeaderControl, "cockpit/upstream")
	if got := controlFrom(clean); got != "cockpit/upstream" {
		t.Errorf("lời khai sạch phải đi nguyên, got %q", got)
	}

	dirty := httptest.NewRequest("GET", "/", nil)
	dirty.Header.Set(request.HeaderControl, "evil\ncontrol: dòng giả "+strings.Repeat("x", 200))
	got := controlFrom(dirty)
	if strings.ContainsAny(got, " \t\r\n") {
		t.Errorf("khoảng trắng và xuống dòng phải bị rửa: %q", got)
	}
	if len(got) > fromMax {
		t.Errorf("lời khai phải có trần %d byte, got %d: %q", fromMax, len(got), got)
	}
}

// Control không có auth và vành đai duy nhất là loopback, nên dòng nhật ký của một lần VẶN
// NÚM phải nói được lệnh đến từ đâu. Đọc dial thì không cần — cái đổi trạng thái mới cần vết.
func TestKnobTurnLogsWhoTurnedIt(t *testing.T) {
	var buf strings.Builder
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(old)

	h := newTestAPI(t, "https://api.anthropic.com").Handler()

	declared := httptest.NewRequest("PUT", "/upstream", strings.NewReader(`{"url":"http://127.0.0.1:11434"}`))
	declared.Header.Set(request.HeaderControl, "cockpit/upstream")
	h.ServeHTTP(httptest.NewRecorder(), declared)
	if logged := buf.String(); !strings.Contains(logged, "from=cockpit/upstream") {
		t.Errorf("nhật ký không nói ai vặn núm: %s", logged)
	}

	buf.Reset()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("DELETE", "/upstream", nil))
	if logged := buf.String(); !strings.Contains(logged, "from="+fromDirect) {
		t.Errorf("không khai thì phải ghi là gọi thẳng: %s", logged)
	}
}

// Vòng đời override: đích trong vành đai → thấy trong status và resolve; host lạ bị từ
// chối; header vẫn thắng override; DELETE quay về config. Vành đai không còn là danh sách
// người khai — nó là đúng đích đã khai cộng chính máy này, nên chỗ duy nhất override đi
// tới được mà khác config là máy của người dùng.
func TestUpstreamOverrideLifecycle(t *testing.T) {
	a := newTestAPI(t, "https://api.anthropic.com")
	h := a.Handler()

	if w := call(t, h, "PUT", "/upstream", `{"url":"https://evil.example"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("host outside the perimeter must be 400, got %d: %s", w.Code, w.Body)
	}
	if w := call(t, h, "PUT", "/upstream", `{"url":"http://127.0.0.1:11434"}`); w.Code != http.StatusOK {
		t.Fatalf("this machine must be 200, got %d: %s", w.Code, w.Body)
	}

	if target, err := a.d.Up.Resolve(""); err != nil || target == nil || target.Host != "127.0.0.1:11434" {
		t.Fatalf("override must beat config: %v %v", target, err)
	}
	if target, err := a.d.Up.Resolve("https://api.anthropic.com"); err != nil || target.Host != "api.anthropic.com" {
		t.Fatalf("header must beat override: %v %v", target, err)
	}

	if w := call(t, h, "DELETE", "/upstream", ""); w.Code != http.StatusOK {
		t.Fatalf("delete override must be 200, got %d", w.Code)
	}
	if target, err := a.d.Up.Resolve(""); err != nil || target.Host != "api.anthropic.com" {
		t.Fatalf("deleting override must revert to config: %v %v", target, err)
	}
}

// Toggle qua API phải chạm toggle thật: tắt qua PUT /plugins/{name} → States thấy false.
func TestPluginToggleViaAPI(t *testing.T) {
	a := newTestAPI(t, "https://api.anthropic.com")
	h := a.Handler()

	if w := call(t, h, "PUT", "/plugins/horn", `{"enabled":false}`); w.Code != http.StatusOK {
		t.Fatalf("toggling built plugin must be 200, got %d: %s", w.Code, w.Body)
	}
	states := a.d.Toggle.States(a.d.Plugins)
	if len(states) != 1 || states[0].Name != "horn" || states[0].Enabled {
		t.Fatalf("disabled plugin must show false in States: %+v", states)
	}

	if w := call(t, h, "PUT", "/plugins/horn", `{"enabled":true}`); w.Code != http.StatusOK {
		t.Fatalf("re-enable must be 200, got %d", w.Code)
	}
	states = a.d.Toggle.States(a.d.Plugins)
	if len(states) != 1 || !states[0].Enabled {
		t.Fatalf("re-enabled plugin must show true in States: %+v", states)
	}
}

func TestControlErrors(t *testing.T) {
	h := newTestAPI(t, "https://api.anthropic.com").Handler()
	cases := []struct {
		name         string
		method, path string
		body         string
		want         int
	}{
		{"unknown plugin", "PUT", "/plugins/ma", `{"enabled":false}`, http.StatusNotFound},
		{"bad body", "PUT", "/plugins/horn", `not-json`, http.StatusBadRequest},
		{"body missing enabled", "PUT", "/plugins/horn", `{}`, http.StatusBadRequest},
		{"empty url", "PUT", "/upstream", `{"url":""}`, http.StatusBadRequest},
		{"wrong method", "POST", "/status", "", http.StatusMethodNotAllowed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if w := call(t, h, c.method, c.path, c.body); w.Code != c.want {
				t.Fatalf("want %d, got %d: %s", c.want, w.Code, w.Body)
			}
		})
	}
}

// Control API dựng với 0 plugin vẫn đứng — status trả mảng rỗng.
func TestControlZeroPlugins(t *testing.T) {
	store := session.NewStore(filepath.Join(t.TempDir(), "state.json"))
	up, err := upstream.New("https://api.anthropic.com", request.InternalHeaders())
	if err != nil {
		t.Fatal(err)
	}
	a := New(Deps{
		Identity: fakeID{},
		Plugins:  nil,
		Store:    store,
		Up:       up,
		Toggle:   plugin.NewToggle(nil),
		Start:    time.Now(),
	})
	h := a.Handler()
	w := call(t, h, "GET", "/status", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status must be 200 even with 0 plugins, got %d: %s", w.Code, w.Body)
	}
	var v statusView
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatalf("status valid JSON: %v", err)
	}
	if len(v.Plugins) != 0 {
		t.Fatalf("0 plugins → status Plugins empty, got %v", v.Plugins)
	}
}

// Buồng lái phải hiện MỌI khoá lõi hiểu, và không hiện khoá nào lõi không hiểu. Trước đây
// ba chỗ tự liệt kê tay tập khoá won.conf, nên `listen` và `control` vắng khỏi bảng mà
// không ai biết là cố ý hay quên — mọi lệch ở đây đều im lặng.
//
// `think` là ngoại lệ DUY NHẤT và có lý do: nil = chưa khai, và chưa khai thì không gửi
// field đi (xem boolOrNil). Ngoại lệ nào khác xuất hiện thì test này đỏ.
func TestConfigShowsEveryCoreKey(t *testing.T) {
	a := newTestAPI(t, "https://api.anthropic.com")
	a.d.Config = &config.Config{Listen: ":8787", Control: ":7777"}

	w := call(t, a.Handler(), "GET", "/config", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /config = %d: %s", w.Code, w.Body)
	}
	var v configView
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatalf("config valid JSON: %v", err)
	}

	for _, k := range config.CoreKeys {
		if _, ok := v.Values[k]; !ok {
			t.Errorf("khoá %q lõi hiểu mà buồng lái không hiện", k)
		}
	}
	known := map[string]bool{}
	for _, k := range config.CoreKeys {
		known[k] = true
	}
	for k := range v.Values {
		if !known[k] {
			t.Errorf("buồng lái hiện %q — khoá này không có trong config.CoreKeys", k)
		}
	}
	// Khoá bootstrap vẫn hiện GIÁ TRỊ, và `locked` nói vì sao không sửa được.
	if v.Values["listen"] != ":8787" {
		t.Errorf("listen phải hiện giá trị đang bind, got %v", v.Values["listen"])
	}
	if _, locked := v.Locked["listen"]; !locked {
		t.Error("listen phải nằm trong locked kèm lý do")
	}
}
