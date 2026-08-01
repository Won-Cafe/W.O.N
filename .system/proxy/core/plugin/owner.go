// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package plugin

import (
	"context"
	"log/slog"
	"strings"
	"time"
)

// Quyền làm chủ — plugin tự khai bằng interface. Tên plugin không sống trong lõi.

// SystemOwner — plugin làm chủ lời hệ thống. Có nó thì khung công cụ chủ được dọn: nó
// tự dựng lời hệ thống, để khung cũ nằm lại là hai lời chồng. Không có nó thì lõi KHÔNG
// dọn — cắt lời dặn của công cụ chủ mà không đặt gì vào chỗ đó là lỗ ròng.
//
// Đúng MỘT plugin được nhận quyền này tại một thời điểm — "ai nhận việc này" ngầm định
// số ít. Cơ chế không ép được điều đó (plugin tự khai qua interface, không qua danh sách
// đóng), nên `SystemOwned` chỉ kêu lên khi phạm; nó không tự chọn một người thắng.
type SystemOwner interface {
	OwnsSystem() bool
}

// SystemOwned — có plugin nào đang làm chủ lời hệ thống không. Hơn một plugin cùng
// nhận quyền là "hai chủ chồng nhau": mỗi plugin vẫn tự dựng phần lời hệ thống của
// mình (Apply không phân biệt), nên kết quả là hai bản tự giới thiệu cạnh nhau mà
// không ai báo — kêu ra đây, một lần, để lỗi wiring không nằm im.
func SystemOwned(plugins []Plugin) bool {
	// Gom tên chứ không đếm: người đọc log cần biết tắt cái nào, không cần biết có mấy cái.
	var owners []string
	for _, p := range plugins {
		if o, ok := p.(SystemOwner); ok && o.OwnsSystem() {
			owners = append(owners, p.Name())
		}
	}
	if len(owners) > 1 {
		slog.Warn("plugin: more than one owns the system prompt — each rebuilds it, so they stack",
			"plugins", strings.Join(owners, ","),
			"fix", "keep one of them enabled in won.conf and set the others to enable = false")
	}
	return len(owners) > 0
}

// Budgeted — plugin tự khai trần thời gian của RIÊNG nó (`<tên>.budget_ms`, đọc ở
// `plugins/base`). Lõi chỉ hỏi "có khai không" qua interface, không đọc tên: tên plugin
// không sống trong lõi. Trần này bọc TRỌN lượt Contribute — đồng hồ chạy từ lúc plugin
// bắt đầu, không từ lúc nó hỏi model — nên nó cắt được cả cái treo trong code của chính
// plugin. Trả 0 = không khai, chịu trần của cả lượt.
type Budgeted interface {
	Budget() time.Duration
}

// budgetFor — trần riêng nếu plugin khai; không thì chịu trần của cả lượt (ctx đã bị bó
// theo `total_budget_ms` trước khi vào Gather). Khai lớn hơn phần còn lại của lượt cũng
// không nới được: context lấy hạn nào tới TRƯỚC.
func budgetFor(ctx context.Context, p Plugin) (context.Context, context.CancelFunc) {
	if b, ok := p.(Budgeted); ok {
		if d := b.Budget(); d > 0 {
			return context.WithTimeout(ctx, d)
		}
	}
	return context.WithCancel(ctx)
}

// TurnVoice — plugin chỉ nói ở lượt của NGƯỜI. Khai nó thì lõi KHÔNG GỌI plugin ấy khi
// người chưa nói lại, và không gọi nghĩa là không tốn gì. Cửa ở lõi chứ không ở từng
// plugin vì chỗ tốn kém nằm TRƯỚC chỗ kiểm; mất gì và không mất gì: § Tiếng của lượt.
type TurnVoice interface {
	SpeaksOnHumanTurn() bool
}

// turnVoice — plugin này chỉ nói ở lượt của người không.
func turnVoice(p Plugin) bool {
	v, ok := p.(TurnVoice)
	return ok && v.SpeaksOnHumanTurn()
}
