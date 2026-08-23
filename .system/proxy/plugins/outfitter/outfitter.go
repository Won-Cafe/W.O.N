// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

// Plugin outfitter — người trang bị. NÓI về đồ nghề, không cầm: đọc danh mục tool
// cùng dấu tay vừa để lại, rồi trả một dòng về việc dùng đồ. Danh mục không bị chạm.
//
// Ranh giữa hai tầng: plugin bày ra *món gì, đích nào, nhãn nào* và khai đúng hình
// dạng đầu ra; soul đọc ra dấu và cân xem dấu nào đáng nói. Không tầng nào chép lời
// của tầng kia — một luật chép hai chỗ là hai bản có thể lệch.
//
// Vai chính danh khai ở README § Ba kẻ đứng bờ: "nói một dòng về một món đang nằm sai
// chỗ… chỉ vào món và vào chỗ món ĐÃ ĐI, không chỉ vào người". Ba mệnh đề ấy quyết định
// được bằng chuỗi, nên chúng có cửa gác bằng code ở gate.go, không nằm nhờ vào việc model
// 4B có chịu tuân lời nhắc hay không.
package outfitter

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"won/proxy/core/plugin"
	"won/proxy/core/request"
	"won/proxy/core/session"
	"won/proxy/core/soul"
	"won/proxy/plugins/base"
	"won/proxy/services/localmodel"
)

func init() { plugin.Register("outfitter", New) }

const (
	marker   = "🧰"
	soulName = "Outfitter"

	purposeCap     = 120 // mục đích một món, cắt còn đủ nhận ra nó làm gì
	catalogCap     = 40  // số món đưa vào tầm mắt. Dài hơn thì đuôi bị bỏ, phải khai ra
	defaultMinRuns = 6   // "nhìn vài lượt rồi mới phán" — đo bằng lần chạy
)

// options — min_runs thi hành "nhìn vài lượt rồi mới phán", một thước cho mọi nhịp.
type options struct {
	MinRuns int `json:"min_runs"`
}

type Outfitter struct {
	base.Base
	llm     *localmodel.Client
	book    *soul.Book
	minRuns int
}

func New(env plugin.Env) (plugin.Plugin, error) {
	b := base.New(env)
	book := b.Book()
	if book == nil {
		return nil, fmt.Errorf("outfitter needs the soul book")
	}

	var o options
	if err := b.ParseOptions(&o); err != nil {
		return nil, fmt.Errorf("outfitter options: %w", err)
	}
	if o.MinRuns < 1 {
		o.MinRuns = defaultMinRuns
	}
	return &Outfitter{Base: b, llm: b.LLM(), book: book, minRuns: o.MinRuns}, nil
}

func (p *Outfitter) Name() string { return "outfitter" }

// SpeaksOnHumanTurn — nói ở lượt của người, không giữa vòng tool (plugin.TurnVoice).
// Không mất dữ kiện: `Snapshot.Reached` rút từ LỊCH SỬ trong request, và request nào
// cũng mang trọn lịch sử — nên vòng tool vừa xong vẫn nằm đó, đọc được ở lượt sau.
// Đổi lấy: nó nói khi có người đọc, thay vì nói vào giữa một vòng máy.
func (p *Outfitter) SpeaksOnHumanTurn() bool { return true }

func (p *Outfitter) Contribute(ctx context.Context, snap *request.Snapshot, sess *session.Session) ([]*plugin.Contribution, error) {
	// Ba cửa đóng trước khi hỏi model — cửa rẻ nhất trước. Còn cửa sau khi model nói
	// nằm ở keep(): cửa trước tiết model, cửa sau giữ vai.
	soulText := p.book.Soul(soulName)
	if p.llm == nil || soulText == "" {
		return nil, nil
	}
	// Ngưỡng đọc LẦN CHẠY, không đọc lượt người. Vật liệu của kẻ này là `<Kit>` và
	// `<Reached>` — cả hai lớn theo lần chạy, không lớn theo số câu người nói. Đo trên
	// nhật ký thật: ở lượt người thứ 3, runs tích luỹ có phiên chỉ 0, tức ngưỡng đọc lượt
	// người gọi model 4B để nói về đồ nghề khi chưa món nào được dùng. Thước đo đúng vật
	// liệu thì dùng được cho mọi nhịp, nên không cần nhận ra mình đang ở nhịp nào.
	if sess.Runs() < p.minRuns {
		return nil, nil // một cái liếc thì chưa nói gì
	}
	kit, dropped := renderKit(snap.Tools)
	if kit == "" {
		return nil, nil // không thấy đồ nghề nào — không có gì để nói
	}
	if dropped > 0 {
		kit += fmt.Sprintf("(+%d món nữa không nêu — danh mục dài hơn tầm mắt)\n", dropped)
	}

	// Lời hệ thống của đệ nền: bản sắc → bản đồ hệ → bản kê cơ học → hợp đồng đầu ra.
	// Hai khối sau là của plugin: nó biết mình gửi gì đi, soul thì không.
	// Không rót bản đồ hệ: House là bản đồ Circle, mang roster tên và cách gọi subagent —
	// người giữ kho không thuộc Circle và không gọi ai. Đo được: có House thì nó gọi ra
	// tên đệ không có trong đối thoại (§ loiterer.go, cùng phép đo).
	system := soulText + "\n\n" + legend() + "\n" +
		base.Contract(
			"silence; the wearer picks their own tools this turn.",
			"Name only tools that appear in `<Kit>`; never invent one.",
			// HÌNH của một ô trong dòng, thứ soul không nói được: soul nói đích phải là chỗ
			// món đã đi, chỗ này nói ô sau mũi tên chứa gì. Cùng luật ấy có cửa code gác
			// (§ gate.go), nên hợp đồng không cần nhắc thêm rằng không trỏ được thì im.
			"Whatever you write after `→` must be copied from a `<Reached>` line of THIS turn.")

	// "Wearer" là chữ của nghề — soul nhìn *người mang*. Outfitter KHÔNG nghỉ giữa
	// vòng tool: đúng lúc đó tay người mang đang chạy nhiều lượt liền, và món nằm sai
	// chỗ chỉ lộ ra khi nhìn cái tay vừa với gì.
	said := sess.Said(soulName)
	user := localmodel.RenderUser(snap, "Wearer", snap.Agent,
		localmodel.Block("Kit", kit),
		localmodel.Block("Reached", p.reached(snap, sess)),
		localmodel.Block("Said", saidBlock(said)))

	out, err := p.Ask(ctx, sess, p.llm, system, user)
	if err != nil {
		return nil, err
	}
	// Hai dạng của cùng một lời: sổ giữ chữ, request giữ chữ có dấu (§ Marker do lõi đeo).
	content := base.Line(marker, soulName, out)
	if content == "" {
		return nil, nil
	}
	if why := keep(content, snap.Tools, snap.Reached, p.book.Names(), said); why != "" {
		slog.Debug("outfitter: dòng trượt cửa", "cửa", why)
		return nil, nil
	}
	sess.Say(soulName, content) // để lượt sau không nói lại — "nói một lần rồi thôi"
	return plugin.One(&plugin.Contribution{Kind: plugin.KindMarker, Tag: soulName,
		Text: base.Stamp(marker, soulName, content)}), nil
}

// saidBlock bày sổ đã nói. Chưa nói gì thì khai đúng thế: để trống một khối là để một
// chỗ model tự điền.
func saidBlock(said []string) string {
	if len(said) == 0 {
		return "Chưa nói gì trong phiên này.\n"
	}
	var sb strings.Builder
	sb.WriteString("Đã nói trong phiên này — đừng nói lại:\n")
	for _, s := range said {
		sb.WriteString("- " + s + "\n")
	}
	return sb.String()
}
