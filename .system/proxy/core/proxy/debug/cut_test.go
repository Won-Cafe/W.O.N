// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package debug

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"won/proxy/core/plugin"
)

// mustQuoteJSON — chuỗi Go thành literal JSON. Chữ thử có nhiều dòng và dấu ngoặc, nối tay
// vào là dựng một JSON hỏng rồi đi tìm bug ở chỗ khác.
func mustQuoteJSON(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// Bất biến nặng nhất của file này: cắt KHÔNG được sửa thân gốc. Thân đi ra upstream là
// chính slice ta đang cắt, nên một lần sửa tại chỗ ở đây là cắt luôn thân tới nhà cung cấp.
func TestCutNeverMutatesInput(t *testing.T) {
	bodies := []string{
		`{"system":[{"type":"text","text":"` + strings.Repeat("x", 300) + `"}],"messages":[{"role":"user","content":[{"type":"thinking","thinking":"` + strings.Repeat("y", 500) + `","signature":"` + strings.Repeat("z", 900) + `"}]}]}`,
		`{"systemInstruction":{"parts":[{"text":"` + strings.Repeat("x", 300) + `"}]},"contents":[{"role":"model","parts":[{"text":"ừ","thoughtSignature":"` + strings.Repeat("s", 3904) + `"}]}]}`,
		`{"tools":[{"functionDeclarations":[{"name":"read_file","description":"` + strings.Repeat("d", 400) + `"}]}]}`,
	}
	for i, body := range bodies {
		raw := []byte(body)
		before := string(raw)
		cutBody(raw)
		if string(raw) != before {
			t.Errorf("thân %d bị sửa tại chỗ — thân đi ra upstream sẽ bị cắt", i)
		}
	}
}

// Một luật cho MỌI chuỗi, không phân biệt của ai: lời người, khối của lõi, blob mã hoá,
// mô tả tool — cùng ngưỡng, cùng hai mép. Cắt đều nên không còn chỗ nào bắt oan chữ của
// công cụ chủ (`<system-reminder>` từng bị cắt vì trùng HÌNH thẻ với khối của lõi).
func TestCutTextKeepsBothEdges(t *testing.T) {
	short := strings.Repeat("đ", cutThreshold)
	if got := cutText(short); got != short {
		t.Errorf("dưới ngưỡng phải giữ nguyên văn: %q", got)
	}
	long := strings.Repeat("a", 60) + "GIỮA" + strings.Repeat("b", 60)
	got := cutText(long)
	if !strings.HasPrefix(got, strings.Repeat("a", cutEdge)) {
		t.Errorf("mất mép đầu: %q", got)
	}
	if !strings.HasSuffix(got, strings.Repeat("b", cutEdge)) {
		t.Errorf("mất mép cuối — một khối hỏng thường hỏng ở đuôi: %q", got)
	}
	if strings.Contains(got, "GIỮA") {
		t.Errorf("phần giữa phải bị bỏ: %q", got)
	}
	// Cắt theo RUNE: cắt theo byte thì một chữ tiếng Việt bị chặt đôi ngay tại mép.
	viet := strings.Repeat("đường", 60)
	if r := []rune(cutText(viet)); len(r) != cutEdge*2+len([]rune(cutMark)) {
		t.Errorf("cắt sai theo rune: %d ký tự", len(r))
	}
}

// Thứ tự khoá gốc còn nguyên, ở MỌI cấp: hai file thân là để so cấu trúc chủ gửi với cấu
// trúc ta gửi đi, mà map chuẩn của Go thì marshal theo bảng chữ cái.
func TestCutBodyPreservesKeyOrder(t *testing.T) {
	got := string(cutBody([]byte(`{"model":"m","messages":[{"role":"user","content":"` +
		strings.Repeat("x", 300) + `"}],"system":"s","tools":[]}`)))
	for _, pair := range [][2]string{
		{`"model"`, `"messages"`}, {`"messages"`, `"system"`}, {`"system"`, `"tools"`},
		{`"role"`, `"content"`},
	} {
		if strings.Index(got, pair[0]) > strings.Index(got, pair[1]) {
			t.Errorf("thứ tự %s trước %s bị xáo:\n%s", pair[0], pair[1], got)
		}
	}
}

// Cấu trúc giữ TRỌN, kể cả schema tool: cái file này soi là hình của request. Chỉ nội dung
// bị bóp — và số/bool không phải nội dung, chúng đọc được nguyên.
func TestCutBodyKeepsShapeAndScalars(t *testing.T) {
	got := string(cutBody([]byte(`{"max_tokens":64000,"stream":true,"temperature":0.1,
		"tools":[{"name":"Read","description":"` + strings.Repeat("d", 400) + `",
		"input_schema":{"type":"object","properties":{"path":{"type":"string","description":"` +
		strings.Repeat("p", 200) + `"}},"required":["path"]}}]}`)))
	for _, want := range []string{
		`"max_tokens": 64000`, `"stream": true`, `"temperature": 0.1`,
		"input_schema", "properties", `"path"`, "required", `"type": "object"`,
	} {
		if !strings.Contains(indent(t, got), want) {
			t.Errorf("mất %q trong bản cắt:\n%s", want, got)
		}
	}
	if strings.Contains(got, strings.Repeat("d", cutThreshold)) {
		t.Error("mô tả tool chưa bị cắt")
	}
	if strings.Contains(got, strings.Repeat("p", cutThreshold)) {
		t.Error("mô tả trong schema chưa bị cắt")
	}
}

// Dấu ngoặc phải đọc ra được bằng mắt: tên thẻ của khối lõi là chỗ nhận ra khối nào.
func TestCutBodyKeepsAngleBrackets(t *testing.T) {
	got := string(cutBody([]byte(`{"system":[{"text":` +
		mustQuoteJSON("<W.O.N>\n"+strings.Repeat("đất ", 400)+"\n</W.O.N>") + `}]}`)))
	if strings.Contains(got, `\u003c`) {
		t.Errorf("dấu ngoặc bị escape: %s", got)
	}
	if !strings.Contains(got, "<W.O.N>") || !strings.Contains(got, "</W.O.N>") {
		t.Errorf("hai mép của khối phải còn tên thẻ: %s", got)
	}
}

// Không phải JSON đọc được (nén, hỏng) → trả nguyên bản, đừng đoán nó đúng hình (#6).
func TestCutBodyPassesThroughUnreadable(t *testing.T) {
	for _, raw := range []string{"", "\x1f\x8b\x08 gzip", `{"a":`} {
		if got := string(cutBody([]byte(raw))); got != raw {
			t.Errorf("thân không đọc được phải giữ nguyên: %q → %q", raw, got)
		}
	}
}

// indent — bản thụt lề, để so câu `"khoá": giá trị` đúng như nó nằm trong file.
func indent(t *testing.T, s string) string {
	t.Helper()
	var out bytes.Buffer
	if err := json.Indent(&out, []byte(s), "", "  "); err != nil {
		t.Fatalf("bản cắt không phải JSON hợp lệ: %v\n%s", err, s)
	}
	return out.String()
}

// Bất biến song song với TestCutNeverMutatesInput, ở nhánh khác: cắt lời hệ thống của lượt
// gọi model nền KHÔNG được sửa `Trace.Calls` gốc — cửa chạy khô trả về đúng slice ấy, nên
// sửa nó là sửa chính prompt thật mà người đọc lại sau này nhìn thấy.
func TestCutCallSystemNeverMutatesInput(t *testing.T) {
	soul := strings.Repeat("s", 400)
	stages := []Stage{{Name: "gather", Plugins: []plugin.PluginDetail{
		{Name: "loiterer", Calls: []plugin.Call{{System: soul, User: "câu của người"}}},
		{Name: "identity"},
	}}}
	out := cutCallSystem(stages)
	if got := stages[0].Plugins[0].Calls[0].System; got != soul {
		t.Errorf("lời hệ thống gốc bị cắt tại chỗ: %d ký tự", len(got))
	}
	cut := out[0].Plugins[0].Calls[0]
	if len([]rune(cut.System)) != 2*cutEdge+len([]rune(cutMark)) {
		t.Errorf("bản nhật ký không về hai mép: %q", cut.System)
	}
	// `user` là chữ của CHÍNH lượt này, không phải bản sao của file trên đĩa — giữ nguyên.
	if cut.User != "câu của người" {
		t.Errorf("câu của người bị chạm: %q", cut.User)
	}
}
