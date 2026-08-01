// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeConf dựng một gốc W.O.N tạm với won.conf cho sẵn, trả về đường dẫn gốc.
func writeConf(t *testing.T, body string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, ".system", "proxy")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, confFileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// Không có won.conf: proxy vẫn chạy bằng Options.Roster (wiring do main quyết)
// + giá trị mặc định. Core không mang default — test truyền roster tường minh.
func TestDefaultsNoFile(t *testing.T) {
	roster := []string{"identity", "loiterer", "memory", "wayfarer"}
	cfg, err := Load(Options{Root: t.TempDir(), Roster: roster})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != DefaultListen {
		t.Errorf("listen = %q, want default %q", cfg.Listen, DefaultListen)
	}
	if cfg.TotalBudgetMs != defaultTotalMs {
		t.Errorf("total = %d, want %d", cfg.TotalBudgetMs, defaultTotalMs)
	}
	if got := cfg.EnabledPlugins(); len(got) != len(roster) {
		t.Errorf("roster = %v, want %v", got, roster)
	}
}

// Trần cả lượt là núm duy nhất của lõi cho thời gian chèn; trần riêng MỘT plugin là lời
// khai của chính plugin (`<tên>.budget_ms`, đọc ở plugins/base), không phải núm của lõi.
// `plugin_budget_ms` đã GỠ: nó trùng vùng với hai cái kia và đọc lên như cái thứ ba.
func TestNoCoreWidePluginBudget(t *testing.T) {
	cfg, err := Load(Options{Root: writeConf(t, "total_budget_ms = 30000\n")})
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.TotalBudget().Milliseconds(); got != 30000 {
		t.Errorf("total = %dms, want 30000", got)
	}
	if _, err := Load(Options{Root: writeConf(t, "plugin_budget_ms = 5000\n")}); err == nil {
		t.Fatal("plugin_budget_ms đã gỡ — khoá này phải là lỗi khởi động, không nuốt im lặng")
	}
}

// timeout_ms và max_tokens là hai trần của một lượt gọi model nền — phải tới
// được client, không dừng ở loader. Không khai thì có mặc định, không để trống.
func TestLocalModelCeilings(t *testing.T) {
	cfg, err := Load(Options{Root: writeConf(t, "model = qwen3.5:4b\ntimeout_ms = 20000\nmax_tokens = 64\n")})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Services.LocalModel.TimeoutMs != 20000 {
		t.Errorf("timeout = %d, want 20000", cfg.Services.LocalModel.TimeoutMs)
	}
	if cfg.Services.LocalModel.MaxTokens != 64 {
		t.Errorf("max_tokens = %d, want 64", cfg.Services.LocalModel.MaxTokens)
	}

	bare, err := Load(Options{Root: writeConf(t, "model = qwen3.5:4b\n")})
	if err != nil {
		t.Fatal(err)
	}
	if bare.Services.LocalModel.MaxTokens != defaultMaxTokens {
		t.Errorf("max_tokens = %d, want default %d", bare.Services.LocalModel.MaxTokens, defaultMaxTokens)
	}
}

// temperature có BA trạng thái, và trạng thái thứ ba là chỗ dễ mất nhất: `off` = không gửi
// field, tức trả quyền cho nhà cung cấp. Không có đường ấy thì mọi lượt gọi đều mang một con
// số của ta và không còn cách nào về. Còn số ngoài tầm quen thì ĐI QUA: mặc định là lời
// khuyên của hệ, không phải cái cổng.
func TestTemperatureHasThreeStates(t *testing.T) {
	temp := func(t *testing.T, body string) *float64 {
		t.Helper()
		cfg, err := Load(Options{Root: writeConf(t, "model = qwen3.5:4b\n"+body)})
		if err != nil {
			t.Fatal(err)
		}
		return cfg.Services.LocalModel.Temperature
	}

	if got := temp(t, ""); got == nil || *got != defaultTemperature {
		t.Errorf("chưa khai → mặc định %v, got %v", defaultTemperature, got)
	}
	if got := temp(t, "temperature = 0.15\n"); got == nil || *got != 0.15 {
		t.Errorf("khai 0.15 → 0.15, got %v", got)
	}
	if got := temp(t, "temperature = off\n"); got != nil {
		t.Errorf("`off` phải là KHÔNG gửi field, got %v", *got)
	}
	// `0` là một lời khai hợp lệ, khác hẳn `off`. Gộp chúng là mất một nghĩa.
	if got := temp(t, "temperature = 0\n"); got == nil || *got != 0 {
		t.Errorf("khai 0 → 0 (không phải nil), got %v", got)
	}
	if got := temp(t, "temperature = 1.8\n"); got == nil || *got != 1.8 {
		t.Errorf("số ngoài tầm quen vẫn phải đi qua — lõi không phán thay người khai, got %v", got)
	}
	if got := temp(t, "temperature = nóng\n"); got == nil || *got != defaultTemperature {
		t.Errorf("chữ không đọc được → mặc định, không vỡ khởi động, got %v", got)
	}
}

// think là núm boolean, HAI giá trị. `off` từng là đường thứ ba để không gửi field đi, và
// nó là một cái bẫy: với model có khối suy nghĩ, VẮNG field không bằng `false` — nhà cung
// cấp tự bật, trần chữ đi hết vào khối suy nghĩ, và lời đáp về rỗng. Test này khoá việc
// núm ấy không mọc lại trạng thái thứ ba.
func TestThinkIsPlainBoolean(t *testing.T) {
	think := func(t *testing.T, body string) bool {
		t.Helper()
		cfg, err := Load(Options{Root: writeConf(t, "model = qwen3.5:4b\n"+body)})
		if err != nil {
			t.Fatal(err)
		}
		return cfg.Services.LocalModel.Think
	}

	if got := think(t, ""); got != defaultThink {
		t.Errorf("chưa khai → mặc định %v, got %v", defaultThink, got)
	}
	if !think(t, "think = true\n") {
		t.Error("khai true phải ra true")
	}
	if think(t, "think = false\n") {
		t.Error("khai false phải ra false")
	}
	if got := think(t, "think = off\n"); got != defaultThink {
		t.Errorf("`off` là chữ không đọc được ở núm boolean → về mặc định %v, got %v", defaultThink, got)
	}
}

// Control API là buồng lái local: chỉ loopback được nhận; "off" = tắt.
func TestControlLoopback(t *testing.T) {
	cases := []struct {
		name    string
		control string
		wantErr bool
	}{
		{"off means disabled", "off", false},
		{"empty defaults to loopback", "", false},
		{"loopback IPv4", "127.0.0.1:8788", false},
		{"localhost", "localhost:8788", false},
		{"loopback IPv6", "[::1]:8788", false},
		{"all interfaces blocked", "0.0.0.0:8788", true},
		{"LAN address blocked", "192.168.1.7:8788", true},
		{"missing port", "127.0.0.1", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Load(Options{Root: t.TempDir(), Control: c.control})
			if c.wantErr && err == nil {
				t.Fatalf("control=%q must be rejected", c.control)
			}
			if !c.wantErr && err != nil {
				t.Fatalf("control=%q must be accepted: %v", c.control, err)
			}
		})
	}
}

// "off" tắt hẳn Control API (Control rỗng trong Config).
func TestControlOff(t *testing.T) {
	cfg, err := Load(Options{Root: t.TempDir(), Control: "off"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Control != "" {
		t.Errorf("control = %q, want empty (disabled)", cfg.Control)
	}
}

// won.conf: `<tên>.enable` bật plugin, key=value chỉnh core, plugin.key đặt option.
func TestConfEnableAndOptions(t *testing.T) {
	root := writeConf(t, `
# experiment
loiterer.enable = true
upstream = https://api.anthropic.com
loiterer.faces = A, B, C
`)
	cfg, err := Load(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Upstream != "https://api.anthropic.com" {
		t.Errorf("upstream = %q", cfg.Upstream)
	}
	if len(cfg.EnabledPlugins()) != 1 {
		t.Fatalf("enabled = %v, want only [loiterer]", cfg.EnabledPlugins())
	}
	env := cfg.Plugins["loiterer"]
	if !env.Enabled || !strings.Contains(string(env.Options), "faces") {
		t.Errorf("loiterer options = %s", env.Options)
	}
}

// Cờ CLI thắng won.conf.
func TestCliOverridesFile(t *testing.T) {
	root := writeConf(t, "loiterer.enable = true\nupstream = https://from-file\n")
	cfg, err := Load(Options{Root: root, Upstream: "https://from-cli"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Upstream != "https://from-cli" {
		t.Errorf("upstream = %q, CLI flag must beat file", cfg.Upstream)
	}
}

// Local-first: LUÔN có một đích, và cái đích không ai phải khai là chính máy này. Không
// có trạng thái "không có đích" — một hệ không đích thì mọi lượt là một lỗi.
func TestUpstreamAlwaysHasATarget(t *testing.T) {
	cases := []struct {
		name     string
		conf     string
		flag     string
		want     string
		declared bool
	}{
		{"vắng key → máy này", "loiterer.enable = true\n", "", DefaultTarget, false},
		{"không có cả file → máy này", "", "", DefaultTarget, false},
		{"khai rỗng cũng là máy này", "upstream =\n", "", DefaultTarget, false},
		{"khai URL → đúng URL", "upstream = https://api.anthropic.com\n", "", "https://api.anthropic.com", true},
		{"cờ thắng file", "upstream = https://from-file\n", "https://from-cli", "https://from-cli", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			if c.conf != "" {
				root = writeConf(t, c.conf)
			}
			cfg, err := Load(Options{Root: root, Upstream: c.flag, Roster: []string{"identity"}})
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Upstream != c.want {
				t.Errorf("upstream = %q, want %q", cfg.Upstream, c.want)
			}
			if cfg.UpstreamDeclared != c.declared {
				t.Errorf("declared = %v, want %v — dòng khởi động phải phân biệt người chọn với mặc định", cfg.UpstreamDeclared, c.declared)
			}
		})
	}

	// Model nền cũng trỏ vào cùng một chỗ, và cùng MỘT hằng: hai literal là hai bản lệch
	// được mà không ai báo.
	cfg, err := Load(Options{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Services.LocalModel.BaseURL != DefaultTarget {
		t.Errorf("base_url = %q, want %q", cfg.Services.LocalModel.BaseURL, DefaultTarget)
	}
}

// Tuỳ chọn cho plugin KHÔNG có dòng `.enable` = lỗi rõ (gõ nhầm tên, hoặc quên khai).
func TestOptionWithoutEnableLine(t *testing.T) {
	root := writeConf(t, "loiterer.faces = A, B\n")
	if _, err := Load(Options{Root: root}); err == nil {
		t.Fatal("tuỳ chọn mà thiếu dòng .enable phải là lỗi")
	}
}

// `enable = false` thì các dòng tuỳ chọn của plugin ĐƯỢC PHÉP nằm lại: tắt một plugin
// không được buộc người khai phải xoá lời khai của mình. Đây là cái bẫy mà núm bật/tắt
// phải gỡ được, nên nó có ca riêng.
func TestDisabledPluginKeepsItsOptions(t *testing.T) {
	root := writeConf(t, "loiterer.enable = false\nloiterer.faces = A, B\n")
	cfg, err := Load(Options{Root: root})
	if err != nil {
		t.Fatalf("enable = false + tuỳ chọn phải chạy được: %v", err)
	}
	if got := cfg.EnabledPlugins(); len(got) != 0 {
		t.Fatalf("enabled = %v, muốn rỗng", got)
	}
}

// Giá trị lạ ở `.enable` là lỗi, không đoán thành false: đoán thì plugin im mà người
// khai tưởng đã bật.
func TestEnableRejectsNonBoolean(t *testing.T) {
	for _, v := range []string{"yes", "1", "on", ""} {
		root := writeConf(t, "loiterer.enable = "+v+"\n")
		if _, err := Load(Options{Root: root}); err == nil {
			t.Fatalf("enable = %q phải là lỗi", v)
		}
	}
}

// Dòng chỉ có chữ (cách bật plugin cũ) là lỗi, và lỗi phải chỉ ra cách viết mới.
func TestBareLineIsAnError(t *testing.T) {
	root := writeConf(t, "loiterer\n")
	_, err := Load(Options{Root: root})
	if err == nil {
		t.Fatal("dòng trần phải là lỗi")
	}
	if !strings.Contains(err.Error(), "loiterer.enable = true") {
		t.Fatalf("lỗi phải chỉ cách viết mới, got %v", err)
	}
}

// `enable` là quyết định của tầng config, không phải tuỳ chọn của plugin: để nó lọt vào
// Options thì ParseOptions coi là khoá lạ và mọi plugin dựng hỏng.
func TestEnableNotPassedToPlugin(t *testing.T) {
	root := writeConf(t, "loiterer.enable = true\n")
	cfg, err := Load(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if raw := string(cfg.Plugins["loiterer"].Options); strings.Contains(raw, "enable") {
		t.Fatalf("options = %s — `enable` phải bị nhấc ra", raw)
	}
}

// Khoá lạ trong won.conf = lỗi, chống typo âm thầm.
func TestUnknownKey(t *testing.T) {
	root := writeConf(t, "bogus = 1\n")
	if _, err := Load(Options{Root: root}); err == nil {
		t.Fatal("unknown key must be an error")
	}
}

// -conf trỏ tường minh vào file không tồn tại = lỗi; còn mặc định vắng thì im.
func TestExplicitConfMissing(t *testing.T) {
	if _, err := Load(Options{Root: t.TempDir(), ConfPath: filepath.Join(t.TempDir(), "nonexistent.conf")}); err == nil {
		t.Fatal("-conf pointing to missing file must be an error")
	}
}

// Không won.conf + Roster nil/[] → 0 plugin, không panic: lõi chạy như reverse
// proxy trong suốt. Roster rỗng là một lời khai hợp lệ, không phải lỗi khởi động.
func TestEmptyRosterZeroPlugins(t *testing.T) {
	t.Run("nil roster", func(t *testing.T) {
		cfg, err := Load(Options{Root: t.TempDir(), Roster: nil})
		if err != nil {
			t.Fatalf("nil roster must be valid, no error: %v", err)
		}
		if got := cfg.EnabledPlugins(); len(got) != 0 {
			t.Fatalf("nil roster → 0 plugins, got %v", got)
		}
	})
	t.Run("empty roster", func(t *testing.T) {
		cfg, err := Load(Options{Root: t.TempDir(), Roster: []string{}})
		if err != nil {
			t.Fatalf("empty roster must be valid, no error: %v", err)
		}
		if got := cfg.EnabledPlugins(); len(got) != 0 {
			t.Fatalf("empty roster → 0 plugins, got %v", got)
		}
	})
}

// Lõi không mang tên plugin cụ thể — Options.Roster là nguồn "bật" duy nhất
// khi không có won.conf. Có won.conf thì file thắng,
// Options.Roster bị bỏ qua (file là nguồn bền, ưu tiên hơn wiring mặc định).
func TestFileOverridesRoster(t *testing.T) {
	root := writeConf(t, "loiterer.enable = true\n")
	cfg, err := Load(Options{Root: root, Roster: []string{"identity", "wayfarer"}})
	if err != nil {
		t.Fatal(err)
	}
	got := cfg.EnabledPlugins()
	if len(got) != 1 || got[0] != "loiterer" {
		t.Fatalf("won.conf must beat Options.Roster, got %v", got)
	}
}

// Thứ tự plugin là thứ tự KHAI, không phải thứ tự chữ cái: nó quyết thứ tự các
// khối trong lời hệ thống, tức nó nằm trong tiền tố cache. Bản sắc đứng trước ký ức
// là một quyết định đọc được ở won.conf, không phải may mắn của bảng chữ cái.
func TestPluginOrderFollowsDeclaration(t *testing.T) {
	root := writeConf(t, "wayfarer.enable = true\nidentity.enable = true\nmemory.enable = true\n")
	cfg, err := Load(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"wayfarer", "identity", "memory"}
	got := cfg.EnabledPlugins()
	if len(got) != len(want) {
		t.Fatalf("enabled = %v, muốn %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("thứ tự = %v, muốn đúng thứ tự khai %v", got, want)
		}
	}
}

// Vắng won.conf thì thứ tự của roster mặc định (main mang) cũng phải được giữ.
func TestRosterOrderKept(t *testing.T) {
	roster := []string{"identity", "memory", "loiterer"}
	cfg, err := Load(Options{Root: t.TempDir(), Roster: roster})
	if err != nil {
		t.Fatal(err)
	}
	for i, name := range cfg.EnabledPlugins() {
		if name != roster[i] {
			t.Fatalf("roster order = %v, muốn %v", cfg.EnabledPlugins(), roster)
		}
	}
}

// Đường dẫn dẫn xuất từ một gốc: đổi bố cục thì sửa core/paths, không sửa loader.
func TestPathsDerivedFromRoot(t *testing.T) {
	root := t.TempDir()
	cfg, err := Load(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Paths.Root != root {
		t.Errorf("root = %q, muốn %q", cfg.Paths.Root, root)
	}
	for _, p := range []string{cfg.Paths.Agents(), cfg.Paths.State(), cfg.Paths.Conf()} {
		if !strings.HasPrefix(p, root) {
			t.Errorf("%q phải neo vào gốc %q", p, root)
		}
	}
}

// Đệ mặc định: vắng key thì có sẵn (nhiều công cụ chủ không có chỗ chọn đệ, và bắt
// người dán trọn soul mỗi lần gọi là thủ tục rườm rà), `off` thì tắt hẳn.
//
// `none` KHÔNG còn là chữ tắt. Một núm hai chữ đồng nghĩa thì file mẫu chỉ dạy được một,
// và cái không được dạy thành mặt ẩn — sống trong code mà không ai biết để dùng hay để bỏ.
// Giờ nó là một cái tên như mọi cái tên khác, và sổ soul là chỗ phán tên ấy có ai không.
func TestDefaultAgentKnob(t *testing.T) {
	cases := map[string]string{
		"":     "Tzu",
		"off":  "",
		"none": "none",
		"Sun":  "Sun",
		" mo ": "mo", // tên rửa khoảng trắng; hoa thường để sổ soul quyết
	}
	for in, want := range cases {
		if got := pickAgent(in); got != want {
			t.Errorf("pickAgent(%q) = %q, muốn %q", in, got, want)
		}
	}
}

// `strip_spans` KHÔNG phải một khoá lõi hiểu, và không được thành một. Cắt chữ trần của
// công cụ chủ đòi chép câu của một ứng dụng cụ thể vào hệ, mà số ứng dụng nhét chữ vào lời
// hệ thống thì không đếm được — nên lõi không nhận việc đó (§ TestUntaggedHostTextIsLeftAlone).
// Khoá lạ phải vỡ RÕ ở lúc khởi động, không nuốt im lặng.
func TestStripSpansIsNotACoreKey(t *testing.T) {
	if _, err := Load(Options{Root: writeConf(t, "strip_spans = A … B\n")}); err == nil {
		t.Fatal("strip_spans đã gỡ — khoá này phải là lỗi khởi động")
	}
}
