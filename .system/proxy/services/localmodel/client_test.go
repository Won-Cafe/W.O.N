// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package localmodel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// warmServer — Ollama giả, giữ lại đường và thân của lượt nạp.
func warmServer(t *testing.T, status int) (*httptest.Server, *string, *map[string]any) {
	t.Helper()
	var path string
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"response":"","done":true}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &path, &body
}

// Warm nạp model mà KHÔNG sinh chữ: đường phải là /api/generate và prompt phải rỗng.
// Gọi /api/chat với một câu thật là trả tiền một lượt nói cho việc chỉ cần nạp.
func TestWarmLoadsWithoutGenerating(t *testing.T) {
	srv, path, body := warmServer(t, http.StatusOK)
	c := New(Config{BaseURL: srv.URL, Model: "m", KeepAlive: "30m", TimeoutMs: 50})
	if err := c.Warm(context.Background()); err != nil {
		t.Fatalf("warm: %v", err)
	}
	if *path != "/api/generate" {
		t.Errorf("đường = %q, muốn /api/generate", *path)
	}
	if got := (*body)["prompt"]; got != "" {
		t.Errorf("prompt = %v, phải rỗng — nạp thôi, không sinh chữ", got)
	}
	if got := (*body)["model"]; got != "m" {
		t.Errorf("model = %v, muốn m", got)
	}
	// keep_alive phải đi kèm: nạp xong mà không khai giữ thì Ollama dỡ sau ~5 phút và
	// cả lượt nạp này thành công cốc.
	if got := (*body)["keep_alive"]; got != "30m" {
		t.Errorf("keep_alive = %v, muốn 30m", got)
	}
}

// Không khai keep_alive thì không gửi field — cùng luật với Chat (#6): lõi không bịa
// giá trị cho núm người khai bỏ trống.
func TestWarmOmitsKeepAliveWhenUnset(t *testing.T) {
	srv, _, body := warmServer(t, http.StatusOK)
	c := New(Config{BaseURL: srv.URL, Model: "m"})
	if err := c.Warm(context.Background()); err != nil {
		t.Fatalf("warm: %v", err)
	}
	if _, ok := (*body)["keep_alive"]; ok {
		t.Error("keep_alive không khai thì không được gửi")
	}
}

// chatServer — Ollama giả cho lượt NÓI, giữ lại thân request.
func chatServer(t *testing.T) (*httptest.Server, *map[string]any) {
	t.Helper()
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`{"message":{"content":"ừ"},"done_reason":"stop"}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &body
}

// Núm sampling phải tới được model, và tới trong `options` — không phải field cấp một của
// thân. Ollama đọc `temperature` ở cấp một thành field lạ và bỏ qua, im lặng.
func TestChatSendsTemperatureInOptions(t *testing.T) {
	srv, body := chatServer(t)
	temp := 0.15
	c := New(Config{BaseURL: srv.URL, Model: "m", Temperature: &temp, MaxTokens: 64})
	if _, err := c.Chat(context.Background(), "sys", "usr"); err != nil {
		t.Fatalf("chat: %v", err)
	}
	opts, ok := (*body)["options"].(map[string]any)
	if !ok {
		t.Fatalf("thân phải chở options: %v", *body)
	}
	if opts["temperature"] != 0.15 {
		t.Errorf("temperature = %v, muốn 0.15", opts["temperature"])
	}
	if opts["num_predict"] != float64(64) {
		t.Errorf("num_predict = %v, muốn 64", opts["num_predict"])
	}
}

// `think` LUÔN có trong thân, cả khi là false. Vắng field không bằng `false`: model có khối
// suy nghĩ sẽ tự bật, trần chữ đi hết vào khối đó, và lời đáp về rỗng — nhìn từ ngoài giống
// hệt một lần im lặng.
func TestChatAlwaysSendsThink(t *testing.T) {
	for _, want := range []bool{false, true} {
		srv, body := chatServer(t)
		c := New(Config{BaseURL: srv.URL, Model: "m", Think: want})
		if _, err := c.Chat(context.Background(), "sys", "usr"); err != nil {
			t.Fatalf("chat: %v", err)
		}
		got, ok := (*body)["think"]
		if !ok {
			t.Fatalf("think phải có trong thân: %v", *body)
		}
		if got != want {
			t.Errorf("think = %v, muốn %v", got, want)
		}
	}
}

// `temperature = off` ở won.conf = field VẮNG, không phải `temperature: 0`. Với Ollama, `0`
// là một lời khai — luôn bốc nhánh đỉnh — còn vắng là nhường mặc định của nó. Gửi 0 thay cho
// vắng là lặng lẽ đổi hành vi, và đó là đường duy nhất người khai không kiểm được.
func TestChatOmitsTemperatureWhenUnset(t *testing.T) {
	srv, body := chatServer(t)
	c := New(Config{BaseURL: srv.URL, Model: "m"})
	if _, err := c.Chat(context.Background(), "sys", "usr"); err != nil {
		t.Fatalf("chat: %v", err)
	}
	opts, _ := (*body)["options"].(map[string]any)
	if got, ok := opts["temperature"]; ok {
		t.Errorf("không khai thì không được gửi temperature, got %v", got)
	}
}

// Nạp hỏng phải TRẢ LỖI để người gọi kêu lên, không nuốt: khởi động vẫn chạy tiếp,
// nhưng lượt đầu sẽ chậm và người giữ hệ có quyền biết vì sao.
func TestWarmReportsFailure(t *testing.T) {
	srv, _, _ := warmServer(t, http.StatusInternalServerError)
	c := New(Config{BaseURL: srv.URL, Model: "m"})
	if err := c.Warm(context.Background()); err == nil {
		t.Error("Ollama trả 500 mà Warm im lặng")
	}
}

// Client nil-safe ở mọi cửa: chưa cấu hình model thì New trả nil, và nil.Warm không
// được sập — main gọi nó trong một goroutine không ai bắt panic.
func TestWarmOnNilClient(t *testing.T) {
	var c *Client
	if err := c.Warm(context.Background()); err == nil {
		t.Error("client nil phải trả lỗi, không phải nil")
	}
}
