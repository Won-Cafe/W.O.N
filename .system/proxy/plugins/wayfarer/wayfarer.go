// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

// Plugin wayfarer — người lữ hành. Cơ học là **cái cân, không phải quan toà**:
// bày trọn số ra, soul + model quyết số nào đáng thành mốc. Không một ngưỡng nào
// trong code — "dài", "lâu", "bất thường" cần một cái nhìn, và hằng số không nhìn.
// Nó cũng không tự chọn hộ dữ kiện nặng nhất: Discipline của soul đã viết sẵn luật
// chọn, và luật chọn phải nằm cùng chỗ với cái nhìn.
package wayfarer

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"won/proxy/core/paths"
	"won/proxy/core/plugin"
	"won/proxy/core/request"
	"won/proxy/core/session"
	"won/proxy/core/soul"
	"won/proxy/plugins/base"
	"won/proxy/services/localmodel"
)

func init() { plugin.Register("wayfarer", New) }

const (
	marker           = "🛣️"
	soulName         = "Wayfarer"
	thresholdExcerpt = 1200
)

type Wayfarer struct {
	base.Base
	llm  *localmodel.Client
	book *soul.Book
}

func New(env plugin.Env) (plugin.Plugin, error) {
	b := base.New(env)
	book := b.Book()
	if book == nil {
		return nil, fmt.Errorf("wayfarer needs the soul book")
	}
	if !b.Paths.Known() {
		return nil, fmt.Errorf("wayfarer needs the W.O.N root path")
	}
	return &Wayfarer{Base: b, llm: b.LLM(), book: book}, nil
}

func (p *Wayfarer) Name() string { return "wayfarer" }

// SpeaksOnHumanTurn — quãng đường đo bằng bước của NGƯỜI, không bằng vòng của máy
// (plugin.TurnVoice).
func (p *Wayfarer) SpeaksOnHumanTurn() bool { return true }

func (p *Wayfarer) Contribute(ctx context.Context, snap *request.Snapshot, sess *session.Session) ([]*plugin.Contribution, error) {
	soulText := p.book.Soul(soulName)
	if p.llm == nil || soulText == "" {
		return nil, nil
	}
	// Không rót bản đồ hệ: mục "Giao việc thế nào" trong House dạy gọi subagent kèm tên
	// tool, và người cắm mốc đọc nó rồi nhả ra một lời gọi tool giả thay vì một cái mốc
	// (§ loiterer.go, cùng phép đo).
	system := soulText + "\n\n" +
		base.Contract(
			"the road is level; nothing measured is worth a milestone.",
			// Vế duy nhất ở lại: nó nối cái soul thấy ("một con số đo được") với TÊN khối
			// thật chở con số ấy — chỗ soul cố ý không nói, vì đổi tên một khối thì soul nói
			// sai mà không ai báo.
			"Every milestone stands on a number that appears in this turn's `<Road>` or a source "+
				"named in `<Threshold>`. Never invent a number, an earlier turn, or a name the "+
				"conversation does not name.")
	user := localmodel.RenderUser(snap, "Recipient", snap.Agent,
		localmodel.Block("Road", p.road(snap, sess)),
		localmodel.Block("Threshold", p.readThreshold()))

	out, err := p.Ask(ctx, sess, p.llm, system, user)
	if err != nil {
		return nil, err
	}
	// Hai dạng của cùng một lời: sổ giữ chữ, request giữ chữ có dấu (§ Marker do lõi đeo).
	content := base.Line(marker, soulName, out)
	if content == "" {
		return nil, nil
	}
	sess.Say(soulName, content) // để lượt sau soul thấy mình đã cắm gì mà không nhắc lại
	return plugin.One(&plugin.Contribution{Kind: plugin.KindMarker, Tag: soulName,
		Text: base.Stamp(marker, soulName, content)}), nil
}

// road bày trọn số đo được. Chỉ số và nguồn — không chữ nào nói nặng hay nhẹ.
func (p *Wayfarer) road(snap *request.Snapshot, sess *session.Session) string {
	now := snap.ReceivedAt
	var sb strings.Builder

	if gap := sess.GapAtStart(); gap > 0 {
		fmt.Fprintf(&sb, "Vắng trước phiên này: %s.\n", dur(gap))
	} else {
		sb.WriteString("Vắng trước phiên này: không đo được (chưa có dấu của lần trước).\n")
	}
	fmt.Fprintf(&sb, "Phiên này: lượt người thứ %d, đệ đã chạy %d lần máy; mở %s trước, lượt gần nhất cách đây %s.\n",
		sess.Turns(), sess.Runs(), dur(now.Sub(sess.FirstSeen())), dur(sess.IdleFor(now)))

	if past := sess.PastTurns(); len(past) > 0 {
		s := make([]string, len(past))
		for i, n := range past {
			s[i] = fmt.Sprint(n)
		}
		fmt.Fprintf(&sb, "Các phiên trước của đệ này, số lượt NGƯỜI (cũ → mới): %s.\n", strings.Join(s, ", "))
	} else {
		sb.WriteString("Các phiên trước của đệ này: chưa có dấu nào.\n")
	}

	for _, f := range p.openFocus(now) {
		sb.WriteString(f + "\n")
	}
	if m := p.lastWritten(now); m != "" {
		sb.WriteString(m + "\n")
	}

	said := sess.Said(soulName)
	if len(said) == 0 {
		sb.WriteString("Mốc đã cắm trong phiên này: chưa có.\n")
	} else {
		sb.WriteString("Mốc đã cắm trong phiên này (đừng cắm lại):\n")
		for _, s := range said {
			sb.WriteString("- " + s + "\n")
		}
	}
	return sb.String()
}

// openFocus — việc đang mở, tuổi tính từ NGÀY TRONG TÊN trang: đó là ngày việc
// được mở. mtime chỉ nói lần cuối trang bị sửa — trang đang được làm thì mtime
// luôn mới, nên lấy mtime làm tuổi việc là bắt đúng cái *bị bỏ*, không phải cái
// *đang kẹt*.
func (p *Wayfarer) openFocus(now time.Time) []string {
	entries, err := os.ReadDir(p.Paths.Zone(paths.ZoneWorking))
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && paths.IsPage(e.Name()) {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	out := make([]string, 0, len(names))
	for _, n := range names {
		line := "working/ đang mở: " + n
		if d, ok := dateInName(n); ok {
			line += fmt.Sprintf(" — mở ngày %s, %s trước", d.Format("02/01/2006"), dur(now.Sub(d)))
		}
		out = append(out, line+".")
	}
	return out
}

// lastWritten — kho ký ức được ghi lần cuối khi nào. Đây là chỗ mtime nói đúng:
// "lần cuối có người đặt bút".
func (p *Wayfarer) lastWritten(now time.Time) string {
	var newest time.Time
	var where string
	for _, zone := range paths.Zones {
		entries, err := os.ReadDir(p.Paths.Zone(zone))
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !paths.IsPage(e.Name()) {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			if info.ModTime().After(newest) {
				newest, where = info.ModTime(), zone+"/"+e.Name()
			}
		}
	}
	if where == "" {
		return ""
	}
	return fmt.Sprintf("Kho ký ức: ghi lần cuối %s trước (%s).", dur(now.Sub(newest)), where)
}

// dateInName đọc ngày mở đầu tên trang (`2026-07-19-…`). Sống sót qua mọi kiểu
// chép, khác với dấu thời gian hệ tệp.
func dateInName(name string) (time.Time, bool) {
	if len(name) < 10 {
		return time.Time{}, false
	}
	d, err := time.Parse("2006-01-02", name[:10])
	return d, err == nil
}

// dur viết khoảng thời gian bằng đơn vị thô nhất — con số để người đọc tự cân.
func dur(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%d giây", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%d phút", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d giờ %d phút", int(d.Hours()), int(d.Minutes())%60)
	default:
		return fmt.Sprintf("%.0f ngày", d.Hours()/24)
	}
}

func (p *Wayfarer) readThreshold() string {
	b, err := os.ReadFile(p.Paths.Threshold())
	if err != nil {
		return ""
	}
	s := strings.TrimSpace(string(b))
	if s == "" {
		return ""
	}
	return request.Truncate(s, thresholdExcerpt)
}
