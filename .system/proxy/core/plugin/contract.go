// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

// Package plugin định nghĩa cơ chế plugin của lõi: hợp đồng, registry,
// pipeline. Lõi không biết tên plugin nào và không chứa config mặc định
// của plugin nào — plugin tự đăng ký, chỉ được dựng khi cấu hình khai báo.
package plugin

import (
	"context"
	"encoding/json"

	"won/proxy/core/paths"
	"won/proxy/core/request"
	"won/proxy/core/session"
)

// Kind — nhịp đổi của đóng góp, và nhịp quyết chỗ chèn. Hai nhịp nên hai Kind.
type Kind int

const (
	KindSystem Kind = iota + 1 // bản sắc, ký ức — cuối lời hệ thống, đứng cả phiên
	KindMarker                 // một tiếng bên lề — sau lượt người, rồi ghim ở đó
)

// Contribution — một đóng góp; nil là im lặng. Chỉ có chữ: plugin nói, không cầm.
// Tag là tên khối bọc Text, và nó nói đúng NỘI DUNG chứ không nói tên plugin —
// identity chèn soul, nên khối tên Soul. Tag rỗng → chèn trần.
type Contribution struct {
	Tag    string
	Kind   Kind
	Text   string
	Plugin string
}

// Plugin là hợp đồng duy nhất giữa lõi và mọi thứ gắn thêm. Trả (nil, nil) là
// im lặng. Lỗi, panic, quá ngân sách → lõi quy về im lặng (#2).
//
// Trả DANH SÁCH vì hai Kind là hai nhịp, và một plugin có thể có việc ở cả hai: memory
// giữ index đứng cả phiên (KindSystem) mà mở trang theo từng lượt (KindMarker). Gộp hai
// nhịp vào một đóng góp là buộc cái đứng yên đi theo nhịp cái đổi. Phần tử nil bị bỏ,
// nên plugin một nhịp cứ trả một phần tử.
type Plugin interface {
	Name() string
	Contribute(ctx context.Context, snap *request.Snapshot, sess *session.Session) ([]*Contribution, error)
}

// One — đường về cho plugin chỉ nói một nhịp. Nil vào, danh sách rỗng ra: im lặng vẫn
// là im lặng, không phải một đóng góp rỗng.
func One(c *Contribution) []*Contribution {
	if c == nil {
		return nil
	}
	return []*Contribution{c}
}

// Env là mọi thứ lõi trao cho factory. Name do lõi gán (xem Build), plugin không
// tự khai: tên tự đặt không quy được về ai.
type Env struct {
	Name     string
	Paths    paths.Tree
	Services *Hub
	Options  json.RawMessage // khối options thô — plugin tự parse schema của mình
	Control  string          // Control API address (host:port), rỗng = tắt — lõi truyền, plugin không đoán (#6)
}

// Hub — main đặt service dùng chung lúc wiring, plugin lấy ra bằng type assertion.
// Chỉ ghi lúc wiring, sau đó chỉ đọc — nên không cần khoá dù plugin chạy song song.
type Hub struct {
	m map[string]any
}

func NewHub() *Hub { return &Hub{m: map[string]any{}} }

func (h *Hub) Set(name string, v any) { h.m[name] = v }

func (h *Hub) Get(name string) any { return h.m[name] }
