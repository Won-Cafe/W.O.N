// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

// Plugin outfitter — người trang bị. NÓI về đồ nghề, không cầm: đọc danh mục tool
// cùng dấu tay vừa để lại, rồi trả một dòng về việc dùng đồ. Danh mục không bị chạm.
//
// Ranh giữa hai tầng: plugin bày ra *món gì, đích nào, vùng nào* và khai đúng hình
// dạng đầu ra; soul đọc ra dấu và cân xem dấu nào đáng nói. Không tầng nào chép lời
// của tầng kia — một luật chép hai chỗ là hai bản có thể lệch.
package outfitter

import (
	"context"
	"fmt"

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

	purposeCap      = 120 // mục đích một món, cắt còn đủ nhận ra nó làm gì
	catalogCap      = 40  // số món đưa vào tầm mắt. Dài hơn thì đuôi bị bỏ, phải khai ra
	defaultMinTurns = 3   // "nhìn vài lượt rồi mới phán" — mặc định khi không khai
)

// options — min_turns thi hành "nhìn vài lượt rồi mới phán".
type options struct {
	MinTurns int `json:"min_turns"`
}

type Outfitter struct {
	base.Base
	llm      *localmodel.Client
	book     *soul.Book
	minTurns int
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
	if o.MinTurns < 1 {
		o.MinTurns = defaultMinTurns
	}
	return &Outfitter{Base: b, llm: b.LLM(), book: book, minTurns: o.MinTurns}, nil
}

func (p *Outfitter) Name() string { return "outfitter" }

// SpeaksOnHumanTurn — nói ở lượt của người, không giữa vòng tool (plugin.TurnVoice).
// Không mất dữ kiện: `Snapshot.Reached` rút từ LỊCH SỬ trong request, và request nào
// cũng mang trọn lịch sử — nên vòng tool vừa xong vẫn nằm đó, đọc được ở lượt sau.
// Đổi lấy: nó nói khi có người đọc, thay vì nói vào giữa một vòng máy.
func (p *Outfitter) SpeaksOnHumanTurn() bool { return true }

func (p *Outfitter) Contribute(ctx context.Context, snap *request.Snapshot, sess *session.Session) ([]*plugin.Contribution, error) {
	// Ba cửa đóng trước khi hỏi model — cửa rẻ nhất trước.
	soulText := p.book.Soul(soulName)
	if p.llm == nil || soulText == "" {
		return nil, nil
	}
	if sess.Turns() < p.minTurns {
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
		base.Contract(soulName, marker,
			"silence; the wearer picks their own tools this turn.",
			"the line you are writing is about the work itself, not about the kit",
			"Name only tools that appear in `<Kit>`; never invent one.",
			"Point at a tool and where it went — never at the person.",
			"Your line must point at something that actually appears in this turn's `<Kit>` or "+
				"`<Reached>`. Never invent an earlier turn, and never name anyone the conversation "+
				"does not name.",
			// Tả HÌNH, không trích câu trọn vẹn: một câu mẫu đầy đủ trong prompt thì model
			// nhỏ chép nguyên văn nó (template.agent.md § đệ nền, nếp 1).
			"Shapes of a line — these are shapes, not sentences to reuse; copying a whole "+
				"sample line verbatim is worse than silence: a tool that would end the groping "+
				"→ name it and name what the hand has been doing instead · a tool doing more "+
				"than its share → name the tool and where it went · one tool right and another "+
				"premature → say which waits, and until when.")

	// "Wearer" là chữ của nghề — soul nhìn *người mang*. Outfitter KHÔNG nghỉ giữa
	// vòng tool: đúng lúc đó tay người mang đang chạy nhiều lượt liền, và món nằm sai
	// chỗ chỉ lộ ra khi nhìn cái tay vừa với gì.
	user := localmodel.RenderUser(snap, "Wearer", snap.Agent,
		localmodel.Block("Kit", kit),
		localmodel.Block("Reached", p.reached(snap, sess)))

	out, err := p.Ask(ctx, sess, p.llm, system, user)
	if err != nil {
		return nil, err
	}
	line := base.Say(marker, soulName, out)
	if line == "" {
		return nil, nil
	}
	return plugin.One(&plugin.Contribution{Kind: plugin.KindMarker, Tag: soulName, Text: line}), nil
}
