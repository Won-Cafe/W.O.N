// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package soul

// IdentityResolver — lõi chỉ cần: request này của đệ nào. Không nhận ra → rỗng (#6).
// Book.Resolve đã thỏa interface — không cần khai lại.
type IdentityResolver interface {
	Resolve(headerAgent, systemText string) string
}
