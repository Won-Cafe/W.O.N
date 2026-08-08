// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

// Package control là buồng lái local: xem trạng thái, đổi upstream, bật/tắt
// plugin. Vành đai: loopback bind + vành đai đích. Control không cầm bí mật nào (#5).
package control

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"won/proxy/core/config"
	"won/proxy/core/plugin"
	"won/proxy/core/request"
	"won/proxy/core/session"
	"won/proxy/core/soul"
	"won/proxy/core/upstream"
)

// Deps là mọi thứ Control API cần, dựng sẵn từ main.
type Deps struct {
	Identity soul.IdentityResolver
	Plugins  []plugin.Plugin
	Store    *session.Store
	Up       *upstream.Upstreams
	Config   *config.Config
	Cockpit  []byte
	Toggle   *plugin.Toggle
	Start    time.Time

	// Ba thứ dưới chỉ `POST /trigger/{name}` dùng, và nó dùng đúng bản dòng chính đang
	// chạy: một lượt chạy khô đo bằng luật khác dòng chính thì nó đo một hệ khác.
	FrameRules   request.FrameRules
	DefaultAgent string
	TotalBudget  time.Duration
}

type API struct{ d Deps }

func New(d Deps) *API { return &API{d: d} }

func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", a.handleCockpit)
	mux.HandleFunc("GET /status", a.handleStatus)
	mux.HandleFunc("PUT /upstream", a.handlePutUpstream)
	mux.HandleFunc("DELETE /upstream", a.handleDeleteUpstream)
	mux.HandleFunc("PUT /plugins/{name}", a.handlePutPlugin)
	mux.HandleFunc("GET /config", a.handleGetConfig)
	mux.HandleFunc("PUT /config", a.handlePutConfig)
	// `/trigger/{name}`, KHÔNG phải `/plugins/{name}/trigger`: mẫu thứ hai đụng với
	// `/plugins/{tên}/` mà mountExtensions đăng ký, và hai mẫu đụng nhau thì mux panic.
	mux.HandleFunc("POST /trigger/{name}", a.handleTrigger)
	a.mountExtensions(mux)
	return withAgent(a.d.Identity, mux)
}

// Available — mọi plugin biên dịch được vào binary, kể cả cái won.conf chưa bật. Khác
// `Plugins`, chỉ là những cái đã DỰNG. Trang điều khiển dựng ô chọn từ đây nên nó không
// giữ bản sao nào của danh sách plugin.
type statusView struct {
	UptimeS   int64          `json:"uptime_s"`
	Upstream  upstreamView   `json:"upstream"`
	Plugins   []plugin.State `json:"plugins"`
	Available []string       `json:"available"`
	Sessions  []session.Info `json:"sessions"`
}

type upstreamView struct {
	Default  string `json:"default"`
	Override string `json:"override"`
}

// configView — núm đọc được, kèm danh sách khoá KHÔNG ghi được và lý do. Trang điều
// khiển dựng ô nhập từ đây, nên nó không phải chép lại danh sách nào.
type configView struct {
	Path   string            `json:"path"`
	Values map[string]any    `json:"values"`
	Locked map[string]string `json:"locked"`
	Note   string            `json:"note"`
}

const noteRestart = "won.conf là cấu hình lúc khởi động — sửa xong phải chạy lại proxy mới có hiệu lực, trừ upstream và bật/tắt plugin (vặn nóng được ở route riêng)."

func (a *API) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	c := a.d.Config
	if c == nil {
		a.writeError(w, http.StatusNotFound, "this proxy was built without config — nothing to read")
		return
	}
	lm := c.Services.LocalModel
	// Mọi khoá trong config.CoreKeys phải có mặt ở đây, kể cả khoá KHÔNG ghi được (`listen`,
	// `control`): buồng lái nói `locked` kèm lý do, chứ không im lặng bỏ khoá ra khỏi bảng —
	// một khoá vắng đọc ra như "lõi không biết khoá này". Test khoá: TestConfigShowsEveryCoreKey.
	// Khoá chưa khai cũng phải hiện, dưới dạng `null` (`strip_sections`) hoặc giá trị mặc định
	// đang chạy: thêm khoá chỉ khi đã khai thì nó vắng mặt ở đúng cấu hình mặc định, và trang
	// điều khiển không dựng nổi ô cho nó.
	vals := map[string]any{
		"listen": c.Listen, "control": c.Control,
		"upstream": c.Upstream, "log_level": c.LogLevel, "default_agent": c.DefaultAgent,
		// *bool: chưa khai → `null`, tức trạng thái "theo log_level" (§ Cấu hình).
		"debug_log":       c.DebugLog,
		"total_budget_ms": c.TotalBudgetMs,
		"model":           lm.Model, "base_url": lm.BaseURL, "timeout_ms": lm.TimeoutMs,
		"max_tokens": lm.MaxTokens, "keep_alive": lm.KeepAlive, "think": lm.Think,
		"temperature": lm.Temperature,
		"ground":      c.Ground, "strip_tags": c.StripTags, "unwrap_tags": c.UnwrapTags,
		"strip_sections": c.StripSections, "strip_identity": c.StripIdentity,
	}
	a.writeJSON(w, http.StatusOK, configView{
		Path: c.Paths.Conf(), Values: vals, Locked: config.Locked, Note: noteRestart,
	})
}

// handlePutConfig ghi núm vào won.conf. Ghi FILE, không đổi tiến trình đang chạy: đổi
// nóng một núm mà file vẫn giá trị cũ là hai sự thật, và lần chạy sau bản nào thắng thì
// không ai biết. Khoá bị chặn trả về trong `refused`, không nuốt im lặng.
func (a *API) handlePutConfig(w http.ResponseWriter, r *http.Request) {
	c := a.d.Config
	if c == nil {
		a.writeError(w, http.StatusNotFound, "this proxy was built without config — nothing to write")
		return
	}
	var body map[string]string
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body) == 0 {
		a.writeError(w, http.StatusBadRequest, `body must be JSON {"key": "value", ...}`)
		return
	}
	res, err := config.UpdateFile(c.Paths.Conf(), body)
	if err != nil {
		a.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	slog.Info("control: config written", "path", res.Path,
		"changed", len(res.Changed), "refused", len(res.Refused), "from", controlFrom(r))
	a.writeJSON(w, http.StatusOK, map[string]any{
		"path": res.Path, "changed": res.Changed, "added": res.Added,
		"refused": res.Refused, "note": noteRestart,
	})
}

// handleCockpit trả trang điều khiển — main đọc sẵn từ services/cockpit, core chỉ giữ
// byte (§ Phân lớp & thư mục: core không import services). Cùng gốc với các route dưới
// nên không cần CORS. Rỗng (chưa wiring) → 404, không phải trang trắng.
//
// Mẫu đăng ký là `GET /{$}` — ĐÚNG path gốc. `GET /` khớp mọi path, và một mẫu như thế
// đụng với route riêng của plugin (`/plugins/{tên}/`): mux từ chối cả hai bằng panic,
// nên chỉ cần một plugin có route riêng cộng buồng lái bật là tiến trình chết.
func (a *API) handleCockpit(w http.ResponseWriter, r *http.Request) {
	if len(a.d.Cockpit) == 0 {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(a.d.Cockpit)
}

func (a *API) handleStatus(w http.ResponseWriter, r *http.Request) {
	def, override := a.d.Up.View()
	a.writeJSON(w, http.StatusOK, statusView{
		UptimeS:   int64(time.Since(a.d.Start).Seconds()),
		Upstream:  upstreamView{Default: def, Override: override},
		Plugins:   a.d.Toggle.States(a.d.Plugins),
		Available: plugin.Registered(),
		Sessions:  a.d.Store.Infos(time.Now()),
	})
}

func (a *API) handlePutUpstream(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		a.writeError(w, http.StatusBadRequest, fmt.Sprintf(`body must be JSON {"url": "https://..."}: %v`, err))
		return
	}
	if body.URL == "" {
		a.writeError(w, http.StatusBadRequest, "url empty — to clear override use DELETE /upstream")
		return
	}
	set, err := a.d.Up.SetOverride(body.URL)
	if err != nil {
		a.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	slog.Info("control: upstream override", "url", set, "from", controlFrom(r))
	a.writeJSON(w, http.StatusOK, map[string]string{"override": set})
}

func (a *API) handleDeleteUpstream(w http.ResponseWriter, r *http.Request) {
	a.d.Up.ClearOverride()
	slog.Info("control: upstream override cleared", "from", controlFrom(r))
	a.writeJSON(w, http.StatusOK, map[string]string{"override": ""})
}

func (a *API) handlePutPlugin(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var body struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Enabled == nil {
		a.writeError(w, http.StatusBadRequest, `body must be JSON {"enabled": true} or {"enabled": false}`)
		return
	}
	if err := a.d.Toggle.Set(name, *body.Enabled); err != nil {
		a.writeError(w, http.StatusNotFound, err.Error())
		return
	}
	slog.Info("control: plugin toggle", "plugin", name, "enabled", *body.Enabled,
		"from", controlFrom(r), "note", cacheNote)
	a.writeJSON(w, http.StatusOK, plugin.State{Name: name, Enabled: *body.Enabled, Note: cacheNote})
}

// cacheNote — bật/tắt một plugin giữa phiên đổi các khối trong lời hệ thống, tức
// đổi tiền tố cache của nhà cung cấp. Lượt kế phải trả lại phí ghi cho cả hội thoại
// đúng một lần, rồi rẻ trở lại. Nói ra ở cả response và log: người vặn núm phải
// thấy cái giá, không phải đoán nó.
const cacheNote = "plugins contributing to the system prompt change the cached prefix — the next turn pays one cache write for the whole conversation"

// writeJSON — encode hỏng sau khi headers đã đi thì không cứu được, nhưng có log.
func (a *API) writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("control: encode response error", "err", err)
	}
}

func (a *API) writeError(w http.ResponseWriter, code int, msg string) {
	a.writeJSON(w, code, map[string]string{"error": msg})
}
