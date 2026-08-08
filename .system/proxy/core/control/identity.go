// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package control

import (
	"context"
	"net/http"
	"strings"

	"won/proxy/core/plugin"
	"won/proxy/core/request"
	"won/proxy/core/soul"
)

// fromMax — trần chữ của lời khai mặt control.
const fromMax = 48

// fromDirect — chưa ai khai. Nói ra là "gọi thẳng", không để trống: một trường vắng đọc như
// nhật ký quên ghi, còn "gọi thẳng" là một câu trả lời thật.
const fromDirect = "(direct)"

// controlFrom — mặt control nào vừa gọi, theo lời NÓ TỰ KHAI. Control không có auth và vành
// đai duy nhất là loopback, nên "lệnh này đến từ đâu" là câu duy nhất nhật ký còn trả lời
// được; lời khai không được tin để cấp quyền, chỉ đủ để lần lại vết.
//
// RỬA trước khi vào nhật ký, và đây là lý do hàm này tồn tại thay vì một lần `Header.Get`:
// chữ ấy là của người ngoài và nó đi thẳng vào một dòng log, nên một dấu xuống dòng trong đó
// là một dòng nhật ký GIẢ do client viết.
func controlFrom(r *http.Request) string {
	raw := strings.TrimSpace(r.Header.Get(request.HeaderControl))
	if raw == "" {
		return fromDirect
	}
	var sb strings.Builder
	for _, c := range raw {
		if sb.Len() >= fromMax {
			break
		}
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9',
			c == '-', c == '_', c == '.', c == '/':
			sb.WriteRune(c)
		default:
			sb.WriteByte('_')
		}
	}
	return sb.String()
}

// withAgent gắn agent đã resolve vào context — route con lấy ra qua AgentCtxKey.
func withAgent(id soul.IdentityResolver, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agent := id.Resolve(r.Header.Get(request.HeaderAgent), "")
		ctx := context.WithValue(r.Context(), plugin.AgentCtxKey{}, agent)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
