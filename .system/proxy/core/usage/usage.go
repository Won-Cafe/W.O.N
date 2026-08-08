// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

// Package usage đọc bốn con số usage nhà cung cấp trả về, dọc theo thân
// response đang chảy qua — không chặn, không đợi thân xong.
package usage

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
)

// Stats — con số nhà cung cấp trả về. Chỉ hai cái ra nhật ký: cỡ prompt và cỡ lời đáp.
// CacheWrite/CacheRead vòi vẫn đọc nhưng không khai: câu chúng trả lời — tiền tố có đứng
// yên không — giờ lõi tự đo từ cấu trúc (§ Cache). Giữ lại vì Input suy từ chúng.
type Stats struct {
	Input      int    `json:"input_tokens"`
	Output     int    `json:"output_tokens"`
	Encoding   string `json:"encoding,omitempty"` // thân nén → không đọc được số
	CacheWrite int    `json:"-"`
	CacheRead  int    `json:"-"`
	// Seen — vòi đã đọc được số hay chưa. Cờ NỘI BỘ, không ra nhật ký: khối `usage` chỉ
	// được ghi khi nó đúng, nên một `"Seen": true` trong file không nói thêm gì.
	Seen bool `json:"-"`
	// Status — mã đích trả về. 0 nghĩa là chưa có lời đáp nào: đích không với tới được, và
	// ErrorHandler chạy thay cho vòi. Dòng chính đọc nó để biết lượt này đã được trả lời
	// chưa (§ Session). Cờ nội bộ, không ra nhật ký.
	Status int `json:"-"`

	// Bản sao thân response, để rút lời đệ vừa trả. Trần hai cỡ: nhật ký debug cần TRỌN lời
	// đáp, dòng chính chỉ cần khúc đầu — đủ để nhận ra nhánh nào vừa nói (§ Session). Luôn giữ,
	// vì cái dòng chính hỏi không phải là chuyện chẩn bệnh.
	limit int
	body  []byte

	// Ba luồng đã RÚT dọc đường, mỗi luồng một trần riêng — chỉ dùng cho thân SSE.
	//
	// Một trần chung trên byte thô thì luồng nào chảy trước ăn hết chỗ, và trên đường SSE thì
	// chữ suy nghĩ luôn chảy trước câu trả lời: đo trên một lượt glm-5.2 thật, 9.874 token đầu
	// ra mà `replied` rỗng — trần 512KB đã cạn trước khi câu trả lời bắt đầu. Rút tại chỗ thì
	// mỗi luồng tiêu chỗ của chính nó, và cái mất khi tràn là phần đuôi của luồng ấy, không
	// phải một luồng khác.
	sse   bool
	text  []byte
	think []byte
	calls []string

	// promptRaw — `prompt_tokens` (OpenAI) / `promptTokenCount` (Gemini) thô, giữ RIÊNG vì cả
	// hai đều GỘP cả CacheRead. Input suy lại từ nó ở cuối mỗi lần quét, nên không con số nào
	// bị trừ hai lần.
	promptRaw int
	// candidatesRaw / thoughtsRaw — Gemini tách chữ trả về khỏi chữ suy nghĩ, mà cả hai đều
	// tính giá đầu ra. Giữ thô rồi cộng lại ở cuối, không cộng dần: quét lại thân gzip thì
	// cộng dần ra số gấp đôi.
	candidatesRaw int
	thoughtsRaw   int
}

// New — full bật (nhật ký debug mở) thì giữ trọn thân response trong trần lớn; tắt thì chỉ
// giữ khúc đầu, đủ cho dòng chính đọc vân tay lời đáp.
func New(full bool) *Stats {
	if full {
		return &Stats{limit: maxCapture}
	}
	return &Stats{limit: peekCapture}
}

// Body — bản sao thân response đã giải nén, rỗng nếu capture tắt hoặc chưa đọc gì.
func (s *Stats) Body() []byte { return s.body }

// Text — lời đệ vừa trả. Thân SSE đã rút xong dọc đường; thân một cục thì đọc tại đây, vì
// một cục JSON dở dang không đọc được từng khúc.
func (s *Stats) Text() string {
	if s.sse {
		return string(s.text)
	}
	return AssistantText(s.body)
}

// Thinking — chữ suy nghĩ, luồng riêng. Cùng hai đường như Text.
func (s *Stats) Thinking() string {
	if s.sse {
		return string(s.think)
	}
	return AssistantThinking(s.body)
}

// Calls — tên tool đệ vừa gọi. Cùng hai đường như Text.
func (s *Stats) Calls() []string {
	if s.sse {
		return s.calls
	}
	return AssistantCalls(s.body)
}

// ctxKey — chỗ mainline gửi sổ ghi số theo request, ModifyResponse nhận lại.
type ctxKey struct{}

// NewContext gắn Stats vào request context trước khi forward, để Tap đọc lại
// được ở ModifyResponse.
func NewContext(ctx context.Context, s *Stats) context.Context {
	return context.WithValue(ctx, ctxKey{}, s)
}

// FromContext — không có Stats trong context (request không qua NewContext) → nil.
func FromContext(ctx context.Context) *Stats {
	s, _ := ctx.Value(ctxKey{}).(*Stats)
	return s
}

// Tap gắn vòi đọc số vào response. Thân đi qua nguyên vẹn — vòi chỉ nhìn byte.
func Tap(resp *http.Response) error {
	st := FromContext(resp.Request.Context())
	if st == nil {
		return nil
	}
	// Ghi mã trước mọi nhánh rẽ: thân nén kiểu lạ thì vòi bỏ cuộc ở dưới, mà câu "đích đã
	// trả lời chưa" vẫn phải có lời đáp.
	st.Status = resp.StatusCode
	// gzip giải được bằng stdlib nên vẫn đọc được số; thân đi qua vẫn NGUYÊN nén —
	// vòi giải một bản sao riêng để đọc, không chạm cái client nhận (#1).
	tap := &tap{rc: resp.Body, st: st}
	switch enc := resp.Header.Get("Content-Encoding"); enc {
	case "", "identity":
	case "gzip":
		tap.gz = true
	default:
		// Nén kiểu khác (br…) thì stdlib không có cửa. Khai ra, không báo 0 — một con
		// số 0 giả còn tệ hơn không có số.
		st.Encoding = enc
		return nil
	}
	// Nhà cung cấp trả lỗi thì kêu lên: một lượt 429 im lặng nhìn từ ngoài y như proxy
	// treo, vì client tự retry với backoff và không in gì. Thân lỗi không có usage, nên
	// bốn con số rỗng ở đó là đúng chứ không phải vòi hỏng.
	if resp.StatusCode >= 400 {
		slog.Warn("upstream: error response", "status", resp.StatusCode,
			"path", resp.Request.URL.Path, "encoding", resp.Header.Get("Content-Encoding"))
	}
	resp.Body = tap
	return nil
}

// reUsage bắt các trường usage ở bất kỳ đâu trong thân. Hai nhà đặt tên khác
// nhau: Anthropic dùng input/output_tokens, OpenAI (và Ollama) dùng
// prompt/completion_tokens. Phần đọc từ cache thì OpenAI treo sâu một tầng, ở
// `prompt_tokens_details.cached_tokens` — quét theo tên trường nên tầng không
// quan trọng, chỉ cái tên. Dấu ngoặc kép ĐÓNG tách `prompt_tokens` khỏi
// `prompt_tokens_details`, y như nó tách `cache_creation_input_tokens`.
// Gemini native thì đặt tên hẳn theo lối khác (`usageMetadata`), và tách phần suy nghĩ ra
// một trường riêng — mà phần đó vẫn tính giá đầu ra.
var reUsage = regexp.MustCompile(`"(input_tokens|output_tokens|prompt_tokens|completion_tokens|cache_creation_input_tokens|cache_read_input_tokens|cached_tokens|promptTokenCount|candidatesTokenCount|cachedContentTokenCount|thoughtsTokenCount)"\s*:\s*(\d+)`)

const (
	// overlap — đuôi giữ giữa hai chunk, để con số bị cắt đôi vẫn đọc được.
	overlap = 64
	// maxCapture — trần bản sao thân response. Bộ nhớ không phình theo response.
	maxCapture = 512 << 10
	// peekCapture — trần khi nhật ký debug tắt: dòng chính chỉ đọc khúc đầu lời đáp, và khúc
	// ấy nằm gọn trong vài chục KB kể cả khi mỗi chữ đi một khung SSE riêng.
	peekCapture = 64 << 10
)

// tap cho byte đi qua nguyên vẹn, dọc đường nhặt số. Dùng được cho cả JSON
// một cục lẫn dòng SSE: thấy trường nào thì ghi đè — giá trị cuối là đúng.
// Không khoá: ReverseProxy copy trong đúng goroutine, mainline đọc sau khi ServeHTTP trả.
type tap struct {
	rc   io.ReadCloser
	st   *Stats
	tail []byte
	gz   bool   // thân nén gzip: gom lại, giải một lần lúc Close
	zbuf []byte // bản sao byte nén, chỉ dùng khi gz
	line []byte // dòng SSE dở dang: một khung có thể bị cắt đôi giữa hai chunk
}

func (t *tap) Read(p []byte) (int, error) {
	n, err := t.rc.Read(p)
	if n > 0 {
		t.scan(p[:n])
	}
	return n, err
}

// Close — chỗ duy nhất đọc được thân gzip: gom xong mới giải. Không giải theo từng
// chunk vì một chunk gzip lẻ không tự giải được, mà usage nằm ở cuối thân.
func (t *tap) Close() error {
	if t.gz && len(t.zbuf) > 0 {
		if plain, err := gunzip(t.zbuf); err == nil {
			// Hạ cờ rồi scan lại: một đường ghi duy nhất cho cả số và capture, nên lời
			// đệ trả cũng thành chữ đã giải mà không cần gán tay.
			t.gz = false
			t.scan(plain)
		} else {
			t.st.Encoding = "gzip" // giải hỏng thì khai ra, không báo 0
		}
		t.zbuf = nil
	}
	// Khung cuối thường không có xuống dòng đóng: không xả thì lời đáp cụt đúng một khung.
	t.event(t.line)
	t.line = nil
	return t.rc.Close()
}

// maxLine — trần một dòng SSE dở dang. Khung thật lớn nhất đo được còn xa dưới đây; dài
// hơn nghĩa là thân này không xuống dòng, và giữ tiếp chỉ để phình bộ nhớ.
const maxLine = 1 << 20

// distill rút chữ khỏi từng dòng SSE ngay khi dòng ấy trọn vẹn, rồi bỏ byte thô đi. Thân
// một cục không có dòng `data:` nào nên hàm này không thấy gì, và bản sao thân vẫn là
// đường đọc — đó là lý do `sse` phải là cờ ĐO ĐƯỢC, không phải một phép đoán theo header (#6).
func (t *tap) distill(chunk []byte) {
	t.line = append(t.line, chunk...)
	for {
		i := bytes.IndexByte(t.line, '\n')
		if i < 0 {
			break
		}
		t.event(t.line[:i])
		t.line = t.line[i+1:]
	}
	if len(t.line) > maxLine {
		t.line = t.line[:0]
	}
}

// event đọc MỘT dòng SSE. Không phải `data: {…}` thì bỏ qua: dòng `event:`, dòng trống,
// và `data: [DONE]` đều không chở chữ.
func (t *tap) event(line []byte) {
	line = bytes.TrimSpace(line)
	if !bytes.HasPrefix(line, []byte("data:")) {
		return
	}
	p := bytes.TrimSpace(line[len("data:"):])
	if len(p) == 0 || p[0] != '{' {
		return
	}
	t.st.sse = true
	t.st.text = appendCapped(t.st.text, eventText(p), t.st.limit)
	t.st.think = appendCapped(t.st.think, eventThinking(p), t.st.limit)
	t.st.calls = addCalls(t.st.calls, eventCalls(p))
}

// appendCapped nối chữ tới trần rồi thôi. Trần theo byte, không theo rune: chỗ này đo bộ
// nhớ, và một chữ tiếng Việt bị chặt đôi ở mép cuối rẻ hơn một luồng không có trần.
func appendCapped(dst []byte, s string, limit int) []byte {
	if s == "" || len(dst) >= limit {
		return dst
	}
	if room := limit - len(dst); len(s) > room {
		s = s[:room]
	}
	return append(dst, s...)
}

func (t *tap) scan(chunk []byte) {
	if t.gz {
		if len(t.zbuf) < maxCapture {
			t.zbuf = append(t.zbuf, chunk...)
		}
		return // byte nén không có gì đọc được; Close sẽ gọi lại với chữ thật
	}
	if len(t.st.body) < t.st.limit {
		t.st.body = append(t.st.body, chunk...)
	}
	t.distill(chunk)
	buf := chunk
	if len(t.tail) > 0 {
		buf = append(append([]byte{}, t.tail...), chunk...)
	}
	for _, m := range reUsage.FindAllSubmatch(buf, -1) {
		v, err := strconv.Atoi(string(m[2]))
		if err != nil {
			continue
		}
		t.st.Seen = true
		switch string(m[1]) {
		case "input_tokens":
			t.st.Input = v
		case "prompt_tokens", "promptTokenCount":
			t.st.promptRaw = v
		case "output_tokens", "completion_tokens":
			t.st.Output = v
		case "cache_creation_input_tokens":
			t.st.CacheWrite = v
		case "cache_read_input_tokens", "cached_tokens", "cachedContentTokenCount":
			t.st.CacheRead = v
		case "candidatesTokenCount":
			t.st.candidatesRaw = v
		case "thoughtsTokenCount":
			t.st.thoughtsRaw = v
		}
	}
	// Gemini tách chữ trả về khỏi chữ suy nghĩ; giá đầu ra tính cả hai, nên `output` của dòng
	// log phải là tổng. Không có `cache_write` nào trên đường này: Gemini không bán phí ghi
	// cache ngầm, nên 0 ở đó là ĐÚNG chứ không phải vòi hỏng.
	if t.st.candidatesRaw > 0 || t.st.thoughtsRaw > 0 {
		t.st.Output = t.st.candidatesRaw + t.st.thoughtsRaw
	}
	// Hai nhà đếm hai nghĩa: `input_tokens` của Anthropic đã TRỪ phần đọc từ cache, `prompt_tokens`
	// của OpenAI thì GỘP cả nó. Quy về nghĩa Anthropic — nghĩa duy nhất giữ được đẳng thức
	// "Tổng prompt = Input + CacheRead + CacheWrite" cho mọi nhà.
	//
	// Suy lại từ hai số THÔ ở cuối mỗi lần quét, không trừ dần: thân gzip bị quét hai lần.
	if t.st.promptRaw > 0 {
		t.st.Input = max(t.st.promptRaw-t.st.CacheRead, 0)
	}
	if len(buf) > overlap {
		buf = buf[len(buf)-overlap:]
	}
	t.tail = append(t.tail[:0], buf...)
}

// gunzip giải một thân gzip trọn vẹn, có trần: thân response là chữ người ngoài gửi,
// nên tỉ lệ nén cao không được phép nở thành bộ nhớ không giới hạn.
func gunzip(b []byte) ([]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	return io.ReadAll(io.LimitReader(zr, maxCapture))
}
