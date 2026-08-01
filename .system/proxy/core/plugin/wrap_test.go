// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package plugin

import (
	"log/slog"
	"strings"
	"testing"

	"won/proxy/core/request"
)

func mustBody(t *testing.T, raw string) *request.Body {
	t.Helper()
	b, err := request.ParseBody([]byte(raw), request.FormatAnthropic)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// Mỗi đóng góp vào một khối có tên, để bên nhận tách được chữ của hệ khỏi chữ
// của người. Tag nói đúng NỘI DUNG, không nói tên plugin: identity chèn soul.
func TestApplyWrapsInTaggedBlocks(t *testing.T) {
	body := mustBody(t, `{"system":"GỐC","messages":[{"role":"user","content":"hi"}]}`)
	cs := []Contribution{
		{Plugin: "identity", Kind: KindSystem, Tag: "Soul", Text: "# Tzu"},
		{Plugin: "memory", Kind: KindMarker, Tag: "Memory", Text: "## Memory focus"},
		{Plugin: "loiterer", Kind: KindMarker, Tag: "Loiterer", Text: "🚶 Loiterer: kìa"},
	}
	Apply(body, cs, freshSession(), speaking(cs))
	sys := body.SystemText()
	if !strings.Contains(sys, "<Soul>") || !strings.Contains(sys, "</Soul>") {
		t.Errorf("thiếu khối <Soul> trong system: %q", sys)
	}
	// Ký ức và marker đi sau lượt người — khối tag ở đó là thứ tách tiếng của
	// hệ khỏi chữ của người dùng.
	out, err := body.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	for _, tag := range []string{"Memory", "Loiterer"} {
		if !strings.Contains(string(out), "<"+tag+">") {
			t.Errorf("thiếu khối <%s>: %s", tag, out)
		}
	}
	// Dấu ngoặc phải đọc ra được bằng mắt, không thành cụm escape sáu byte như
	// dưới đây — xem mustJSON.
	if strings.Contains(string(out), "\\u003c") {
		t.Errorf("tag bị escape HTML: %s", out)
	}
}

// Tag rỗng → chèn trần, không bọc. Không bịa tên khối hộ plugin.
func TestApplyNoTagStaysBare(t *testing.T) {
	body := mustBody(t, `{"system":"GỐC","messages":[]}`)
	cs := []Contribution{{Plugin: "identity", Kind: KindSystem, Text: "# Tzu"}}
	Apply(body, cs, freshSession(), speaking(cs))
	if got := body.SystemText(); strings.Contains(got, "<") {
		t.Errorf("tag rỗng mà vẫn bọc: %q", got)
	}
}

// Tag KHÔNG cứu được một đóng góp vô danh: #4 kiểm trên text gốc, trước khi bọc.
// Nếu kiểm sau khi bọc thì mọi lời đều "có nguồn" nhờ cái tag — bất biến rỗng ruột.
func TestTagDoesNotLaunderAnonymousText(t *testing.T) {
	slog.SetDefault(slog.New(slog.NewTextHandler(&strings.Builder{}, nil)))
	body := mustBody(t, `{"system":"GỐC","messages":[{"role":"user","content":"hi"}]}`)
	cs := []Contribution{
		{Plugin: "rogue", Kind: KindMarker, Tag: "Loiterer", Text: "lời không mang tên nguồn"},
	}
	Apply(body, cs, freshSession(), speaking(cs))
	out, err := body.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "lời không mang tên nguồn") {
		t.Error("đóng góp vô danh phải bị bỏ, tag không phải chỗ trú")
	}
}
