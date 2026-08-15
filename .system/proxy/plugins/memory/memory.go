// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

// Plugin memory mở kho ký ức: mọi lượt rót index các trang, và khi trang chạm
// tới lượt đang chảy thì rót trọn trang đó. Chọn trang là bước cơ học dùng model
// nền, không phải agent bờ thứ tư: ba người đứng bờ là ba giọng — nói một dòng,
// có marker, chịu luật "được nói, không được cầm". Bước này không nói; nó chọn mở
// trang nào. Thứ nó giao ra là trang của chính người dùng, nguyên văn, kèm nguồn.
package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"won/proxy/core/paths"
	"won/proxy/core/plugin"
	"won/proxy/core/request"
	"won/proxy/core/session"
	"won/proxy/plugins/base"
	"won/proxy/services/localmodel"
)

func init() { plugin.Register("memory", New) }

const (
	// Lượt đầu chỉ đưa index: đệ chưa có gì để trang phải chạm vào. Đếm bằng lượt
	// NGƯỜI, không bằng lần chạy: một lượt người kéo hàng chục lần chạy, nên đọc lần
	// chạy là mở trang ngay trong lượt đầu.
	firstPickTurn = 2

	defaultMaxOpenPerTurn = 2

	// defaultStoneWeight — mỗi confirm = +stoneWeight, mỗi contest = −.
	defaultStoneWeight = 10
	// defaultScorer — agent được phép gọi scoring endpoint.
	defaultScorer = "Shu"

	// noteSpent — sổ kho đã tính xong và không còn gì để nói trong phiên này: hoặc nó vốn
	// rỗng, hoặc nó đã được đặt xuống một lần. Một dấu cho cả hai vì bên đọc hỏi đúng một
	// câu — còn phải nói gì nữa không.
	noteSpent = "∅"

	// defaultMaxIndexPerZone — trần dòng index MỖI VÙNG, không phải trần cả kho: một
	// trần cả kho sẽ cắt theo thứ tự vùng và xoá trọn `personal/` (vùng đọc sau cùng).
	//
	// Nó cũng là trần thật của việc mở trang: bộ chọn chỉ chọn được trong index, nên
	// số vùng nhân trần này là trần cho cả phiên. Một núm, không phải hai.
	defaultMaxIndexPerZone = 20
)

// options — ngưỡng là chính sách, không phải khẩu vị của code. Vùng KHÔNG là núm:
// bốn vùng là hình của kho, không phải khẩu vị người khai (paths.Zones).
type options struct {
	MaxOpenPerTurn  int    `json:"max_open_per_turn"`
	MaxIndexPerZone int    `json:"max_index_per_zone"`
	StoneWeight     int    `json:"stone_weight"`
	Scorer          string `json:"scorer"`
}

// chatter — chỉ phần lõi plugin cần ở model nền, để test không phải gọi mạng.
type chatter interface {
	Chat(ctx context.Context, system, user string) (string, error)
}

type Memory struct {
	base.Base
	llm          chatter
	perTurn      int
	indexPerZone int
	stoneWeight  int
	scorer       string
	scoreMu      sync.Mutex
}

func New(env plugin.Env) (plugin.Plugin, error) {
	b := base.New(env)
	if !b.Paths.Known() {
		return nil, fmt.Errorf("memory needs the W.O.N root path")
	}
	var o options
	if err := b.ParseOptions(&o); err != nil {
		return nil, fmt.Errorf("memory options: %w", err)
	}
	m := &Memory{Base: b, perTurn: o.MaxOpenPerTurn, indexPerZone: o.MaxIndexPerZone}
	if m.perTurn < 1 {
		m.perTurn = defaultMaxOpenPerTurn
	}
	if m.indexPerZone < 1 {
		m.indexPerZone = defaultMaxIndexPerZone
	}
	m.stoneWeight = o.StoneWeight
	if m.stoneWeight < 1 {
		m.stoneWeight = defaultStoneWeight
	}
	m.scorer = o.Scorer
	if m.scorer == "" {
		m.scorer = defaultScorer
	}
	// Gán qua cửa nil: *Client nil nhét vào interface thành interface KHÔNG nil,
	// và khi đó `p.llm == nil` nói dối.
	if lm := b.LLM(); lm != nil {
		m.llm = lm
	}
	return m, nil
}

func (p *Memory) Name() string { return "memory" }

func (p *Memory) Contribute(ctx context.Context, snap *request.Snapshot, sess *session.Session) ([]*plugin.Contribution, error) {
	// Index dựng MỘT LẦN cho mỗi phiên rồi nằm lại. Quét đĩa mỗi lần chạy là trả hai
	// lần giá cho cùng một thứ: một lượt người kéo hàng chục lần chạy, và kho ghi thêm
	// giữa phiên thì khối index đổi ngay trong lời hệ thống đang đứng.
	lines, paths, dropped := sess.MemIndex()
	if lines == "" {
		pages, drop := p.list()
		if len(pages) == 0 {
			// Kho rỗng không phải im lặng TRỌN. Lời mời viết trang đầu sống trong `renderUse`,
			// mà `renderUse` nằm SAU cửa này — nên khi kho rỗng, thứ duy nhất mở được kho lại
			// chính là thứ bị cửa này chặn. Mồi nằm trong cửa nó phải mở, và vòng không khép:
			// đo trên kho thật, `procedural/` rỗng trọn qua 80 phiên (§ renderUse).
			//
			// Một lời, đúng một lần mỗi phiên, đi chung đường `takeNote` với mọi lời gọi việc
			// khác — nên nó cũng chịu đủ ba cửa ở đó: lượt của người, đã qua lượt đầu, chưa tiêu.
			if sess.Note() == "" {
				sess.SetNote(renderSeed(p.scorer))
			}
			if text := p.takeNote(snap, sess); text != "" {
				return plugin.One(&plugin.Contribution{Kind: plugin.KindMarker, Tag: "Memory", Text: text}), nil
			}
			return nil, nil
		}
		lines, dropped = indexBlock(pages, p.stoneWeight), drop
		paths = make([]string, len(pages))
		for i, pg := range pages {
			paths[i] = pg.Path
		}
		sess.SetMemIndex(lines, paths, dropped)
		sess.SeePages(paths)
		if sess.Note() == "" {
			n := nudge(pages, sess, p.stoneWeight, p.scorer)
			if n == "" {
				n = noteSpent // đã tính, không có gì — không tính lại
			}
			sess.SetNote(n)
		}
	}
	p.pick(ctx, snap, sess, lines, paths)
	// HAI NHỊP, HAI KHỐI. Index đứng cả phiên nên nó vào lời hệ thống — chỗ đứng yên,
	// và ở nhà cung cấp có cache thì chỗ ấy rẻ dần. Trang mở đổi theo lượt nên nó xuống
	// cuối mảng messages, vai người.
	//
	// Cái được đo thấy KHÔNG phải cache: đích đang chạy không trả trường cache nào. Cái
	// được là lõi chỉ đặt nhịp lượt ở lượt của người, nên khối trang không đi lại ở từng
	// lần chạy trong một vòng tool — đo trên một lượt thật: năm lần chạy, một lần gửi.
	//
	// Khối chỉ kể trang mở TRONG lượt này, không kể dồn: lõi ghim khối của mỗi lượt lại
	// đúng chỗ nó (§ Cache), nên kể dồn là mỗi khối chở lại trọn phần khối trước đã chở.
	out := []*plugin.Contribution{{
		Kind: plugin.KindSystem,
		Tag:  "Memory",
		Text: renderIndex(lines, dropped, len(paths), p.scorer, p.scoreRoute()),
	}}
	// MỘT tiếng, một message: hai khối của cùng memory đi chung một đóng góp, vì lõi ghim
	// theo từng đóng góp và hai đóng góp là hai khối phải tự dò lại chỗ đứng.
	//
	// Cả khối chỉ dựng ở lượt của người, cùng cửa với `Apply`: giữa vòng tool lõi không đặt
	// khối mới xuống, nên dựng nó ở đó là dựng một thứ chắc chắn bị bỏ. Khối cũ vẫn về chỗ
	// đã ghim — đường ấy đi qua sổ phiên, không qua đây. Đo trên một lượt thật: 16 lần chạy,
	// 15 lần dựng thừa, và nhật ký đọc ra như thể trang được chèn lại mỗi lần.
	if snap != nil && snap.HumanSpokeLast {
		if text := joinBlocks(p.takeNote(snap, sess), renderOpened(sess.OpenedAt(sess.Turns()))); text != "" {
			out = append(out, &plugin.Contribution{Kind: plugin.KindMarker, Tag: "Memory", Text: text})
		}
	}
	return out, nil
}

// takeNote trả sổ kho ĐÚNG MỘT LẦN mỗi phiên rồi đánh dấu đã tiêu. Nó là thứ duy nhất
// trong khối này gọi tới một việc, nên nó đi ở nhịp lượt chứ không nằm trong lời hệ
// thống: một lời gọi việc chôn dưới tiền tố là một lời bị lướt qua. Lõi ghim khối của
// lượt lại đúng chỗ nó (§ Cache), nên nói một lần là nó nằm trong ngữ cảnh tới hết phiên.
//
// Cùng cửa lượt với bộ chọn, cùng một lý do: lượt đầu đệ chưa có gì để sổ phải chạm vào.
// Và cửa `HumanSpokeLast` phải khớp cửa của `Apply` — tiêu sổ ở một lần chạy mà lõi
// không đặt khối xuống là mất hẳn lời nhắc của cả phiên.
func (p *Memory) takeNote(snap *request.Snapshot, sess *session.Session) string {
	if snap == nil || !snap.HumanSpokeLast || sess.Turns() < firstPickTurn {
		return ""
	}
	n := sess.Note()
	if n == "" || n == noteSpent {
		return ""
	}
	sess.SetNote(noteSpent)
	return n
}

// scoreRoute — đường tới cửa sỏi, dựng từ chính tên plugin: lõi mount extension dưới
// `/plugins/<tên>/`, nên chép cứng chuỗi là một bản lệch được khi tên đổi. Không kèm
// host:port — địa chỉ Control API là cấu hình của lõi, plugin không biết và không đoán (#6).
func (p *Memory) scoreRoute() string { return "PUT /plugins/" + p.Name() + "/update" }

// joinBlocks nối các khối không rỗng bằng một dòng trống, bỏ qua khối rỗng.
func joinBlocks(parts ...string) string {
	kept := make([]string, 0, len(parts))
	for _, s := range parts {
		if s != "" {
			kept = append(kept, s)
		}
	}
	return strings.Join(kept, "\n\n")
}

// pick chọn trang để mở, MỖI LƯỢT NGƯỜI một lần. Index chốt ở đầu phiên nên nó có
// giới hạn của nó; những lần mở về sau là phần bù — một trang chỉ hoá liên quan khi
// câu chuyện đã đi tới đó thì không lượt đầu nào đoán ra được.
//
// Ba cửa đóng trước khi hỏi model: chưa tới lượt người, chưa qua lượt đầu, hoặc đã hỏi
// trong chính lượt này. Không có trần riêng cho cả phiên — bộ chọn chỉ chọn được trong
// index, nên trần index đã là trần của nó.
func (p *Memory) pick(ctx context.Context, snap *request.Snapshot, sess *session.Session, lines string, paths []string) {
	// Trang mở đi theo nhịp lượt, mà lõi chỉ đặt nhịp lượt ở lượt của người (§ Chỗ đứng
	// của tiếng lượt) — mở giữa vòng tool là dựng một khối không ai đặt xuống. Khối index
	// thì vẫn đi mọi lần chạy; cửa này chỉ đóng đường mở thêm.
	if !snap.HumanSpokeLast || sess.Turns() < firstPickTurn || p.llm == nil {
		return
	}
	// Một lượt người kéo hàng chục lần chạy. Không có cửa này thì "mỗi lượt một lần hỏi"
	// thành "mỗi lần chạy một lần hỏi", và kho mở sạch trong đúng một lượt.
	if sess.Turns() <= sess.PickedAt() {
		return
	}
	var cand []string
	for _, path := range paths {
		if !sess.HasOpened(path) {
			cand = append(cand, path)
		}
	}
	if len(cand) == 0 {
		return
	}
	// Trần phải vào TỚI hợp đồng, không chỉ vào bộ lọc lời đáp: bảo model "nhiều nhất
	// hai" rồi cắt ở bốn là núm nâng lên không có tác dụng. Kẹp theo số ứng viên vì
	// một hợp đồng xin nhiều trang hơn số đang có là ô trống mời điền.
	limit := min(p.perTurn, len(cand))

	// Bộ chọn chỉ đọc INDEX, không đọc ruột trang: index đã có tiêu đề và dòng mô tả của
	// từng trang, nên chở ruột lên chỉ để chọn là trả giá cho thứ không dùng (§ Ký ức).
	out, err := p.Ask(ctx, sess, p.llm, selectorSystem(lines, limit),
		localmodel.RenderUser(snap, "Recipient", snap.Agent))
	// Ghi sổ ngay sau khi hỏi, kể cả khi hỏng: lần hỏi của lượt này đã tiêu rồi. Ghi sau
	// khi thành công thì một model đứt sẽ bị hỏi lại ở mọi lần chạy còn lại của lượt.
	sess.SetPickedAt(sess.Turns())
	if err != nil {
		return // model nền hỏng → chỉ còn index, dòng chính vẫn chảy (#2)
	}
	// Trang mở ra bằng DÀN Ý: heading do người viết đặt nên nó không sai được. Đệ đọc dàn
	// ý rồi tự quyết có cần trọn trang hay không; đường dẫn nằm ngay đó.
	for _, path := range parsePicks(out, cand, limit) {
		text, err := p.read(path)
		if err != nil {
			continue
		}
		sess.Open(path, outline(text))
	}
}

// read đọc một trang trong kho: bỏ frontmatter và khoảng trắng hai đầu. Frontmatter là
// sổ sỏi của máy — nó đã hiện ở index, chở nó vào ngữ cảnh lần nữa là nhiễu.
func (p *Memory) read(path string) (string, error) {
	b, err := os.ReadFile(filepath.Join(p.Paths.Memory(), path))
	if err != nil {
		return "", err
	}
	body, _ := splitFrontmatter(string(b))
	return strings.TrimSpace(body), nil
}

// page — một dòng index. head và desc là hai dòng đầu có nghĩa: thiếu desc thì
// model nền chọn sai hoặc bịa (29–67% so với 89%; điều kiện đo ở selectorHead —
// số chờ đo lại). S và F là sỏi từ frontmatter.
type page struct {
	Path string    // "vùng/tên.md"
	Head string    // dòng `# …`
	Desc string    // dòng in nghiêng ngay dưới tiêu đề
	S    int       // confirm — sỏi cộng
	F    int       // contest — sỏi trừ
	Mod  time.Time // lần cuối có người đặt bút — thứ tự index và cái trần đọc nó
}

// list quét các vùng, trả index và số trang bị bỏ vì quá trần mỗi vùng.
func (p *Memory) list() ([]page, int) {
	var out []page
	dropped := 0
	for _, zone := range paths.Zones {
		zp := p.listZone(zone)
		if len(zp) > p.indexPerZone {
			dropped += len(zp) - p.indexPerZone
			zp = zp[len(zp)-p.indexPerZone:] // giữ phần cuối = trang mới
		}
		out = append(out, zp...)
	}
	return out, dropped
}

// listZone đọc một vùng, sắp theo LẦN SỬA CUỐI — cũ trước, mới sau. Không sắp theo
// tên: cái trần trên kia giữ phần cuối để giữ trang còn sống, mà "còn sống" là lần
// cuối có người đặt bút, không phải chữ cái. Tên mở đầu bằng ngày thì hai cách trùng
// nhau, nhưng `personal/self.md` và mọi trang không theo nếp ngày thì không.
func (p *Memory) listZone(zone string) []page {
	dir := p.Paths.Zone(zone)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil // vùng chưa có — không phải lỗi
	}
	var out []page
	for _, e := range entries {
		if e.IsDir() || !paths.IsPage(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue // trang biến mất giữa ReadDir và Info — bỏ, không chặn cả vùng
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		body, fm := splitFrontmatter(string(b))
		s, f := parseFrontmatter(fm)
		head, desc := headAndDesc(body)
		if head == "" && desc == "" {
			continue // trang rỗng ruột
		}
		if !hasBody(body) {
			continue // mới có khung, chưa có ruột — mở ra không đưa thêm gì ngoài dòng index
		}
		out = append(out, page{Path: zone + "/" + e.Name(), Head: head, Desc: desc, S: s, F: f, Mod: info.ModTime()})
	}
	// Hai trang cùng dấu thời gian thì tên phân xử — thứ tự phải tất định, vì nó là
	// thứ tự các dòng trong lời hệ thống.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Mod.Equal(out[j].Mod) {
			return out[i].Path < out[j].Path
		}
		return out[i].Mod.Before(out[j].Mod)
	})
	return out
}

// hasBody — trang có gì ngoài KHUNG không. Khung là: dòng tiêu đề đầu, dòng in nghiêng
// trọn vẹn, đường kẻ, và khoảng trắng.
//
// Vì sao cần, và nó không phải chuyện lý thuyết: `personal/self.md` có đủ tiêu đề và mô tả
// nên nó qua được cửa "rỗng ruột" ở trên, vào index, rồi bộ chọn mở nó — mà ruột trang
// đúng một dòng `*(chưa có — chờ moments củng cố)*`. Đo trên một phiên thật: một lượt gọi
// model và 301 byte ngữ cảnh để chở về chữ "chưa có". Và nó sẽ lặp lại MỌI phiên, vì
// trang ấy luôn có mặt và dòng mô tả của nó hợp với gần như mọi lượt — tức nó chiếm một
// suất trong trần `max_open_per_turn` vĩnh viễn.
//
// Cửa cũ đo sai thứ: nó hỏi "có dòng index không", mà cái đáng hỏi là "mở ra có thêm gì
// so với dòng index không". Không dựng ngưỡng độ dài — một con số như thế không đo được
// cái gì thật. Dùng lại đúng hình mà `headAndDesc` đã dùng: dòng in nghiêng trọn vẹn là
// chú của khung, không phải chữ của người. Tiêu đề THỨ HAI trở đi thì tính là ruột — lúc
// ấy trang đã có dàn ý để mở.
func hasBody(text string) bool {
	head := false
	for _, ln := range strings.Split(text, "\n") {
		ln = strings.TrimSpace(ln)
		switch {
		case ln == "":
		case strings.HasPrefix(ln, "#"):
			if head {
				return true
			}
			head = true
		case strings.Trim(ln, "-*_ ") == "": // đường kẻ
		case len(ln) > 1 && strings.HasPrefix(ln, "*") && strings.HasSuffix(ln, "*"):
		default:
			return true
		}
	}
	return false
}

// headAndDesc lấy dòng `# tiêu đề` và dòng in nghiêng ngay dưới nó — nếp chung của
// mọi trang trong kho.
func headAndDesc(text string) (head, desc string) {
	for _, ln := range strings.Split(text, "\n") {
		ln = strings.TrimSpace(ln)
		switch {
		case head == "" && strings.HasPrefix(ln, "# "):
			head = strings.TrimSpace(ln[2:])
		case head != "" && desc == "" && len(ln) > 4 && strings.HasPrefix(ln, "*") && strings.HasSuffix(ln, "*"):
			desc = strings.TrimSpace(strings.Trim(ln, "*"))
			return head, desc
		}
	}
	return head, desc
}
