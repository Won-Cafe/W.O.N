// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

// Composition root của Proxy Inject: đọc config → dựng services → dựng
// plugins đã khai báo → giao cho lõi. Chỉ wiring, không logic.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"won/proxy/core/config"
	"won/proxy/core/control"
	"won/proxy/core/ground"
	"won/proxy/core/plugin"
	"won/proxy/core/proxy"
	"won/proxy/core/request"
	"won/proxy/core/session"
	"won/proxy/core/soul"
	"won/proxy/core/upstream"
	"won/proxy/services/cockpit"
	"won/proxy/services/localmodel"
	"won/proxy/services/logging"
)

// Blank-import mọi plugin nằm ở plugins_gen.go — sinh tự động bằng
// go generate. Thêm/bớt plugin thì chạy lại generate, không sửa tay file này.
//go:generate go run ./tools/genplugins

// defaultRoster — plugin bật khi không có won.conf. THỨ TỰ CÓ NGHĨA: nó là thứ tự các
// khối trong lời hệ thống, đọc từ trên xuống. won.conf có thì thứ tự của người khai
// thắng (xem config.EnabledPlugins).
//
// Khớp đúng các dòng `<tên>.enable = true` trong won.conf.example, kể cả thứ tự: xoá
// won.conf đi thì hệ chèn y như cũ. Ba plugin cuối cần `model`, và `model` cũng đã có
// mặc định — nên "không có won.conf" là một cấu hình chạy đủ, không phải cấu hình rút gọn.
var defaultRoster = []string{"identity", "memory", "loiterer", "outfitter", "wayfarer"}

func main() {
	root := flag.String("root", "", "W.O.N root (empty = auto-detect dir containing .system)")
	listen := flag.String("listen", "", "data-plane address (empty = "+config.DefaultListen+")")
	controlFlag := flag.String("control", "", "Control API address (empty = disabled; 'off' = disabled)")
	upstreamFlag := flag.String("upstream", "", "model target (empty = this machine, "+config.DefaultTarget+")")
	conf := flag.String("conf", "", "won.conf path (empty = <root>/.system/proxy/won.conf if present)")
	logLevel := flag.String("log", "", "log level: debug|info|silent (empty = won.conf, then info)")
	flag.Parse()

	// Mức log dựng hai nhịp: nhịp đầu để việc đọc config có tiếng, nhịp sau theo
	// won.conf sau khi đã đọc được. `proxy.MsgRunStart` nối ở đây vì chỉ main biết cả tên
	// bản ghi của lõi lẫn cách console tách khối.
	level := logging.Normalize(*logLevel)
	console := logging.Open(level, proxy.MsgRunStart)

	cfg, err := config.Load(config.Options{
		Root:     *root,
		Listen:   *listen,
		Control:  *controlFlag,
		Upstream: *upstreamFlag,
		ConfPath: *conf,
		Roster:   defaultRoster,
	})
	if err != nil {
		fatal("config: %v", err)
	}
	if *logLevel == "" {
		level = logging.Normalize(cfg.LogLevel) // cờ thắng file, đúng chuỗi ưu tiên của config
		console = logging.Open(level, proxy.MsgRunStart)
	}

	// Sổ soul hỏng không chặn dòng chính — chạy tiếp không căn cước (#2).
	book, err := soul.Load(cfg.Paths.Agents())
	if err != nil {
		slog.Warn("soul: not loaded — running without identity", "dir", cfg.Paths.Agents(), "err", err)
		book = soul.Empty()
	}

	// Đệ mặc định phải CÓ THẬT trong sổ soul. Khai một cái tên không có thì lõi sẽ nhận
	// ra một đệ rỗng và chèn một khối trống — thà tắt và nói ra (#6).
	defaultAgent := book.Resolve(cfg.DefaultAgent, "")
	if cfg.DefaultAgent != "" && defaultAgent == "" {
		slog.Warn("soul: default_agent not found — falling back to passthrough",
			"asked", cfg.DefaultAgent, "have", strings.Join(book.Names(), ","))
	}

	// Đất đến từ file người giữ hệ khai (`ground` trong won.conf) — main đọc, lõi
	// chèn. Mẫu không khớp file nào thì kêu lên rồi chạy tiếp: dòng chính không đứng
	// vì một file thiếu (#2).
	groundFiles, groundMiss := ground.Load(cfg.Paths, cfg.Ground)
	if len(groundMiss) > 0 {
		slog.Warn("ground: patterns matched nothing", "patterns", strings.Join(groundMiss, ","))
	}
	if len(groundFiles) == 0 {
		slog.Warn("ground: no file matched — running without ground", "declared", strings.Join(cfg.Ground, ","))
	}

	lm := localmodel.New(localmodel.Config{
		BaseURL:     cfg.Services.LocalModel.BaseURL,
		Model:       cfg.Services.LocalModel.Model,
		TimeoutMs:   cfg.Services.LocalModel.TimeoutMs,
		MaxTokens:   cfg.Services.LocalModel.MaxTokens,
		KeepAlive:   cfg.Services.LocalModel.KeepAlive,
		Think:       cfg.Services.LocalModel.Think,
		Temperature: cfg.Services.LocalModel.Temperature,
	})
	hub := plugin.NewHub()
	hub.Set(localmodel.ServiceName, lm)
	hub.Set(soul.ServiceName, book)

	// Nạp model nền ở NỀN, một lần. Máy nguội mất tới hàng chục giây để nạp, mà cổng
	// phải mở ngay — chờ ở đây là đổi một lượt chậm lấy một khởi động chậm. `keep_alive`
	// lo phần giữ; cái này chỉ lo lượt ĐẦU. Hỏng thì kêu một dòng rồi thôi: lượt đầu
	// chậm đúng như trước, không ai mất gì (#2).
	if lm != nil {
		go func() {
			if err := lm.Warm(context.Background()); err != nil {
				slog.Warn("localmodel: warm-up failed — the first turn will wait for the model to load",
					"model", cfg.Services.LocalModel.Model, "err", err)
			}
		}()
	}

	names := cfg.EnabledPlugins()
	// Kêu chỉ khi có plugin THẬT cần model nền. Bản mới sạch chạy identity + memory,
	// không ai cần nó — một warn ở đó là lời sai với người vừa mở hệ lần đầu.
	if lm == nil {
		if need := needLocalModel(names); len(need) > 0 {
			slog.Warn("localmodel: not configured — these plugins will stay silent",
				"plugins", strings.Join(need, ","))
		}
	}
	var plugins []plugin.Plugin
	var failed []string
	for _, name := range names {
		p, err := plugin.Build(name, plugin.Env{
			Options:  cfg.Plugins[name].Options,
			Paths:    cfg.Paths,
			Services: hub,
			Control:  cfg.Control,
		})
		if err != nil {
			slog.Warn("plugin: build failed", "plugin", name, "err", err)
			failed = append(failed, name)
			continue
		}
		plugins = append(plugins, p)
	}
	if len(failed) > 0 {
		// Mọi plugin fail → chạy 0-plugin như reverse proxy trong suốt, không Fatalf.
		slog.Warn("plugin: skipped", "failed", strings.Join(failed, ","), "built", len(plugins))
	}

	toggle := plugin.NewToggle(plugins)
	up, err := upstream.New(cfg.Upstream, request.InternalHeaders())
	if err != nil {
		fatal("upstream: %v", err)
	}
	store := session.NewStore(cfg.Paths.State())

	// Một lời khai, hai chỗ dùng: dòng chính và buồng lái phải đo khung bằng ĐÚNG một bộ
	// luật. Dựng hai bản là hai bản lệch được khi thêm hạng luật thứ năm mà chỉ sửa một chỗ.
	frameRules := request.FrameRules{
		Strip:    cfg.StripTags,
		Unwrap:   cfg.UnwrapTags,
		Sections: cfg.StripSections,
		Identity: cfg.StripIdentity,
	}

	prx, err := proxy.New(proxy.Deps{
		Identity:     book,
		DefaultAgent: defaultAgent,
		Store:        store,
		Plugins:      plugins,
		TotalBudget:  cfg.TotalBudget(),
		DebugDir:     debugDir(cfg, level),
		Up:           up,
		Toggle:       toggle,
		FrameRules:   frameRules,
		Ground:       ground.Text(groundFiles),
		House:        book.House(),
		Workspace:    cfg.Paths.Root,
	})
	if err != nil {
		fatal("proxy: %v", err)
	}

	// Buồng lái không mở được thì KHÔNG chết theo: nó là mặt tuỳ chọn, dòng chính là mặt
	// chính (#2).
	cockpitAddr := ""
	if ln, err := listenControl(cfg.Control); err != nil {
		slog.Warn("control: not opened — running without the cockpit", "addr", cfg.Control, "err", err,
			"fix", "another process holds that port (often an older proxy still running); free it, pick another port, or comment out `control`")
	} else if ln != nil {
		cockpitAddr = cfg.Control
		ctrl := control.New(control.Deps{
			Identity:     book,
			Plugins:      plugins,
			Store:        store,
			Up:           up,
			Config:       cfg,
			Cockpit:      cockpit.Page(),
			Toggle:       toggle,
			Start:        time.Now(),
			FrameRules:   frameRules,
			DefaultAgent: defaultAgent,
			TotalBudget:  cfg.TotalBudget(),
		})
		go func() {
			if err := http.Serve(ln, ctrl.Handler()); err != nil {
				slog.Warn("control: server stopped", "err", err)
			}
		}()
	}

	// Một sự kiện "hệ đã mở" nên một hình, in SAU mọi cảnh báo ở trên: chỗ hỏng đọc trước,
	// tổng kết đọc sau. Chi tiết từng hàng chỉ hiện ở mức debug, và nằm ngay dưới hàng của
	// nó chứ không thành bản ghi riêng (§ Quan sát được).
	verbose := level == logging.LevelDebug
	banner := logging.Banner{Title: "W.O.N Proxy Inject", At: time.Now()}
	// Nói địa chỉ này LÀ gì, không nói tên biến môi trường của một hãng: proxy đứng trước cả
	// ba định dạng, nên một tên biến cụ thể ở đây đúng cho một công cụ và sai cho phần còn
	// lại. Tên biến của từng công cụ thuộc QUICKSTART, không thuộc dòng khởi động.
	banner.Add("proxy", logging.WebAddr(cfg.Listen), "← Base URL for all requests")
	banner.Add("cockpit", logging.WebAddr(cockpitAddr))
	banner.Add("upstream", cfg.Upstream, upstreamNote(cfg.UpstreamDeclared))
	banner.Add("agents", soulSummary(book, defaultAgent))
	if verbose {
		banner.Detail(strings.Join(book.Names(), " "))
	}
	banner.Add("ground", groundSummary(groundFiles))
	if verbose {
		banner.Detail(strings.Join(groundNames(groundFiles), " "))
	}
	banner.Add("plugins", strings.Join(pluginNames(plugins), " "))
	banner.Add("conf", confLabel(cfg), "log="+level+" · debug_log="+onOff(debugLogOn(cfg, level)))
	console.Print(banner)

	if err := http.ListenAndServe(cfg.Listen, prx); err != nil {
		fatal("server: %v", err)
	}
}

// upstreamNote — chú thích chỉ có khi đích là mặc định. Khai rõ thì không cần chú gì.
func upstreamNote(declared bool) string {
	if declared {
		return ""
	}
	return "(default — this machine)"
}

// soulSummary — số đệ, có bản đồ hệ không, và đệ mặc định. Đệ mặc định phải hiện ở dòng
// khởi động: nó là cái đoán duy nhất lõi được phép làm (#6).
func soulSummary(book *soul.Book, defaultAgent string) string {
	s := strconv.Itoa(len(book.Names())) + " souls"
	if book.House() != "" {
		s += " + house"
	}
	return s + " · default " + orNone(defaultAgent)
}

// confLabel — file cấu hình thật đang chạy. Vắng file là một cấu hình chạy đủ, không
// phải một cái thiếu.
func confLabel(cfg *config.Config) string {
	if cfg.ConfPath == "" {
		return "(none — built-in defaults)"
	}
	// Rút gọn chỉ khi file nằm trong cây: ngoài cây thì `filepath.Rel` trả chuỗi `../..`
	// dài hơn chính đường dẫn.
	if rel, err := filepath.Rel(cfg.Paths.Root, cfg.ConfPath); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return cfg.ConfPath
}

// groundSummary — số file rồi phân bổ theo thư mục gốc, đúng thứ tự nạp. Tên từng file
// chỉ hiện ở mức debug, dưới hàng `ground` của khối khởi động.
func groundSummary(files []ground.File) string {
	if len(files) == 0 {
		return ""
	}
	count := map[string]int{}
	var order []string
	for _, f := range files {
		head := f.Rel
		if i := strings.IndexByte(head, '/'); i >= 0 {
			head = head[:i]
		}
		if _, seen := count[head]; !seen {
			order = append(order, head)
		}
		count[head]++
	}
	parts := []string{strconv.Itoa(len(files)) + " files"}
	for _, k := range order {
		if count[k] == 1 && strings.HasSuffix(k, ".md") {
			parts = append(parts, k) // file lẻ ở gốc: gọi đúng tên nó, không phải "x 1"
			continue
		}
		parts = append(parts, k+" "+strconv.Itoa(count[k]))
	}
	return strings.Join(parts, " · ")
}

// listenControl — mở cửa buồng lái. Rỗng (không khai) → không cửa, không lỗi: đó là
// trạng thái bình thường, không phải một cái thiếu.
func listenControl(addr string) (net.Listener, error) {
	if addr == "" {
		return nil, nil
	}
	return net.Listen("tcp", addr)
}

// fatal — chết thì phải NHÌN THẤY, ở mọi mức log. `log.Fatalf` đi qua handler của slog:
// mức `silent` ghi vào io.Discard nên tiến trình tắt mà không một chữ nào ra, và mức
// thường thì một cái chết hiện lên như một dòng INFO. Bất biến #3 nói im lặng là mặc định
// của *ngữ cảnh chèn*, không phải của cái chết.
func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "W.O.N Proxy Inject stopped: "+format+"\n", args...)
	os.Exit(1)
}

// debugDir — nhà của hồ sơ chẩn bệnh, hoặc rỗng để lõi tắt nó im lặng. Núm riêng
// (`debug_log`) chứ không theo mức log: nhật ký mang thân request (#5).
func debugDir(cfg *config.Config, level string) string {
	if !debugLogOn(cfg, level) {
		return ""
	}
	return cfg.Paths.Run()
}

// debugLogOn — lời khai thắng mức log; chưa khai thì theo mức log.
func debugLogOn(cfg *config.Config, level string) bool {
	if cfg.DebugLog != nil {
		return *cfg.DebugLog
	}
	return level == logging.LevelDebug
}

// needLocalModel — plugin nào trong roster nghĩ bằng model nền. Danh sách nằm ở main
// vì đây là wiring: lõi không biết plugin nào dùng service nào.
func needLocalModel(names []string) []string {
	voices := map[string]bool{"loiterer": true, "outfitter": true, "wayfarer": true}
	var need []string
	for _, n := range names {
		if voices[n] {
			need = append(need, n)
		}
	}
	return need
}

// pluginNames — plugin đã DỰNG được, không phải danh sách đã khai: dòng khởi động
// phải nói cái đang chạy.
func pluginNames(plugins []plugin.Plugin) []string {
	out := make([]string, 0, len(plugins))
	for _, p := range plugins {
		out = append(out, p.Name())
	}
	return out
}

// groundNames — file nào thật sự thành đất, theo đúng thứ tự chèn.
func groundNames(files []ground.File) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, f.Rel)
	}
	return out
}

// onOff — trạng thái một núm bật/tắt, cho khối khởi động.
func onOff(on bool) string {
	if on {
		return "on"
	}
	return "off"
}

// orNone — dòng khởi động phải nói cái đang chạy, kể cả khi nó là "không có".
func orNone(s string) string {
	if s == "" {
		return "(none — passthrough when unrecognized)"
	}
	return s
}
