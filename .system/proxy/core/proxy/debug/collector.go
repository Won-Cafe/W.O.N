// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package debug

import (
	"hash/fnv"
	"net/http"
	"time"

	"won/proxy/core/plugin"
	"won/proxy/core/request"
	"won/proxy/core/usage"
)

// Tài liệu nhật ký chẩn bệnh: gom dữ liệu một request rồi vẽ lại TRỌN phiên. Chỉ sống
// ở log_level=debug; `Collector.on=false` thì mọi việc nặng ở đây đóng cửa. Ghi file: log.go.
//
// File này chở SỐ ĐO và HỎI–ĐÁP, không chở lời hệ thống hay danh mục tool: cấu trúc thân
// đã nằm nguyên trong hai file thân, lặp lại ở đây chỉ tổ dư. Bù lại `asked`/`replied`
// KHÔNG có trần — chỗ duy nhất trong nhật ký đọc được trọn chữ.

// Stage — một chặng của dòng chính, ra `flow` của từng lần chạy. Struct chứ không map:
// thứ tự field là thứ tự đọc, không phải thứ tự chữ cái. Vắng chặng `marshal` nghĩa là
// lượt ấy không bị sửa gì và thân đi ra là thân chủ gửi.
type Stage struct {
	Name     string                `json:"name"`
	Ms       int64                 `json:"ms"`
	BytesIn  int                   `json:"bytes_in,omitempty"`
	BytesOut int                   `json:"bytes_out,omitempty"`
	Inserted int                   `json:"inserted,omitempty"`
	Failed   bool                  `json:"failed,omitempty"`
	Plugins  []plugin.PluginDetail `json:"plugins,omitempty"`
}

// Collector gom dữ liệu một request, chia đôi khi ghi: phần không đổi ra `head`, phần
// của lượt ra `turns`. Field mainline gán trực tiếp thì xuất, field nội bộ giữ chữ thường.
type Collector struct {
	on bool // nhật ký đang mở; tắt thì collector chỉ giữ ts và usage

	ts     time.Time
	path   string
	format string
	target string
	model  string

	Agent       string
	Session     string
	PrevSession string // khoá chưa thăng cấp, nếu lượt này vừa thăng cấp

	humans []string // lời người mở từng lượt, nguyên văn
	stages []Stage

	// Chính sách của cảnh — ra khối `head`, không lặp mỗi lượt.
	Framed      bool
	SystemOwned bool

	// Lõi đặt gì vào lượt NÀY — của lần chạy, không của phiên: khối đã có mặt trong lời
	// hệ thống đang tới thì appendBlock không chạm, nên hai cờ này đổi theo lượt.
	Ground bool
	House  bool

	Opened string // mốc mở phiên, vào tên thư mục để hai hội thoại cùng khoá không đè nhau
	Usage  *usage.Stats
	err    string
	cut    string

	// Hai con trỏ tới thân THẬT — không cắt, không copy. Việc cắt làm ở Log.Write, tức sau
	// khi thân đã đi ra upstream (xem cut.go).
	rawIn  []byte
	rawOut []byte
	marks  []request.CacheMark
}

// NewCollector — agent là header căn cước ĐỌC THÔ, giá trị ban đầu trước khi lõi resolve.
// log nil (nhật ký tắt) vẫn trả Collector hợp lệ, chỉ mọi việc nặng tự đóng cửa.
func NewCollector(log *Log, now time.Time, r *http.Request, format request.Format, target string, agent string) *Collector {
	return &Collector{
		on: log != nil, ts: now, path: r.URL.Path,
		format: format.String(), target: target, Agent: agent,
	}
}

// Stage ghi một chặng. extra đặt các con số riêng của chặng đó.
func (c *Collector) Stage(name string, from time.Time, extra func(*Stage)) {
	if !c.on {
		return
	}
	s := Stage{Name: name, Ms: time.Since(from).Milliseconds()}
	if extra != nil {
		extra(&s)
	}
	c.stages = append(c.stages, s)
}

// ReadBody rút lời người mở từng lượt — chữ duy nhất tài liệu này còn tự đọc lấy từ thân —
// và chuỗi khối để đo tiền tố. Gọi SAU khi lõi đã chèn xong và TRƯỚC Marshal: đo đúng cái
// sắp đi ra, không đo cái vừa nhận vào.
func (c *Collector) ReadBody(b *request.Body, placed []string, rules request.FrameRules) {
	if !c.on {
		return
	}
	c.humans = b.HumanTexts(placed, rules)
	c.model = b.Model()
	c.marks = b.CacheMarks()
}

// SnapshotIn giữ con trỏ tới thân chủ gửi. Gọi NGAY sau khi đọc được thân: lượt hỏng ở
// bất cứ chặng nào sau đó vẫn còn "công cụ chủ gửi lên cái gì" trong file thân.
func (c *Collector) SnapshotIn(in []byte) {
	if !c.on {
		return
	}
	c.rawIn = in
}

// SnapshotOut giữ con trỏ tới thân đi ra.
func (c *Collector) SnapshotOut(out []byte) {
	if !c.on {
		return
	}
	c.rawOut = out
}

// Fail ghi câu hỏng của lượt. Thân nguyên bản không phải giữ thêm ở đây: SnapshotIn đã
// chụp nó trước mọi nhánh, nên file thân vẫn trả lời được câu ấy.
func (c *Collector) Fail(msg string) { c.err = msg }

// Cut ghi lượt bị cắt giữa dòng. nil (chảy trọn) → không ghi gì. Tách khỏi Fail vì client
// tự ngắt không phải lỗi của ai; nó chỉ là lý do bản ghi không có lời đáp.
func (c *Collector) Cut(err error) {
	if !c.on || err == nil {
		return
	}
	c.cut = err.Error()
}

// sessionDoc — trọn nhật ký một phiên, dựng lại mỗi lần ghi. Request mang trọn lịch sử
// nên phiên vẽ lại được từ nó; proxy chỉ phải tự gom nhịp chặng và usage.
type sessionDoc struct {
	Head  headEntry `json:"head"`
	Turns []turnDoc `json:"turns"`
}

// headEntry — nhận dạng phiên: cái KHÔNG đổi giữa các lượt. Cái đổi được (model, target,
// lõi chèn gì) nằm ở từng lần chạy, vì dòng đầu chỉ giữ được giá trị của lần cuối và các
// lần trước sẽ bị dán nhãn sai.
type headEntry struct {
	Agent   string     `json:"agent"`
	Format  string     `json:"format"`
	Opened  string     `json:"opened"`
	Path    string     `json:"path"`
	Session string     `json:"session"`
	Policy  policyView `json:"policy"`
}

// policyView — cảnh của khung công cụ chủ. Ở `head` vì không đổi giữa các lượt.
type policyView struct {
	HostFramed  bool `json:"host_framed"`
	SystemOwned bool `json:"system_owned"`
}

// turnDoc — một lượt người: câu hỏi mở lượt, rồi các lần proxy chạy trong lượt đó. Một
// câu hỏi, n lần chạy, mỗi lần một câu trả lời.
type turnDoc struct {
	Turn  int      `json:"turn"`
	Asked string   `json:"asked,omitempty"`
	Runs  []runRec `json:"runs,omitempty"`
}

// runRec — một lần proxy chạy. Replied/Thinking/ToolCalls rút ngay từ response của CHÍNH
// lần chạy này — không đợi lượt sau: nếu đây là lượt CUỐI của phiên thì "lượt sau" không
// bao giờ tới, và không có field riêng thì không ai thấy đệ đã nói gì.
//
// Field xếp theo nhóm: nhận dạng · lượt hỏng · chữ · số đo · danh sách.
type runRec struct {
	Ms     int64  `json:"ms"`
	Ts     string `json:"ts"`
	Model  string `json:"model,omitempty"`  // model của CHÍNH lần chạy này
	Target string `json:"target,omitempty"` // đích đã resolve, đổi nóng được giữa phiên

	Error string `json:"error,omitempty"`
	// Cut — lượt bị cắt giữa dòng, kèm lời của context. Vắng nghĩa là thân chảy trọn.
	Cut string `json:"cut,omitempty"`
	// BodyIn — thân đã cắt, CHỈ ở bản ghi không có phiên: ở đó không có thư mục phiên
	// nào để đặt file thân, mà "công cụ chủ gửi lên cái gì" là câu duy nhất còn lại.
	BodyIn string `json:"body_in,omitempty"`

	Replied  string `json:"replied,omitempty"`
	Thinking string `json:"thinking,omitempty"`

	Inserted insertedView `json:"inserted"`
	Cache    *cacheView   `json:"cache,omitempty"`
	Usage    *usage.Stats `json:"usage,omitempty"`

	ToolCalls []string `json:"tool_calls,omitempty"`
	Flow      []Stage  `json:"flow,omitempty"`
}

// insertedView — lõi đặt gì vào lượt này.
type insertedView struct {
	Ground bool `json:"ground"`
	House  bool `json:"house"`
}

// cacheView — bao nhiêu phần của lần chạy này DÙNG LẠI được của lần chạy trước, đo trên
// cấu trúc chứ không hỏi upstream. Ranh với `usage`: đây nói hệ có ĐÁNG được cache không,
// `usage` nói nhà cung cấp CHO bao nhiêu (§ Cache).
type cacheView struct {
	Kept  int `json:"kept"` // số khối đầu trùng khít lần chạy trước
	Of    int `json:"of"`   // tổng số khối lần này
	Reuse int `json:"reuse_pct"`

	SharedBytes int `json:"shared_bytes"`
	TotalBytes  int `json:"total_bytes"`
	// LostBytes — byte lần trước đã gửi mà lần này bỏ đi: phần upstream ghi vào cache rồi
	// không lần nào dùng lại được.
	LostBytes int `json:"lost_bytes,omitempty"`

	Broke string `json:"broke,omitempty"` // khối đầu tiên lệch, ở lần này
	Was   string `json:"was,omitempty"`   // khối đứng đúng chỗ đó ở lần trước
}

// compareMarks — tiền tố chung dài nhất giữa chuỗi lần này và lần chạy TRƯỚC của cùng
// phiên. Lần đầu không có gì để so nên trả nil: nhật ký khai cái đo được, không khai 0%
// cho một phép đo chưa chạy.
func compareMarks(prev, now []request.CacheMark) *cacheView {
	if len(prev) == 0 || len(now) == 0 {
		return nil
	}
	v := &cacheView{Of: len(now)}
	for _, m := range now {
		v.TotalBytes += m.Bytes
	}
	i := 0
	for i < len(prev) && i < len(now) && prev[i].Hash == now[i].Hash && prev[i].Slot == now[i].Slot {
		v.SharedBytes += now[i].Bytes
		i++
	}
	v.Kept = i
	if v.TotalBytes > 0 {
		v.Reuse = v.SharedBytes * 100 / v.TotalBytes
	}
	for _, m := range prev[i:] {
		v.LostBytes += m.Bytes
	}
	if i < len(now) {
		v.Broke = markName(now[i])
	}
	if i < len(prev) {
		v.Was = markName(prev[i])
	}
	return v
}

// markName — tên gọi được của một khối: vai, kèm nhãn khi khối tự khai tên.
func markName(m request.CacheMark) string {
	if m.Label == "" {
		return m.Slot
	}
	return m.Slot + " " + m.Label
}

// Write dựng lại trọn tài liệu của phiên rồi ghi đè file của nó. Nil-safe.
func (l *Log) Write(c *Collector) {
	if l == nil {
		return
	}
	rec := runRec{
		Ms: time.Since(c.ts).Milliseconds(), Ts: c.ts.Format(time.RFC3339Nano),
		Model: c.model, Target: c.target, Error: c.err, Cut: c.cut,
		Inserted: insertedView{Ground: c.Ground, House: c.House},
		Flow:     cutCallSystem(c.stages),
	}
	if c.Usage != nil {
		if c.Usage.Seen || c.Usage.Encoding != "" {
			rec.Usage = c.Usage
		}
		rec.ToolCalls = c.Usage.Calls()
		// Hai luồng chữ, hai field: trộn lại thì suy nghĩ chiếm hết chỗ và lời đáp thật bị
		// đẩy ra sau — đo được trên một lượt Gemini thật.
		rec.Replied = c.Usage.Text()
		rec.Thinking = c.Usage.Thinking()
	}

	// Không có khoá phiên (lỗi trước khi nhận ra đệ) → file rời, thân đi kèm vì ở đó không
	// có thư mục phiên nào.
	if c.Session == "" {
		rec.BodyIn = cutText(string(c.rawIn))
		l.writeOther(rec)
		return
	}
	// Khoá thăng cấp: dời file cũ và mang theo nhịp đã gom.
	if c.PrevSession != "" && c.PrevSession != c.Session {
		l.renameSession(c.Agent, c.PrevSession, c.Session, c.Opened)
		l.migrateRecs(c.PrevSession, c.Session)
	}
	rec.Cache = l.compareRun(c.Session, c.marks)

	turns := splitTurns(c.humans)
	// Nhịp gắn vào NỘI DUNG lượt người, không vào số thứ tự lượt: retry sau khi sửa lời
	// là một lượt khác, và bản ghi của bản nháp cũ không được đội lốt lượt đang chảy.
	l.remember(c.Session, turnKey(turns[len(turns)-1].Asked), rec)
	for i := range turns {
		turns[i].Runs = l.recall(c.Session, turnKey(turns[i].Asked))
	}
	l.writeSession(c.Agent, c.Session, c.Opened, sessionDoc{
		Head: headEntry{
			Agent: request.AgentOrUnknown(c.Agent), Format: c.format, Opened: c.Opened,
			Path: c.path, Session: c.Session,
			Policy: policyView{HostFramed: c.Framed, SystemOwned: c.SystemOwned},
		},
		Turns: turns,
	}, c.rawIn, c.rawOut)
}

// splitTurns — mỗi lời người mở một lượt. Không có lời người nào (mới chỉ có vùng system
// và vòng máy) → một lượt số 0, để nhịp của lần chạy vẫn có nhà.
func splitTurns(humans []string) []turnDoc {
	if len(humans) == 0 {
		return []turnDoc{{Turn: 0}}
	}
	turns := make([]turnDoc, 0, len(humans))
	for i, h := range humans {
		turns = append(turns, turnDoc{Turn: i + 1, Asked: h})
	}
	return turns
}

// turnKey — vân tay lời người mở lượt. Sửa lời rồi gửi lại → khoá khác → bản ghi cũ
// không hiện ra dưới lượt mới.
func turnKey(human string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(human))
	return h.Sum64()
}
