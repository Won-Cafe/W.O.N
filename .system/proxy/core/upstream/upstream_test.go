// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package upstream

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestResolveUpstream(t *testing.T) {
	u, err := New("https://api.anthropic.com", nil)
	if err != nil {
		t.Fatal(err)
	}

	target, err := u.Resolve("")
	if err != nil || target == nil || target.Host != "api.anthropic.com" {
		t.Fatalf("no header must fall back to config: %v %v", target, err)
	}

	// Vành đai suy từ lời khai: đúng đích đã khai, cộng chính máy này. Không có danh sách
	// nào để khai thêm, nên cũng không có bẫy so-chuỗi giữa localhost và 127.0.0.1.
	cases := []struct {
		header string
		ok     bool
	}{
		{"https://api.anthropic.com", true}, // đích đã khai
		{"http://127.0.0.1:11434", true},    // chính máy này
		{"http://localhost:11434", true},    // cũng chính máy này, tên khác
		{"http://[::1]:11434", true},        // và IPv6
		{"http://169.254.169.254", false},   // host lạ — chống open-proxy
		{"http://evil.example", false},      // host lạ
		{"::invalid", false},                // header hỏng
	}
	for _, c := range cases {
		_, err := u.Resolve(c.header)
		if c.ok && err != nil {
			t.Errorf("header %q must be accepted: %v", c.header, err)
		}
		if !c.ok && err == nil {
			t.Errorf("header %q must be rejected clearly", c.header)
		}
	}

	// Lõi dựng không đích (chỉ người dựng lõi làm được — config luôn có mặc định): dòng
	// chính bị từ chối rõ thay vì đoán một người nhận.
	u2, err := New("", nil)
	if err != nil {
		t.Fatal(err)
	}
	if target, err := u2.Resolve(""); err != nil || target != nil {
		t.Fatalf("no source → nil, no error: %v %v", target, err)
	}
	if _, err := u2.Resolve("https://api.anthropic.com"); err == nil {
		t.Fatal("with no target declared, an outside host must be rejected clearly")
	}
}

// Đích mặc định là local nên 502 phải nói được LÀM GÌ TIẾP, không chỉ nói hỏng: "Ollama
// chưa bật" nhìn từ ngoài y như "proxy hỏng". Và client tự ngắt giữa dòng thì im — đó là
// chuyện thường của streaming, không phải lỗi của đích.
func TestUnreachableSpeaksTheNextStep(t *testing.T) {
	cases := []struct {
		name   string
		target string
		err    error
		want   string // rỗng = không được ghi gì ra client
	}{
		{"đích local → chỉ đường mở model server", "http://127.0.0.1:11434", errors.New("connection refused"), "start the model server"},
		{"localhost cũng là local", "http://localhost:11434", errors.New("connection refused"), "start the model server"},
		{"đích ngoài → soát lời khai", "https://api.anthropic.com", errors.New("no such host"), "won.conf"},
		{"client tự ngắt → im", "http://127.0.0.1:11434", context.Canceled, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			target, err := url.Parse(c.target)
			if err != nil {
				t.Fatal(err)
			}
			rec := httptest.NewRecorder()
			unreachable(*target)(rec, httptest.NewRequest(http.MethodPost, "/v1/messages", nil), c.err)

			body := rec.Body.String()
			if c.want == "" {
				if body != "" {
					t.Fatalf("phải im, mà ghi ra: %q", body)
				}
				return
			}
			if rec.Code != http.StatusBadGateway {
				t.Errorf("code = %d, want 502", rec.Code)
			}
			if !strings.Contains(body, c.want) {
				t.Errorf("lời khuyên thiếu %q:\n%s", c.want, body)
			}
			if !strings.Contains(body, target.Host) {
				t.Errorf("phải nói rõ đích nào:\n%s", body)
			}
		})
	}
}
