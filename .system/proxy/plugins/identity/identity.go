// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

// Plugin identity giữ bản sắc: chèn soul file + luật kênh nếu lời hệ thống chưa
// mang. Nhận làm chủ lời hệ thống (SystemOwner) — tắt nó thì lõi không chạm khung IDE.
package identity

import (
	"context"
	"fmt"
	"strings"

	"won/proxy/core/plugin"
	"won/proxy/core/request"
	"won/proxy/core/session"
	"won/proxy/core/soul"
	"won/proxy/plugins/base"
)

func init() { plugin.Register("identity", New) }

type Identity struct {
	base.Base
	book *soul.Book
}

func New(env plugin.Env) (plugin.Plugin, error) {
	b := base.New(env)
	book := b.Book()
	if book == nil {
		return nil, fmt.Errorf("identity needs the soul book")
	}
	return &Identity{Base: b, book: book}, nil
}

func (p *Identity) Name() string { return "identity" }

// OwnsSystem — lời hệ thống là việc của tôi. Xem plugin.SystemOwner.
func (p *Identity) OwnsSystem() bool { return true }

func (p *Identity) Contribute(ctx context.Context, snap *request.Snapshot, sess *session.Session) ([]*plugin.Contribution, error) {
	if snap.Agent == "" {
		return nil, nil // không đoán căn cước (#6)
	}
	soulText := p.book.Soul(snap.Agent)
	if soulText == "" {
		return nil, nil
	}
	if t := p.book.Title(snap.Agent); t != "" && strings.Contains(snap.System, t) {
		return nil, nil // bản sắc đã có mặt — không chèn trùng
	}
	// Soul là bản sắc; luật kênh là thứ mọi đệ phải cùng biết. Rót cả hai: đệ không
	// phải thuộc lòng hệ, hệ tự tới.
	return plugin.One(&plugin.Contribution{
		Kind: plugin.KindSystem,
		Tag:  "Soul",
		Text: soulText + "\n\n" + wiring(snap.Agent),
	}), nil
}
