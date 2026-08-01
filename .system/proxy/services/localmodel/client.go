// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

// Package localmodel — client cho model nhỏ chạy local (API dạng Ollama) cùng
// bộ đồ nghề nói chuyện với nó: dựng prompt từ Snapshot, trích dòng marker.
package localmodel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"won/proxy/core/plugin"
	"won/proxy/core/request"
)

const ServiceName = "localmodel"

const (
	defaultTimeout = 2500 * time.Millisecond

	// Trần chữ khi ghi vết vào nhật ký — đủ thấy model đang nói kiểu gì.
	respLogCap     = 200
	thinkingLogCap = 600
)

// Config — cái client cần biết. Nhận tham số trần chứ không nhận struct của loader
// cấu hình: service không phải biết cấu hình đến từ đâu.
type Config struct {
	Model       string
	BaseURL     string
	KeepAlive   string   // "" = không gửi field keep_alive
	Think       bool     // núm boolean, luôn gửi
	Temperature *float64 // nil = không gửi field temperature
	TimeoutMs   int
	MaxTokens   int
}

// Client nil-safe: chưa cấu hình thì New trả nil, mọi plugin dựa vào nó tự im.
type Client struct {
	model       string
	baseURL     string
	keepAlive   string
	think       bool
	temperature *float64     // nil = không gửi field
	http        *http.Client // trần một lượt gọi; ctx của plugin là trần lượt chèn
	maxTokens   int
}

func New(cfg Config) *Client {
	if cfg.BaseURL == "" || cfg.Model == "" {
		return nil
	}
	timeout := time.Duration(cfg.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return &Client{
		baseURL:     strings.TrimRight(cfg.BaseURL, "/"),
		model:       cfg.Model,
		maxTokens:   cfg.MaxTokens,
		keepAlive:   cfg.KeepAlive,
		think:       cfg.Think,
		temperature: cfg.Temperature,
		http:        &http.Client{Timeout: timeout},
	}
}

// warmTimeout — trần riêng cho lượt nạp. Rộng hơn hẳn `timeout_ms` vì đây không phải
// một lượt nói: nạp model nguội từ đĩa mất tới hàng chục giây, và trần của lượt nói áp
// vào đây thì cắt ngang đúng lúc nó đang làm việc.
const warmTimeout = 5 * time.Minute

// Warm nạp trọng số model vào máy mà KHÔNG sinh chữ: `/api/generate` với prompt rỗng
// chỉ nạp rồi trả. Vì sao có: lượt gọi đầu tiên của một máy nguội phải chờ nạp, và cái
// chờ ấy nằm trên đường đi của đệ — gọi ở lúc khởi động là dời nó ra khỏi đó.
//
// Nó KHÔNG thay `keep_alive`: keep_alive là cái giữ model nằm lại, đây chỉ đặt điểm bắt
// đầu cho nó. Sau lần này, mọi lượt gọi thật tự gia hạn.
func (c *Client) Warm(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("local model not configured")
	}
	body := map[string]any{"model": c.model, "prompt": "", "stream": false}
	if c.keepAlive != "" {
		body["keep_alive"] = c.keepAlive
	}
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, warmTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/generate", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	// Client riêng: `c.http` mang trần của một lượt nói, quá chặt cho một lượt nạp.
	resp, err := (&http.Client{Timeout: warmTimeout}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("local model returned %s", resp.Status)
	}
	slog.Info("localmodel: warmed", "model", c.model,
		"took", time.Since(start).Milliseconds(), "keep_alive", orUnset(c.keepAlive))
	return nil
}

func orUnset(s string) string {
	if s == "" {
		return "(unset — Ollama unloads after ~5 min idle)"
	}
	return s
}

// Chat gửi prompt tới model nền, trả nguyên văn lời đáp. Ghi vết vào Trace nếu chẩn
// bệnh mở — chỗ duy nhất phân biệt được hai cái im.
func (c *Client) Chat(ctx context.Context, system, user string) (out string, err error) {
	if c == nil {
		return "", fmt.Errorf("local model not configured")
	}
	start := time.Now()
	out, reply, err := c.chat(ctx, system, user)
	if tr := plugin.TraceFrom(ctx); tr != nil {
		call := plugin.Call{
			Model: c.model, Ms: time.Since(start).Milliseconds(),
			System: system, User: user, Output: out,
			Thinking:    request.Truncate(reply.Message.Thinking, thinkingLogCap),
			ThinkingLen: len(reply.Message.Thinking), DoneReason: reply.DoneReason,
		}
		if err != nil {
			call.Err = err.Error()
		}
		tr.Record(call)
	}
	// Khối suy nghĩ ăn trọn trần chữ: model nghĩ xong thì hết chỗ để nói. Đây là ca
	// im lặng có thuốc, nên phải kêu lên.
	if out == "" && reply.Message.Thinking != "" {
		slog.Warn("localmodel: budget spent on thinking, nothing left to say",
			"model", c.model, "thinking_len", len(reply.Message.Thinking),
			"done_reason", reply.DoneReason, "max_tokens", c.maxTokens,
			"fix", "set think = false, or raise max_tokens")
	}
	return out, err
}

// chatReply — phần lời đáp Ollama trả về mà lõi đọc.
type chatReply struct {
	Message struct {
		Content  string `json:"content"`
		Thinking string `json:"thinking"`
	} `json:"message"`
	DoneReason string `json:"done_reason"`
}

func (c *Client) chat(ctx context.Context, system, user string) (string, chatReply, error) {
	var reply chatReply
	// Núm nào không khai thì VẮNG khỏi thân, không đi bằng số 0: với Ollama `temperature: 0`
	// là một lời khai (bốc nhánh đỉnh), còn vắng field là nhường cho mặc định của nó. Hai
	// nghĩa khác nhau, nên không có đường nào gộp chúng.
	opts := map[string]any{}
	if c.temperature != nil {
		opts["temperature"] = *c.temperature
	}
	if c.maxTokens > 0 {
		opts["num_predict"] = c.maxTokens
	}
	// `think` luôn gửi: VẮNG field không bằng `false` — model có khối suy nghĩ sẽ tự bật, và
	// trần chữ đi hết vào khối đó nên lời đáp về rỗng. Ollama nhận field này cả với model
	// không biết suy nghĩ, nên gửi sẵn không loại model nào.
	reqBody := map[string]any{
		"model":    c.model,
		"stream":   false,
		"messages": []map[string]string{{"role": "system", "content": system}, {"role": "user", "content": user}},
		"options":  opts,
		"think":    c.think,
	}
	// `keep_alive` giữ model nằm sẵn trong máy. Không khai thì Ollama dỡ nó sau ~5 phút
	// nhàn rỗi, và lượt gọi kế phải chờ nạp lại — cái chờ ấy nằm trên đường đi của đệ,
	// nên nới ngân sách chỉ là chịu đựng nó, còn đây là bỏ nó đi.
	if c.keepAlive != "" {
		reqBody["keep_alive"] = c.keepAlive
	}
	b, err := json.Marshal(reqBody)
	if err != nil {
		return "", reply, err
	}

	// Lượt gọi không có tiếng ở console: nhiều plugin gọi song song nên dòng ở đây không tự
	// khai của ai, mà mọi con số của nó đã vào `plugin.Trace` → `debug_detail.json` dưới
	// đúng plugin đã gọi (§ Quan sát được).
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/chat", bytes.NewReader(b))
	if err != nil {
		return "", reply, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		slog.Warn("localmodel: request failed", "model", c.model, "took", time.Since(start).Milliseconds(), "err", err)
		return "", reply, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Warn("localmodel: bad status", "status", resp.Status, "took", time.Since(start).Milliseconds())
		return "", reply, fmt.Errorf("local model returned %s", resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(&reply); err != nil {
		return "", reply, err
	}

	return reply.Message.Content, reply, nil
}
