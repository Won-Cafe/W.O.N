// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

// Plugin loiterer — người tạt ngang. Cơ học: đưa bản sao request + bản đồ hệ +
// khuôn mặt cho model local mang soul Loiterer, trích một dòng marker. Nói hay im
// là việc của soul, không của code.
package loiterer

import (
	"context"
	"fmt"
	"strings"

	"won/proxy/core/plugin"
	"won/proxy/core/request"
	"won/proxy/core/session"
	"won/proxy/core/soul"
	"won/proxy/plugins/base"
	"won/proxy/services/localmodel"
)

func init() { plugin.Register("loiterer", New) }

const (
	marker   = "🚶"
	soulName = "Loiterer"
)

// options — schema thuộc về plugin, không thuộc lõi.
type options struct {
	Faces []string `json:"faces"` // khuôn mặt do tầng cơ học gán xoay vòng
}

// defaultFaces — khớp `loiterer.faces` trong won.conf.example. Chưa khai thì lấy danh
// sách này; khai `off` thì ghé vô danh.
var defaultFaces = []string{"anh xe ôm", "cô bán nước", "chú bảo vệ"}

type Loiterer struct {
	base.Base
	llm   *localmodel.Client
	book  *soul.Book
	faces []string
}

func New(env plugin.Env) (plugin.Plugin, error) {
	b := base.New(env)
	book := b.Book()
	if book == nil {
		return nil, fmt.Errorf("loiterer needs the soul book")
	}
	var o options
	if err := b.ParseOptions(&o); err != nil {
		return nil, fmt.Errorf("loiterer options: %w", err)
	}
	faces := o.Faces
	if len(faces) == 0 {
		faces = defaultFaces
	} else if len(faces) == 1 && strings.EqualFold(strings.TrimSpace(faces[0]), "off") {
		faces = nil
	}
	return &Loiterer{Base: b, llm: b.LLM(), book: book, faces: faces}, nil
}

func (p *Loiterer) Name() string { return "loiterer" }

// SpeaksOnHumanTurn — một góc nhìn tạt ngang là để người đọc. Giữa vòng tool thì
// không có ai ở đó nghe (plugin.TurnVoice).
func (p *Loiterer) SpeaksOnHumanTurn() bool { return true }

func (p *Loiterer) Contribute(ctx context.Context, snap *request.Snapshot, sess *session.Session) ([]*plugin.Contribution, error) {
	soulText := p.book.Soul(soulName)
	if p.llm == nil || soulText == "" {
		return nil, nil
	}
	// Khuôn mặt do tầng cơ học gán xoay vòng, và tầng cơ học cũng ĐEO nó: nó nằm trong
	// cái tên Say dán vào đầu dòng, không phải một khuôn nữa để model tự dựng.
	// faces rỗng = ghé vô danh.
	name := soulName
	var extra []string
	if len(p.faces) > 0 {
		face := p.faces[sess.Runs()%len(p.faces)]
		name = soulName + " (" + face + ")"
		extra = append(extra, "Today you drop by wearing this face: "+face+
			". Let it colour how you speak; do not announce it.")
	}

	// Cửa chốt cấm một LỐI NÓI, không cấm vật liệu: hội thoại là vật liệu duy nhất
	// Loiterer có, nên một cửa chạm tới nó là cửa đóng cả nghề.
	//
	// Khuôn của lời rót ở ĐÂY, không ở soul: soul lẫn đặc tả định dạng thì model nhỏ đọc
	// nó như cái form phải điền thay vì một người phải thành (template.agent.md § đệ nền).
	// Tả HÌNH chứ không trích câu hoàn chỉnh — đo được: một câu mẫu trọn vẹn trong prompt
	// thì model 9B chép nguyên văn nó, kể cả vào lượt câu ấy vô nghĩa.
	// KHÔNG rót bản đồ hệ vào đây. House là bản đồ của Circle: nó mang roster tên trong
	// backtick và trọn mục dạy gọi subagent kèm tên tool — hai thứ người bên lề không
	// thuộc và không dùng được, mà model nhỏ thì bắt chước cái nó thấy. Đo trên 54 lượt:
	// có House 8 lượt hỏng, bỏ House còn 3, và mọi lần gọi ra một cái tên lạ đều ở nhánh
	// có House. Bớt luôn 2.5KB mỗi lượt.
	system := soulText + "\n\n" +
		base.Contract(soulName, marker,
			"silence; the conversation goes on untouched.",
			"the line you are writing answers the user or tells the recipient what to do — "+
				"you are passing by, not taking the job",
			append([]string{
				"Your line must point at words that actually appear in THIS conversation. " +
					"Never invent an earlier turn, and never name anyone the conversation does not name.",
				"Shapes of a line — these are shapes, not sentences to reuse; copying a whole " +
					"sample line verbatim is worse than silence: the same question asked across " +
					"several turns → point at the loop itself, in this turn's words · a word " +
					"nobody defined while both sides already act on it → name that word · " +
					"deferred items stacking up → count them out · a habit forming that nobody " +
					"has named → name it, without judging it.",
			}, extra...)...)
	user := localmodel.RenderUser(snap, "Recipient", snap.Agent)

	out, err := p.Ask(ctx, sess, p.llm, system, user)
	if err != nil {
		return nil, err
	}
	line := base.Say(marker, name, out)
	if line == "" {
		return nil, nil
	}
	return plugin.One(&plugin.Contribution{Kind: plugin.KindMarker, Tag: soulName, Text: line}), nil
}
