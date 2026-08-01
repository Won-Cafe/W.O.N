// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"won/proxy/core/request"
)

// TestRouterPathFormat — router quyết format từ path (bất biến #6, không sniff body):
// POST /v1/messages → Anthropic (mainline), POST /v1/chat/completions → OpenAI
// (mainline), path lạ → Unknown (passthrough, không inject).
func TestRouterPathFormat(t *testing.T) {
	var lastPath string
	var lastBody string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		lastBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`))
	}))
	defer ts.Close()

	p := newTestProxy(t, ts.URL)

	// mainline Anthropic: POST /v1/messages — marker horn được chèn (KindMarker).
	anthBody := `{"model":"m","system":"s","messages":[{"role":"user","content":"hi"}]}`
	w := call(t, p, "POST", "/v1/messages", anthBody)
	if w.Code != http.StatusOK {
		t.Fatalf("/v1/messages want 200, got %d: %s", w.Code, w.Body)
	}
	if lastPath != "/v1/messages" {
		t.Fatalf("forwarded path: %q", lastPath)
	}
	if !strings.Contains(lastBody, "horn") {
		t.Fatalf("Anthropic mainline must inject marker: %s", lastBody)
	}

	// mainline OpenAI: POST /v1/chat/completions — marker horn thành một message
	// mới sau lượt người, và lời hệ thống của client không bị chạm.
	oaiBody := `{"model":"gpt","messages":[{"role":"system","content":"s"},{"role":"user","content":"hi"}]}`
	lastBody = ""
	w = call(t, p, "POST", "/v1/chat/completions", oaiBody)
	if w.Code != http.StatusOK {
		t.Fatalf("/v1/chat/completions want 200, got %d: %s", w.Code, w.Body)
	}
	if lastPath != "/v1/chat/completions" {
		t.Fatalf("forwarded path: %q", lastPath)
	}
	if !strings.Contains(lastBody, "horn") {
		t.Fatalf("OpenAI mainline must inject marker: %s", lastBody)
	}
	// System text của body forwarded phải còn nguyên (không bị biến dạng).
	if !strings.Contains(lastBody, `"role":"system","content":"s"`) {
		t.Fatalf("OpenAI system message must be preserved: %s", lastBody)
	}

	// passthrough: GET /v1/models — không dòng chính, đi qua nguyên vẹn.
	lastBody = ""
	w = call(t, p, "GET", "/v1/models", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET passthrough want 200, got %d", w.Code)
	}

	// passthrough: POST path lạ — Unknown format, không inject.
	lastBody = ""
	weird := `{"x":1}`
	w = call(t, p, "POST", "/v1/weird", weird)
	if w.Code != http.StatusOK {
		t.Fatalf("weird path passthrough want 200, got %d: %s", w.Code, w.Body)
	}
	if lastBody != weird {
		t.Fatalf("passthrough must forward untouched: %q", lastBody)
	}
	if strings.Contains(lastBody, "horn") {
		t.Fatalf("passthrough must NOT inject: %s", lastBody)
	}
}

// TestRouterUnknownNoFilter — danh mục tool đi qua nguyên vẹn ở mọi format. Lõi
// không còn đường nào thu hẹp `tools`; test này khoá cả nhánh Unknown (fail-open,
// chốt #3) lẫn nhánh đã biết.
func TestRouterUnknownNoFilter(t *testing.T) {
	var lastBody string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		lastBody = string(b)
		w.Write([]byte(`{}`))
	}))
	defer ts.Close()

	p := newTestProxy(t, ts.URL)
	// Một lượt dòng chính trước, có tools: cả hai món phải còn nguyên bên kia.
	anthBody := `{"model":"m","system":"s","messages":[{"role":"user","content":"hi"}],"tools":[{"name":"keep"},{"name":"alsokeep"}]}`
	call(t, p, "POST", "/v1/messages", anthBody)
	if !strings.Contains(lastBody, `"keep"`) || !strings.Contains(lastBody, `"alsokeep"`) {
		t.Fatalf("dòng chính không được cắt món nào: %q", lastBody)
	}

	// Lượt Unknown: body có tools → passthrough nguyên vẹn.
	weird := `{"tools":[{"name":"a"},{"name":"b"}]}`
	lastBody = ""
	call(t, p, "POST", "/v1/weird", weird)
	if lastBody != weird {
		t.Fatalf("Unknown passthrough must not touch tools: %q", lastBody)
	}
}

// TestFormatFromPathUnit — kiểm tra trực tiếp helper FormatFromPath.
func TestFormatFromPathUnit(t *testing.T) {
	cases := []struct {
		path string
		want request.Format
	}{
		{"/v1/messages", request.FormatAnthropic},
		{"/v1/chat/completions", request.FormatOpenAI},
		{"/something/else", request.FormatUnknown},
		{"/v1/messages/count", request.FormatUnknown}, // suffix phải khớp đúng
		{"", request.FormatUnknown},
	}
	for _, c := range cases {
		if got := request.FormatFromPath(c.path); got != c.want {
			t.Fatalf("FormatFromPath(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

// Không nhận ra đệ nào mà có đệ mặc định → dùng nó. Thiếu cơ chế này thì Claude Code
// (không có chỗ chọn đệ) phải dán trọn soul mỗi lần gọi — thủ tục rườm rà cho việc lẽ
// ra tự chạy. Chỉ kiểm đúng chỗ quyết định: mainline chọn tên nào.
func TestDefaultAgentFillsInWhenUnrecognized(t *testing.T) {
	// Một lượt của đệ: có lời dạy, có cuộc đang chảy.
	turn := parsed(t, `{"messages":[
		{"role":"system","content":"lời của công cụ chủ"},
		{"role":"user","content":"hỏi 1"},
		{"role":"assistant","content":"đáp 1"},
		{"role":"user","content":"hỏi 2"}]}`)
	book := emptyResolver{}

	if got, _ := agentFor(Deps{Identity: book, DefaultAgent: "Tzu"}, "", turn); got != "Tzu" {
		t.Errorf("không nhận ra ai mà có mặc định → phải là Tzu, got %q", got)
	}
	// Tắt thì về đúng nếp cũ: rỗng, và mainline chuyển tiếp nguyên bản.
	if got, why := agentFor(Deps{Identity: book, DefaultAgent: ""}, "", turn); got != "" || why != "no agent" {
		t.Errorf("tắt mặc định thì phải rỗng: %q %q", got, why)
	}
	// Nhận ra rồi thì mặc định KHÔNG được giành chỗ.
	sunTurn := parsed(t, `{"messages":[
		{"role":"system","content":"# Sun - 🗺️ The Scout\nlời dạy"},
		{"role":"user","content":"hỏi 1"},
		{"role":"assistant","content":"đáp 1"},
		{"role":"user","content":"hỏi 2"}]}`)
	if got, _ := agentFor(Deps{Identity: namedResolver("Sun"), DefaultAgent: "Tzu"}, "", sunTurn); got != "Sun" {
		t.Errorf("đã nhận ra Sun mà mặc định giành chỗ: %q", got)
	}
}

// Ba cửa chặn lượt việc nhà phải đứng trên MỌI đường lõi tự nhận ra đệ, không chỉ trên đường
// đoán. Lượt đặt tiêu đề mang đúng lời hệ thống của đệ nên nó khớp tiêu đề soul y như một
// lượt thật — và lọt vào thì nó vừa tốn đất, vừa mang một thân không phải hội thoại ấy vào
// phiên, chỗ sổ ghim mất mỏ neo (§ Session).
func TestHousekeepingTurnIsGatedOnEveryPathTheCoreGuesses(t *testing.T) {
	// Đặt tiêu đề: có lời dạy, nhưng một message và không tools — không cuộc nào đang chảy.
	chore := parsed(t, `{"messages":[
		{"role":"system","content":"# Sun - 🗺️ The Scout"},
		{"role":"user","content":"đặt tiêu đề cho phiên"}]}`)

	if got, why := agentFor(Deps{Identity: namedResolver("Sun")}, "", chore); got != "" || why != "housekeeping turn" {
		t.Errorf("đường tiêu đề phải qua ba cửa: got %q (%q)", got, why)
	}
	if got, why := agentFor(Deps{Identity: emptyResolver{}, DefaultAgent: "Tzu"}, "", chore); got != "" || why != "housekeeping turn" {
		t.Errorf("đường đoán phải qua ba cửa: got %q (%q)", got, why)
	}
	// Lời khai của người thì KHÔNG đo lại: người đã nói đây là lượt của Sun.
	if got, _ := agentFor(Deps{Identity: namedResolver("Sun")}, "Sun", chore); got != "Sun" {
		t.Errorf("đệ tường minh không phải qua cửa nào: got %q", got)
	}
}

func parsed(t *testing.T, body string) *request.Body {
	t.Helper()
	b, err := request.ParseBody([]byte(body), request.FormatOpenAI)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

type emptyResolver struct{}

func (emptyResolver) Resolve(string, string) string { return "" }

// namedResolver — bản giả của sổ soul, giữ đúng HAI ĐƯỜNG của bản thật: tên khai ở header,
// hoặc tiêu đề soul nằm trong lời hệ thống. Trả tên bất kể tham số thì mọi lượt hoá "đệ tường
// minh", và cửa lượt việc nhà không bao giờ nổ trong test.
type namedResolver string

func (n namedResolver) Resolve(header, systemText string) string {
	if strings.EqualFold(header, string(n)) {
		return string(n)
	}
	if systemText != "" && strings.Contains(systemText, "# "+string(n)) {
		return string(n)
	}
	return ""
}
