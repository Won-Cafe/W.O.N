// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package usage

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// Vòi phải cho byte đi qua nguyên vẹn — nó là chỗ đọc số, không phải chỗ chặn.
func TestTapPassesBytesThrough(t *testing.T) {
	src := `{"usage":{"input_tokens":12,"cache_read_input_tokens":17000,"output_tokens":40}}`
	st := &Stats{}
	tp := &tap{rc: io.NopCloser(strings.NewReader(src)), st: st}
	got, err := io.ReadAll(tp)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != src {
		t.Fatalf("thân bị sửa: %s", got)
	}
	if st.Input != 12 || st.CacheRead != 17000 || st.Output != 40 {
		t.Fatalf("số đọc sai: %+v", st)
	}
}

// `cache_creation_input_tokens` có đuôi là `input_tokens` — dấu ngoặc kép mở là
// thứ tách hai trường. Đọc lẫn thì con số quan trọng nhất bị ghi đè.
func TestTapDoesNotConfuseCacheFieldWithInput(t *testing.T) {
	st := &Stats{}
	tp := &tap{rc: io.NopCloser(strings.NewReader(`{"cache_creation_input_tokens":9999}`)), st: st}
	io.ReadAll(tp)
	if st.CacheWrite != 9999 {
		t.Errorf("cache_write phải là 9999, got %d", st.CacheWrite)
	}
	if st.Input != 0 {
		t.Errorf("input không được nhận giá trị của cache_write, got %d", st.Input)
	}
}

// Stream đếm dồn: output_tokens tăng qua từng message_delta, giá trị cuối là
// giá trị đúng. Và một con số bị cắt đôi giữa hai lần Read vẫn phải đọc được —
// đó là việc của đuôi giữ lại.
func TestTapReadsLastValueAcrossChunks(t *testing.T) {
	sse := `event: message_start
data: {"message":{"usage":{"input_tokens":5,"cache_read_input_tokens":16800,"output_tokens":1}}}

event: message_delta
data: {"usage":{"output_tokens":137}}

`
	st := &Stats{}
	tp := &tap{rc: io.NopCloser(strings.NewReader(sse)), st: st}
	// Đọc từng 7 byte để chắc chắn có con số bị cắt đôi.
	buf := make([]byte, 7)
	for {
		if _, err := tp.Read(buf); err != nil {
			break
		}
	}
	if st.Output != 137 {
		t.Errorf("output phải là giá trị cuối 137, got %d", st.Output)
	}
	if st.CacheRead != 16800 {
		t.Errorf("cache_read bị cắt đôi mà không đọc được: %d", st.CacheRead)
	}
}

// OpenAI (và cửa compat của Gemini) treo phần đọc từ cache ở
// `prompt_tokens_details.cached_tokens`. Trước đây không trường nào khớp, nên cache_read
// đứng 0 vĩnh viễn trên cả nhánh này — mà 0 là dấu "có gì đang phá tiền tố", tức một
// báo động giả suốt đời.
//
// Đọc từng 7 byte: `prompt_tokens` tới TRƯỚC `cached_tokens`, nên đây cũng là chỗ khoá
// việc Input được suy lại từ hai số thô chứ không trừ dần.
func TestTapReadsOpenAICachedTokens(t *testing.T) {
	src := `{"usage":{"prompt_tokens":31000,"completion_tokens":80,` +
		`"prompt_tokens_details":{"cached_tokens":30000}}}`
	st := &Stats{}
	tp := &tap{rc: io.NopCloser(strings.NewReader(src)), st: st}
	buf := make([]byte, 7)
	for {
		if _, err := tp.Read(buf); err != nil {
			break
		}
	}
	if st.CacheRead != 30000 {
		t.Errorf("cache_read phải là 30000, got %d", st.CacheRead)
	}
	// Nghĩa Anthropic: Input là phần TRẢ GIÁ ĐẦY ĐỦ, không gồm phần đọc từ cache.
	if st.Input != 1000 {
		t.Errorf("input phải là 31000-30000, got %d", st.Input)
	}
	if got := st.Input + st.CacheRead + st.CacheWrite; got != 31000 {
		t.Errorf("đẳng thức tổng prompt vỡ: %d, phải là 31000", got)
	}
}

// `prompt_tokens_details` có TIỀN TỐ là `prompt_tokens`, nên dấu ngoặc kép đóng là thứ
// tách hai trường — cùng cái bẫy của `cache_creation_input_tokens`, ở đầu bên kia.
func TestTapDoesNotConfusePromptDetailsWithPromptTokens(t *testing.T) {
	st := &Stats{}
	tp := &tap{rc: io.NopCloser(strings.NewReader(
		`{"usage":{"prompt_tokens_details":{"cached_tokens":7}}}`)), st: st}
	io.ReadAll(tp)
	if st.CacheRead != 7 {
		t.Errorf("cached_tokens phải là 7, got %d", st.CacheRead)
	}
	if st.Input != 0 {
		t.Errorf("vắng prompt_tokens thì Input phải là 0, got %d", st.Input)
	}
}

// Gemini native đặt tên hẳn theo lối khác, và tách chữ suy nghĩ ra một trường riêng — mà
// phần đó vẫn tính giá đầu ra, nên `output` phải là tổng. `promptTokenCount` thì GỘP phần
// cached, y như OpenAI.
func TestTapReadsGeminiUsageMetadata(t *testing.T) {
	src := `{"usageMetadata":{"promptTokenCount":33000,"cachedContentTokenCount":31000,` +
		`"candidatesTokenCount":120,"thoughtsTokenCount":400,"totalTokenCount":33520}}`
	st := &Stats{}
	tp := &tap{rc: io.NopCloser(strings.NewReader(src)), st: st}
	buf := make([]byte, 7)
	for {
		if _, err := tp.Read(buf); err != nil {
			break
		}
	}
	if st.CacheRead != 31000 {
		t.Errorf("cache_read phải là 31000, got %d", st.CacheRead)
	}
	if st.Input != 2000 {
		t.Errorf("input phải là 33000-31000, got %d", st.Input)
	}
	if st.Output != 520 {
		t.Errorf("output phải gồm cả chữ suy nghĩ (120+400), got %d", st.Output)
	}
	// Gemini không bán phí ghi cache ngầm: 0 ở đây là ĐÚNG, không phải vòi hỏng.
	if st.CacheWrite != 0 {
		t.Errorf("cache_write phải là 0 trên đường Gemini, got %d", st.CacheWrite)
	}
	if got := st.Input + st.CacheRead; got != 33000 {
		t.Errorf("tổng prompt vỡ: %d, muốn 33000", got)
	}
}

// Nhánh Anthropic không được đụng tới: `input_tokens` đã trừ cache sẵn, nên nó đi thẳng,
// không qua đường suy lại.
func TestTapLeavesAnthropicInputUntouched(t *testing.T) {
	st := &Stats{}
	tp := &tap{rc: io.NopCloser(strings.NewReader(
		`{"usage":{"input_tokens":40,"cache_read_input_tokens":30000}}`)), st: st}
	io.ReadAll(tp)
	if st.Input != 40 {
		t.Errorf("input của Anthropic phải giữ nguyên 40, got %d", st.Input)
	}
}

// Anthropic nén response, và trước đây gzip làm mất CẢ bốn con số LẪN lời đệ trả:
// `capture` giữ byte nén nên không rút được gì. gzip giải được bằng stdlib, nên khai
// "không đọc được" là khai sai — chỉ br mới thật không có cửa.
func TestReadsGzippedBody(t *testing.T) {
	plain := []byte(`{"usage":{"input_tokens":1234,"output_tokens":56,` +
		`"cache_read_input_tokens":900,"cache_creation_input_tokens":7}}`)
	var z bytes.Buffer
	zw := gzip.NewWriter(&z)
	zw.Write(plain)
	zw.Close()

	st := New(true)
	resp := &http.Response{
		Header: http.Header{"Content-Encoding": []string{"gzip"}},
		Body:   io.NopCloser(bytes.NewReader(z.Bytes())),
		Request: (&http.Request{}).WithContext(
			NewContext(context.Background(), st)),
	}
	if err := Tap(resp); err != nil {
		t.Fatal(err)
	}

	// Thân phải đi qua NGUYÊN nén — client giải, không phải ta (#1).
	got, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !bytes.Equal(got, z.Bytes()) {
		t.Error("thân đi qua bị sửa — client sẽ không giải được")
	}

	if !st.Seen {
		t.Fatal("không đọc được số nào từ thân gzip")
	}
	if st.Input != 1234 || st.Output != 56 || st.CacheRead != 900 || st.CacheWrite != 7 {
		t.Errorf("số sai: in=%d out=%d read=%d write=%d", st.Input, st.Output, st.CacheRead, st.CacheWrite)
	}
	if st.Encoding != "" {
		t.Errorf("gzip đọc được rồi mà vẫn khai không đọc được: %q", st.Encoding)
	}
	if !bytes.Equal(st.Body(), plain) {
		t.Error("capture phải giữ chữ ĐÃ GIẢI, không phải byte nén")
	}
}

// Nén kiểu stdlib không có cửa (br) thì khai ra, và không gắn vòi — số 0 giả tệ hơn
// không có số.
func TestDeclaresUnreadableEncoding(t *testing.T) {
	st := &Stats{}
	resp := &http.Response{
		Header: http.Header{"Content-Encoding": []string{"br"}},
		Body:   io.NopCloser(bytes.NewReader([]byte("\x00nén-brotli"))),
		Request: (&http.Request{}).WithContext(
			NewContext(context.Background(), st)),
	}
	Tap(resp)
	io.ReadAll(resp.Body)
	resp.Body.Close()
	if st.Encoding != "br" {
		t.Errorf("phải khai encoding br, got %q", st.Encoding)
	}
	if st.Seen {
		t.Error("không đọc được mà báo đã thấy số")
	}
}

// Upstream trả lỗi thì phải kêu lên. Trước đây lõi im, và một lượt 429 nhìn từ ngoài y
// như proxy treo: client tự retry với backoff, không in gì, người dùng chờ mãi. Đo được
// khi thử đường Anthropic thật — mất hai mươi phút mới thấy status=429.
func TestTapStillWorksOnErrorStatus(t *testing.T) {
	st := &Stats{}
	resp := &http.Response{
		StatusCode: 429,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(`{"type":"error","error":{"type":"rate_limit_error"}}`)),
		Request: (&http.Request{URL: mustURL(t, "http://x/v1/messages")}).WithContext(
			NewContext(context.Background(), st)),
	}
	if err := Tap(resp); err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(got), "rate_limit_error") {
		t.Error("thân lỗi phải đi qua nguyên vẹn — client cần đọc được lỗi")
	}
	// Thân lỗi không có usage: rỗng là ĐÚNG, không phải vòi hỏng.
	if st.Seen {
		t.Error("thân lỗi không có số mà báo đã thấy")
	}
}

func mustURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// Quét lại thân gzip không được làm số đổi: Input suy lại từ hai số THÔ ở cuối mỗi lần
// quét, không trừ dần — trừ dần thì lần quét thứ hai trừ CacheRead thêm một lần nữa.
func TestInputStableAcrossRescan(t *testing.T) {
	st := &Stats{}
	body := `{"usage":{"prompt_tokens":1000,"prompt_tokens_details":{"cached_tokens":750}}}`
	tp := &tap{rc: io.NopCloser(strings.NewReader(body)), st: st}
	io.ReadAll(tp)
	first := st.Input
	if first != 250 {
		t.Fatalf("Input = %d, muốn 250 (1000 gộp cả 750 đọc từ cache)", first)
	}
	tp.scan([]byte(body)) // quét lại đúng thân ấy, như đường gzip làm ở Close
	if st.Input != first {
		t.Errorf("quét lại ra %d, lần đầu %d", st.Input, first)
	}
}

// Khối `usage` trong nhật ký chỉ chở số và nhãn đọc được. Cờ NỘI BỘ không được lọt ra
// file: `Seen` từng lọt, và một khoá PascalCase giữa toàn snake_case đọc ra như một con
// số của nhà cung cấp.
func TestStatsJSONHasNoInternalFlags(t *testing.T) {
	b, err := json.Marshal(&Stats{Input: 1, Seen: true, Status: 200, limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	for _, banned := range []string{"Seen", "Status", "limit", "promptRaw", "body"} {
		if strings.Contains(got, banned) {
			t.Errorf("cờ nội bộ %q lọt ra nhật ký: %s", banned, got)
		}
	}
	// Số cache của nhà cung cấp KHÔNG ra nhật ký: câu chúng trả lời giờ lõi tự đo từ cấu
	// trúc (`cache`), và hai phép đo cho một câu hỏi là chỗ người đọc tin nhầm cái yếu hơn.
	for _, gone := range []string{"cache_read_input_tokens", "cache_creation_input_tokens", "cache_hit_pct"} {
		if strings.Contains(got, gone) {
			t.Errorf("số cache của nhà cung cấp còn ra nhật ký: %q trong %s", gone, got)
		}
	}
	for _, want := range []string{"input_tokens", "output_tokens"} {
		if !strings.Contains(got, want) {
			t.Errorf("thiếu %q: %s", want, got)
		}
	}
}

// Chữ suy nghĩ chảy TRƯỚC câu trả lời trên đường SSE. Một trần chung trên byte thô thì
// suy nghĩ ăn hết chỗ và `replied` ra rỗng — đo được trên một lượt glm-5.2 thật (9.874
// token đầu ra, `replied` rỗng). Rút dọc đường thì mỗi luồng tiêu chỗ của chính nó.
func TestTapKeepsAnswerAfterOversizedReasoning(t *testing.T) {
	var src bytes.Buffer
	think := strings.Repeat("n", 1000)
	for i := 0; i < 700; i++ { // ~700KB, vượt trần bản sao thân
		src.WriteString(`data: {"choices":[{"delta":{"reasoning_content":"` + think + `"}}]}` + "\n")
	}
	src.WriteString(`data: {"choices":[{"delta":{"content":"câu trả lời"}}]}` + "\n")
	src.WriteString(`data: {"usage":{"prompt_tokens":10,"completion_tokens":9874}}` + "\n")
	src.WriteString("data: [DONE]\n")

	st := New(true)
	tp := &tap{rc: io.NopCloser(bytes.NewReader(src.Bytes())), st: st}
	io.ReadAll(tp)
	tp.Close()

	if got := st.Text(); got != "câu trả lời" {
		t.Fatalf("lời đáp phải sống sót qua chữ suy nghĩ, có: %q", got)
	}
	if st.Thinking() == "" {
		t.Error("chữ suy nghĩ của đường OpenAI (`reasoning_content`) phải đọc được")
	}
	if st.Output != 9874 {
		t.Errorf("số usage phải còn nguyên, có: %d", st.Output)
	}
	// Đường cũ đọc lúc cuối trên bản sao thân đã cụt — giữ lại làm mốc đối chiếu.
	if AssistantText(st.Body()) != "" {
		t.Error("bản sao thân lẽ ra đã cạn trần trước khi lời đáp tới")
	}
}

// Một khung SSE có thể bị cắt đôi giữa hai chunk, và khung cuối thường không có xuống
// dòng đóng. Cả hai chỗ mất chữ thì mất im lặng.
func TestTapDistillsAcrossChunkAndTailLine(t *testing.T) {
	head := `data: {"choices":[{"delta":{"content":"một "}}]}` + "\n" + `data: {"choices":[{"delta":`
	tail := `{"content":"hai"}}]}` + "\n" + `data: {"choices":[{"delta":{"content":" ba"}}]}`

	st := New(true)
	tp := &tap{rc: io.NopCloser(io.MultiReader(strings.NewReader(head), strings.NewReader(tail))), st: st}
	io.ReadAll(tp)
	tp.Close()

	if got := st.Text(); got != "một hai ba" {
		t.Fatalf("chữ ghép sai: %q", got)
	}
}

// Thân một cục không có dòng `data:` nào: cờ sse phải nằm im và đường đọc vẫn là bản sao
// thân. Đoán theo header thay vì đo trên chữ là thêm một phép đoán (#6).
func TestTapReadsWholeBodyWhenNotSSE(t *testing.T) {
	st := New(true)
	src := `{"choices":[{"message":{"content":"một cục","tool_calls":[{"function":{"name":"read_file"}}]}}]}`
	tp := &tap{rc: io.NopCloser(strings.NewReader(src)), st: st}
	io.ReadAll(tp)
	tp.Close()

	if st.sse {
		t.Error("thân một cục không phải SSE")
	}
	if got := st.Text(); got != "một cục" {
		t.Fatalf("lời đáp sai: %q", got)
	}
	if calls := st.Calls(); len(calls) != 1 || calls[0] != "read_file" {
		t.Fatalf("tên tool sai: %v", calls)
	}
}
