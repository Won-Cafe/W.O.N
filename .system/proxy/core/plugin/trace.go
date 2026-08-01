// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package plugin

import (
	"context"
	"sync"
)

// Call — một lượt gọi model nền, ghi lại để chẩn bệnh. Đây là chỗ phân biệt
// im-vì-soul với im-vì-lạc-khuôn. ThinkingLen + DoneReason đọc ra ca im đặc thù:
// khối suy nghĩ ăn trọn trần chữ, output rỗng, done_reason="length".
type Call struct {
	Model       string `json:"model"`
	Ms          int64  `json:"ms"`
	System      string `json:"system"`
	User        string `json:"user"`
	Output      string `json:"output"`
	Thinking    string `json:"thinking,omitempty"`
	ThinkingLen int    `json:"thinking_len,omitempty"`
	DoneReason  string `json:"done_reason,omitempty"`
	Err         string `json:"err,omitempty"`
}

// Trace gom các lượt gọi của một plugin trong một request. Plugin chạy song song
// nên mỗi plugin một Trace riêng.
type Trace struct {
	mu    sync.Mutex
	calls []Call
}

// Record nil-safe: Trace nil thì no-op.
func (t *Trace) Record(c Call) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.calls = append(t.calls, c)
}

func (t *Trace) Calls() []Call {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]Call(nil), t.calls...)
}

type tracingKey struct{}
type traceKey struct{}

// WithTracing bật ghi vết cho request này. Tắt thì không prompt nào bị giữ.
func WithTracing(ctx context.Context) context.Context {
	return context.WithValue(ctx, tracingKey{}, true)
}

func tracing(ctx context.Context) bool {
	on, _ := ctx.Value(tracingKey{}).(bool)
	return on
}

func withTrace(ctx context.Context, t *Trace) context.Context {
	return context.WithValue(ctx, traceKey{}, t)
}

// TraceFrom — service gọi model nền lấy chỗ ghi vết. Nil nghĩa là không ghi.
func TraceFrom(ctx context.Context) *Trace {
	t, _ := ctx.Value(traceKey{}).(*Trace)
	return t
}
