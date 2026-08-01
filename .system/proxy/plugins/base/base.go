// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

// Package base — helper chung cho plugin: lấy service ra khỏi Hub, parse options,
// hỏi model nền đúng một lần cho mỗi câu. Base không implement Contribute —
// plugin tự viết phần của mình.
package base

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"won/proxy/core/paths"
	"won/proxy/core/plugin"
	"won/proxy/core/session"
	"won/proxy/core/soul"
	"won/proxy/services/localmodel"
)

// reservedBudget — núm thời gian của MỌI plugin, nên nó ở base chứ không chép vào
// schema từng plugin. base nhấc nó ra khỏi options trước khi plugin đọc phần của mình.
const reservedBudget = "budget_ms"

// defaultBudgetMs — trần riêng mặc định của MỘT plugin, khớp `<tên>.budget_ms` trong
// won.conf.example. Nằm trong trần cả lượt (`total_budget_ms`), nên cái nào hết trước
// thì cái đó cắt.
const defaultBudgetMs = 20000

// Base nhúng vào plugin: tên lõi dựng, bố cục cây, chỗ lấy service, trần thời gian
// riêng. opts là options ĐÃ trừ các khoá base giữ — plugin chỉ thấy khoá của nó.
type Base struct {
	Name     string
	Paths    paths.Tree
	Services *plugin.Hub

	budget time.Duration   // 0 = không khai riêng, chịu trần của lõi
	opts   json.RawMessage // phần còn lại, dành cho schema của plugin
}

// New nhấc các khoá base giữ ra khỏi options. Options hỏng không chặn dựng plugin:
// phần còn lại giao nguyên vẹn, và plugin sẽ tự vỡ rõ ở ParseOptions nếu thật sự sai.
func New(env plugin.Env) Base {
	b := Base{Name: env.Name, Paths: env.Paths, Services: env.Services, opts: env.Options,
		budget: defaultBudgetMs * time.Millisecond}
	if len(env.Options) == 0 {
		return b
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal(env.Options, &all); err != nil {
		return b
	}
	if raw, ok := all[reservedBudget]; ok {
		var ms int
		// `budget_ms = 0` là đường tắt trần riêng: chỉ còn trần cả lượt. Mặc định là một
		// lời khai, nên phải có cách nói ngược lại nó.
		if err := json.Unmarshal(raw, &ms); err == nil {
			b.budget = time.Duration(max(ms, 0)) * time.Millisecond
		}
		delete(all, reservedBudget)
		if rest, err := json.Marshal(all); err == nil {
			b.opts = rest
		}
	}
	return b
}

// Chatter — chỉ phần plugin cần ở model nền. Interface để test không gọi mạng.
type Chatter interface {
	Chat(ctx context.Context, system, user string) (string, error)
}

// Budget — trần riêng plugin khai qua `<tên>.budget_ms`; 0 = không khai. Lõi đọc nó qua
// interface `plugin.Budgeted` và bọc TRỌN lượt Contribute, nên đồng hồ chạy từ lúc plugin
// bắt đầu chứ không từ lúc nó hỏi model.
func (b Base) Budget() time.Duration { return b.budget }

// Ask hỏi model nền — mỗi câu hỏi khác nhau chỉ hỏi MỘT LẦN trong phiên: cùng câu hỏi
// thì lời đáp không thể mới. Vân tay lấy trên lời hỏi, không trên soul — soul là ai
// trả lời, lời hỏi mới là cái được hỏi.
// Không mở đồng hồ ở đây: trần của plugin đã bọc trọn lượt Contribute ở lõi, nên một
// đồng hồ thứ hai tại đây chỉ có thể muộn hơn — hai chỗ đặt hạn cho một việc là hai
// chỗ lệch được.
func (b Base) Ask(ctx context.Context, sess *session.Session, llm Chatter, system, user string) (string, error) {
	if sess != nil && sess.Asked(b.Name, user) {
		slog.Debug("plugin: same question — not asked again", "plugin", b.Name)
		return "", nil
	}
	return llm.Chat(ctx, system, user)
}

// ParseOptions unmarshal phần options CỦA PLUGIN vào struct — các khoá base giữ đã
// bị nhấc ra ở New. Khoá lạ là lỗi, không bỏ qua: nuốt im lặng thì người khai tưởng
// núm đã xoay, mà plugin chạy bằng mặc định.
func (b Base) ParseOptions(out any) error {
	if len(b.opts) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(b.opts))
	dec.DisallowUnknownFields()
	return dec.Decode(out)
}

// Book / LLM — nil-safe: chưa wiring thì nil, và plugin dựa vào nó tự im. Chỗ tra Hub
// ở phía DÙNG, không phía cấp: service không cần biết cơ chế plugin để được dùng.
func (b Base) Book() *soul.Book {
	v, _ := b.Services.Get(soul.ServiceName).(*soul.Book)
	return v
}

func (b Base) LLM() *localmodel.Client {
	v, _ := b.Services.Get(localmodel.ServiceName).(*localmodel.Client)
	return v
}
