// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

// Package config dựng cấu hình từ ba nguồn, nguồn trước thắng nguồn sau:
// cờ CLI > won.conf > mặc định. Đường dẫn neo vào gốc W.O.N. won.conf là text
// phẳng: dòng trần = bật plugin, `#` = tắt, `key = value` = chỉnh.
package config

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"won/proxy/core/paths"
)

// Mặc định — tham số phụ dùng sẵn, không cần khai báo.
const (
	// Mặc định ở đây KHỚP từng dòng đang bật trong won.conf.example: xoá won.conf đi thì
	// hành vi không đổi, và file mẫu không phải nói một con số mà lõi lại chạy con khác.
	//
	// defaultTotalMs — trần cho CẢ một lượt chèn. Bốn plugin dùng model chạy song song
	// trong Gather, nhưng Ollama xử lý tuần tự khi OLLAMA_NUM_PARALLEL = 1, nên trần riêng
	// của chúng cộng dồn: 4 × `budget_ms` = 80s, nên trần cả lượt là cái cắt trước.
	defaultTotalMs   = 60000
	confFileName     = "won.conf"
	defaultTimeoutMs = 15000
	defaultMaxTokens = 160   // trần chữ model nền — agent bờ trả một dòng
	defaultAgent     = "Tzu" // đệ dùng khi công cụ chủ không nói đang gọi ai

	defaultModel     = "qwen3.5:4b"
	defaultKeepAlive = "30m"

	// defaultThink — `false` tắt khối suy nghĩ của model nền. Ollama nhận field này cả với
	// model không biết suy nghĩ, nên gửi sẵn không loại model nào; để bật thì khối ấy ăn hết
	// max_tokens và lời đáp về rỗng.
	defaultThink = false

	// defaultTemperature — con số này bị kẹp giữa hai phía. Không thấp hơn: ba agent bờ
	// được quyền IM, mà im là nhánh thiểu số — bản sao hội thoại chiếm gần hết prompt, nên
	// nhánh dễ nhất luôn là nói một câu về nó; về gần 0 thì model luôn lấy nhánh đỉnh và
	// `∅` không còn cửa nào. Không cao hơn: model nền chép khuôn bài mẫu dài nhất nó thấy
	// (§ Vì sao model nhỏ), và giữ khuôn là việc của chính con số này.
	defaultTemperature = 0.4

	// defaultStripSections — mục trong sách hướng dẫn của công cụ chủ không thuộc tay đệ.
	// Phần dạy dùng đồ nghề KHÔNG có ở đây, và đó là chủ đích.
	defaultStripSections = "Tone and style, Text output, auto memory"

	// Khung công cụ chủ đã gặp thật. Không phải danh sách đóng — mỗi công cụ bọc một
	// kiểu, nên nó là giá trị mặc định của một núm, không phải hằng của lõi.
	//   VS Code:     <instructions> <modeInstructions> <userRequest>
	//   Claude Code: <system-reminder>
	defaultStripTags  = "system-reminder, instructions, modeInstructions, workspace_info"
	defaultUnwrapTags = "userRequest"

	// Khối mở đầu bằng "You are " là lời khẳng định căn cước của công cụ chủ — căn
	// cước thứ hai cạnh <Soul>, lệch vai đệ nếu để vào. Bắt bằng "You are " để đủ rộng
	// cho mọi biến thể (Claude Code, VS Code, Cursor…). Metadata của nhà cung cấp
	defaultStripIdentity = "You are "
)

// offValue — MỘT chữ để tắt một núm, cho mọi núm không phải boolean. Một chữ vì hai chữ
// đồng nghĩa thì file mẫu chỉ dạy được một, và cái không được dạy trở thành mặt ẩn: nó
// sống trong code mà không ai biết để mà dùng, cũng không ai biết để mà bỏ. Núm boolean
// không dùng chữ này — chúng đã có `false`, và một núm hai đường tắt là một núm khó đọc.
const offValue = "off"

// Địa chỉ mặc định — MỘT nhà duy nhất cho ba địa chỉ này. Exported vì dòng help của
// cờ CLI ở main phải in đúng chúng: một literal chép sang chỗ khác là một literal
// lệch được mà không ai báo.
//
// DefaultTarget là chính máy này, và nó là đích của cả dòng chính lẫn model nền. Một mặc
// định như thế không phá #6: cái #6 cấm là đoán một người NHẬN — chữ không rời máy, khoá
// không tới tay ai, và vắng người thì lỗi nổ ngay ở lượt đầu.
const (
	DefaultListen  = "127.0.0.1:8787"
	DefaultControl = "127.0.0.1:7777"
	DefaultTarget  = "http://127.0.0.1:11434"

	// DefaultLogLevel — GIÁ TRỊ mặc định ở đây, còn việc quy một tên lạ về mức nào là
	// việc của main. Hai vai khác nhau, nên hai chỗ; nhưng giá trị thì một nhà, để buồng
	// lái không hiện `""` trong khi hệ đang chạy ở `info`.
	DefaultLogLevel = "info"
)

// Options — giá trị từ cờ CLI. Rỗng = "không truyền", nhường nguồn sau lo.
// Roster = plugin bật khi không có won.conf — wiring mặc định do main quyết.
type Options struct {
	Root     string
	Listen   string
	Control  string
	Upstream string
	ConfPath string
	Roster   []string
}

// Envelope — phần lõi hiểu về một plugin. Options giao nguyên vẹn cho plugin.
type Envelope struct {
	Enabled bool
	Options json.RawMessage
}

// LocalModel — đích, trần thời gian, trần chữ của một lượt gọi model nền.
// Temperature nil = khai `off`: không gửi field, để nhà cung cấp tự quyết.
type LocalModel struct {
	Model       string   `json:"model"`
	Think       bool     `json:"think"`
	Temperature *float64 `json:"temperature,omitempty"`
	BaseURL     string   `json:"base_url"`
	KeepAlive   string   `json:"keep_alive,omitempty"`
	TimeoutMs   int      `json:"timeout_ms"`
	MaxTokens   int      `json:"max_tokens"`
}

type Services struct {
	LocalModel LocalModel
}

type Config struct {
	Listen   string
	Control  string
	Upstream string
	LogLevel string // debug|info|silent — rỗng hoặc tên lạ = info
	// ConfPath — file cấu hình THẬT đã đọc; rỗng = không có file nào, hệ chạy mặc định.
	ConfPath string
	// DebugLog — có ghi hồ sơ chẩn bệnh xuống `run/` không; núm riêng vì nó ghi THÂN REQUEST
	// xuống đĩa (#5), khác việc in ra màn hình. Ba trạng thái: nil = theo `log_level`,
	// true/false = lời khai thắng (§ Cấu hình).
	DebugLog *bool
	// UpstreamDeclared — người khai có nói đích hay không. Dòng khởi động phải phân biệt
	// "đích do người chọn" với "đích mặc định vì không ai nói": hai chuyện khác nhau.
	UpstreamDeclared bool
	// DefaultAgent — đệ dùng khi không nhận ra căn cước nào. "off" ở won.conf = tắt,
	// và tắt nghĩa là lượt không nhận ra ai đi qua nguyên bản.
	DefaultAgent string

	Paths    paths.Tree
	Services Services
	// Ground — mẫu đường dẫn khối đất, đúng thứ tự khai; thứ tự nằm trong tiền tố cache.
	// Ba trạng thái: xem groundPatterns.
	Ground []string

	TotalBudgetMs int
	// Bốn luật cắt khung — nghĩa từng luật ở request.FrameRules, chỗ chúng được dùng.
	// Khai RỖNG tường minh ở won.conf = không cắt loại ấy (xem tagList).
	StripTags     []string
	UnwrapTags    []string
	StripSections []string
	StripIdentity []string

	Plugins map[string]Envelope
	// pluginOrder — thứ tự dựng, giữ đúng thứ tự khai. Map không có thứ tự, mà thứ tự
	// chèn là một phần của tiền tố cache: xem EnabledPlugins.
	pluginOrder []string
}

// Load dựng Config từ cờ CLI + won.conf + mặc định. Roster rỗng → 0 plugin:
// core chạy như reverse proxy trong suốt, không Fatalf. Lỗi chỉ khi -conf trỏ
// tường minh vào file không tồn tại hoặc lỗi cú pháp.
func Load(opts Options) (*Config, error) {
	tree := paths.Tree{Root: resolveRoot(opts.Root)}

	confPath := opts.ConfPath
	explicit := confPath != ""
	if confPath == "" {
		confPath = tree.Conf()
	}
	file, err := parseConf(confPath)
	if err != nil {
		if explicit || !os.IsNotExist(err) {
			return nil, fmt.Errorf("won.conf %s: %w", confPath, err)
		}
		file = &confData{}
	}
	// Chỉ ghi đường dẫn khi file có thật: vắng won.conf là một cấu hình chạy đủ.
	loadedConf := ""
	if file.present {
		loadedConf = confPath
	}

	declaredUpstream := firstNonEmpty(opts.Upstream, file.core["upstream"])
	cfg := &Config{
		ConfPath:         loadedConf,
		Listen:           firstNonEmpty(opts.Listen, file.core["listen"], DefaultListen),
		Upstream:         pickUpstream(declaredUpstream),
		UpstreamDeclared: strings.TrimSpace(declaredUpstream) != "",
		Ground:           groundPatterns(file),
		Paths:            tree,
		TotalBudgetMs:    atoiOr(file.core["total_budget_ms"], defaultTotalMs),
		StripTags:        tagList(file, "strip_tags", defaultStripTags),
		UnwrapTags:       tagList(file, "unwrap_tags", defaultUnwrapTags),
		StripSections:    tagList(file, "strip_sections", defaultStripSections),
		StripIdentity:    tagList(file, "strip_identity", defaultStripIdentity),
		LogLevel:         firstNonEmpty(file.core["log_level"], DefaultLogLevel),
		DebugLog:         boolOrNil(file.core["debug_log"]),
		DefaultAgent:     pickAgent(file.core["default_agent"]),
		Services: Services{LocalModel: LocalModel{
			BaseURL:     firstNonEmpty(file.core["base_url"], DefaultTarget),
			Model:       firstNonEmpty(file.core["model"], defaultModel),
			TimeoutMs:   atoiOr(file.core["timeout_ms"], defaultTimeoutMs),
			MaxTokens:   atoiOr(file.core["max_tokens"], defaultMaxTokens),
			Think:       thinkOr(file.core["think"], defaultThink),
			Temperature: floatOr(file.core["temperature"], defaultTemperature),
			KeepAlive:   firstNonEmpty(file.core["keep_alive"], defaultKeepAlive),
		}},
		Plugins: map[string]Envelope{},
	}

	// control: cờ > file > mặc định. `off` = tắt hẳn, và đó là cách DUY NHẤT để tắt.
	// Mặc định BẬT vì endpoint chấm điểm ký ức (`PUT /plugins/memory/update`) sống dưới
	// Control API: tắt nó là `memory.scorer` không còn đường gọi nào.
	ctrl := firstNonEmpty(opts.Control, file.core["control"], DefaultControl)
	if ctrl == offValue {
		ctrl = ""
	}
	cfg.Control = ctrl
	if cfg.Control != "" {
		if err := mustLoopback(cfg.Control); err != nil {
			return nil, fmt.Errorf("control: %w", err)
		}
	}

	// Plugin: có won.conf → theo `<tên>.enable`; không có file → Options.Roster.
	roster := opts.Roster
	if file.present {
		// Tuỳ chọn cho plugin KHÔNG có dòng `.enable` = gõ nhầm tên plugin, hoặc quên khai
		// nó. Vỡ rõ, không nuốt. Còn `enable = false` thì các dòng tuỳ chọn của nó ĐƯỢC
		// PHÉP nằm lại: giữ lời khai của mình trong lúc tắt plugin là việc bình thường,
		// và bắt xoá chúng đi là cái bẫy duy nhất mà một núm bật/tắt phải gỡ được.
		for name, o := range file.opts {
			if _, declared := o[optEnable]; !declared {
				return nil, fmt.Errorf("won.conf: %q has options but no on/off line — add %q or %q",
					name, name+"."+optEnable+" = true", name+"."+optEnable+" = false")
			}
		}
		roster = nil
		for _, name := range file.enableOrder {
			on, err := enableFlag(file.opts[name][optEnable])
			if err != nil {
				return nil, fmt.Errorf("won.conf: %s.%s: %w", name, optEnable, err)
			}
			if on {
				roster = append(roster, name)
			}
		}
	}
	for _, name := range roster {
		if _, dup := cfg.Plugins[name]; dup {
			continue
		}
		raw, err := pluginOptions(file.opts[name])
		if err != nil {
			return nil, fmt.Errorf("options %q: %w", name, err)
		}
		cfg.Plugins[name] = Envelope{Enabled: true, Options: raw}
		cfg.pluginOrder = append(cfg.pluginOrder, name)
	}
	return cfg, nil
}

// optEnable — núm bật/tắt của MỌI plugin. Nó là quyết định của tầng config (có dựng
// plugin ấy hay không), nên nó bị nhấc ra trước khi options tới tay plugin: `ParseOptions`
// coi khoá lạ là lỗi, nên để nó lọt vào là mọi plugin dựng hỏng.
const optEnable = "enable"

// pluginOptions dựng options gửi cho plugin, trừ `enable`. Không còn khoá nào → nil, để
// plugin đọc đúng "không khai gì" thay vì một object rỗng.
func pluginOptions(o map[string]any) (json.RawMessage, error) {
	rest := make(map[string]any, len(o))
	for k, v := range o {
		if k != optEnable {
			rest[k] = v
		}
	}
	if len(rest) == 0 {
		return nil, nil
	}
	return json.Marshal(rest)
}

// enableFlag đọc `<tên>.enable`. Chỉ `true`/`false`, không nhận gì khác: một giá trị lạ
// đoán ra thành `false` là plugin im mà người khai tưởng đã bật.
func enableFlag(v any) (bool, error) {
	s, _ := v.(string)
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("must be true or false, got %q", s)
	}
}

// confData — kết quả parse won.conf. present=false = không có file.
type confData struct {
	core        map[string]string         // ground, listen, control, upstream, ...
	opts        map[string]map[string]any // plugin.key = value, kể cả `enable`
	enableOrder []string                  // plugin có dòng `.enable`, đúng thứ tự xuất hiện
	present     bool
}

// parseConf đọc won.conf: bỏ comment (`#` tới cuối dòng) và dòng trống. MỌI dòng có
// nghĩa đều là `khoá = giá trị`; một dòng chỉ có chữ là lỗi, kèm cách viết đúng.
func parseConf(path string) (*confData, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	d := &confData{present: true, core: map[string]string{}, opts: map[string]map[string]any{}}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' {
			continue
		}
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = strings.TrimSpace(line[:i])
			if line == "" {
				continue
			}
		}
		if eq := strings.IndexByte(line, '='); eq >= 0 {
			key := strings.TrimSpace(line[:eq])
			val := strings.TrimSpace(line[eq+1:])
			if dot := strings.IndexByte(key, '.'); dot >= 0 {
				plugin, opt := key[:dot], key[dot+1:]
				if d.opts[plugin] == nil {
					d.opts[plugin] = map[string]any{}
				}
				if opt == optEnable {
					if _, dup := d.opts[plugin][optEnable]; !dup {
						d.enableOrder = append(d.enableOrder, plugin)
					}
					d.opts[plugin][optEnable] = val // giữ nguyên chữ: enableFlag soát nó
					continue
				}
				d.opts[plugin][opt] = typedValue(val)
			} else if knownCoreKey(key) {
				d.core[key] = val
			} else {
				return nil, fmt.Errorf("unknown key %q", key)
			}
			continue
		}
		return nil, fmt.Errorf("line %q has no `=` — enable a plugin with %q", line, line+"."+optEnable+" = true")
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return d, nil
}

// CoreKeys — mọi khoá lõi hiểu trong won.conf. MỘT danh sách, ba chỗ đọc: cửa nhận khoá
// (knownCoreKey), buồng lái hiện giá trị đang chạy, và test khoá hai chỗ ấy không lệch
// nhau. Liệt kê tay ở từng chỗ thì mọi lệch đều im lặng: khoá thiếu ở cửa bị chối oan,
// thiếu ở buồng lái thì không ai thấy nó đang mang giá trị gì.
var CoreKeys = []string{
	"think", "model", "listen", "ground", "control", "upstream", "debug_log",
	"base_url", "keep_alive", "log_level", "max_tokens", "timeout_ms", "default_agent",
	"total_budget_ms", "temperature", "strip_tags", "unwrap_tags",
	"strip_sections", "strip_identity",
}

func knownCoreKey(k string) bool { return slices.Contains(CoreKeys, k) }

// thinkOr — núm boolean, hai giá trị, KHÔNG có đường `off` như floatOr ngay dưới: với model
// có khối suy nghĩ, vắng field không bằng `false` — nhà cung cấp tự bật, trần chữ đi hết vào
// khối suy nghĩ, lời đáp về rỗng. Chữ không đọc được cũng về mặc định, cùng luật với atoiOr.
func thinkOr(v string, def bool) bool {
	if b := boolOrNil(v); b != nil {
		return *b
	}
	return def
}

// floatOr — chưa khai thì mặc định, `off` thì KHÔNG gửi field đi, còn lại là con số người
// khai. Số không đọc được cũng về mặc định, cùng luật với atoiOr.
//
// KHÔNG chặn khoảng, và đó là chủ đích: trần của mỗi nhà cung cấp mỗi khác, nên lõi chặn
// theo trần của nhà nào thì cũng là lõi phán thay người khai. Chỉ mặc định là lời khuyên
// của hệ; phần còn lại là quyền của người giữ hệ.
func floatOr(v string, def float64) *float64 {
	t := strings.TrimSpace(v)
	switch strings.ToLower(t) {
	case "":
		d := def
		return &d
	case offValue:
		return nil
	}
	if f, err := strconv.ParseFloat(t, 64); err == nil {
		return &f
	}
	d := def
	return &d
}

// boolOrNil — chưa khai thì nil, và nil nghĩa là không gửi field đó đi.
func boolOrNil(v string) *bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true":
		t := true
		return &t
	case "false":
		f := false
		return &f
	}
	return nil
}

// pickUpstream — luôn có một đích, và cái đích không ai phải khai là chính máy này. Không
// có trạng thái "không có đích": một hệ không đích thì mọi lượt là một lỗi, mà mặc định
// không rời máy thì không có gì phải cân nhắc để chọn nó.
func pickUpstream(v string) string {
	if v = strings.TrimSpace(v); v != "" {
		return v
	}
	return DefaultTarget
}

// groundPatterns đọc núm `ground`. Ba trạng thái, không hai: vắng key → mặc định; khai
// `off` hoặc rỗng → tắt đất hẳn; khai danh sách → đúng danh sách ấy. Gộp "chưa khai" với
// "khai là không có" thì không còn cách nào tắt đất mà vẫn giữ file README.
func groundPatterns(file *confData) []string {
	v, declared := file.core["ground"]
	if !declared {
		return defaultGround()
	}
	if s := strings.ToLower(strings.TrimSpace(v)); s == "" || s == offValue {
		return nil
	}
	return splitCSV(v)
}

// defaultGround — README rồi trọn ba trục, sinh từ paths.Axis đúng thứ tự What → Own →
// Need. README nói hệ là gì; ba trục nói người dùng là ai, thiếu nó thì đệ đứng trên đất
// chung mà không biết đang đứng cạnh ai.
func defaultGround() []string {
	out := make([]string, 0, len(paths.Axis)+1)
	out = append(out, "./"+paths.Readme)
	for _, a := range paths.Axis {
		out = append(out, a+"/**/*.md")
	}
	return out
}

// typedValue đoán kiểu từ text: có phẩy → danh sách; toàn số → int; còn lại → chuỗi.
func typedValue(v string) any {
	if strings.Contains(v, ",") {
		return splitCSV(v)
	}
	if n, err := strconv.Atoi(v); err == nil {
		return n
	}
	return v
}

// resolveRoot: cờ -root nếu có; nếu không, dò ngược tới thư mục chứa `.system/`.
func resolveRoot(flagRoot string) string {
	if flagRoot != "" {
		if abs, err := filepath.Abs(flagRoot); err == nil {
			return abs
		}
		return flagRoot
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return paths.Find(cwd)
}

// mustLoopback — Control API chỉ chấp nhận loopback.
func mustLoopback(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("address %q invalid: %w", addr, err)
	}
	if host == "localhost" {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return nil
	}
	return fmt.Errorf("address %q not loopback — Control API must listen locally", addr)
}

// EnabledPlugins — plugin được bật, ĐÚNG thứ tự khai (dòng trần trong won.conf, hoặc
// roster của main khi không có file). Thứ tự này là thứ tự khối trong lời hệ thống, nên
// nó nằm trong tiền tố cache: đảo nó là bắt hội thoại đang chạy tính token lại từ đầu
// đúng một lần, rồi rẻ trở lại.
func (c *Config) EnabledPlugins() []string {
	names := make([]string, 0, len(c.pluginOrder))
	for _, name := range c.pluginOrder {
		if c.Plugins[name].Enabled {
			names = append(names, name)
		}
	}
	return names
}

func (c *Config) TotalBudget() time.Duration {
	return time.Duration(c.TotalBudgetMs) * time.Millisecond
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func atoiOr(s string, def int) int {
	if s == "" {
		return def
	}
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// pickAgent — đệ mặc định. Vắng key = defaultAgent; `off` = tắt hẳn, và tắt nghĩa là lượt
// không nhận ra căn cước nào sẽ đi qua nguyên bản.
func pickAgent(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "":
		return defaultAgent
	case offValue:
		return ""
	}
	return strings.TrimSpace(v)
}

// tagList — danh sách tag khai ở won.conf. Vắng key = mặc định; khai RỖNG tường minh
// (`strip_tags =`) = không cắt loại ấy. Phân biệt hai cái đó là điều kiện để tắt được
// một luật mà không phải xoá cả dòng.
func tagList(file *confData, key, fallback string) []string {
	v, declared := file.core[key]
	if !declared {
		v = fallback
	}
	return splitCSV(v)
}
