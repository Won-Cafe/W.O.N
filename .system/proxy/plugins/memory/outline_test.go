// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package memory

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"won/proxy/core/paths"
	"won/proxy/core/plugin"
)

func jsonMarshal(v any) ([]byte, error) { return json.Marshal(v) }

func pluginEnv(root string, opts json.RawMessage) plugin.Env {
	return plugin.Env{Paths: paths.Tree{Root: root}, Services: plugin.NewHub(), Options: opts, Control: "127.0.0.1:7777"}
}

// newMemOpt — plugin với options khai tay; newMem luôn tắt tóm tắt nên không dùng được
// cho nhánh này. Giá trị theo đúng kiểu won.conf giao xuống (`typedValue`: toàn số →
// int, còn lại → chuỗi), nên số phải là số ở đây — khai chuỗi là đo một đường không có.
func newMemOpt(t *testing.T, root string, llm chatter, opts map[string]any) *Memory {
	t.Helper()
	raw, err := jsonMarshal(opts)
	if err != nil {
		t.Fatal(err)
	}
	p, err := New(pluginEnv(root, raw))
	if err != nil {
		t.Fatal(err)
	}
	m := p.(*Memory)
	m.llm = llm
	return m
}

// Trang nhiều mục → dàn ý là các dòng heading, và ruột KHÔNG đi theo.
func TestOutlineTakesHeadings(t *testing.T) {
	page := "# Focus — scoring\n\nMở bài.\n\n## Sỏi tuyến tính\n\nRuột A.\n\n### Cổng /update\n\nRuột B.\n"
	got := outline(page)
	for _, want := range []string{"# Focus — scoring", "## Sỏi tuyến tính", "### Cổng /update"} {
		if !strings.Contains(got, want) {
			t.Errorf("dàn ý thiếu %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Ruột A") || strings.Contains(got, "Mở bài") {
		t.Errorf("dàn ý không được chở ruột:\n%s", got)
	}
}

// Một heading thì dàn ý bằng đúng dòng đã có ở index — không thêm gì cho ai. Lúc ấy
// cắt phần đầu: chữ thật cắt ngang câu vẫn hơn một bản kê rỗng.
func TestOutlineFallsBackWhenOneHeading(t *testing.T) {
	if got := outline(focusPage); !strings.Contains(got, "Ruột trang focus") {
		t.Errorf("một heading thì phải cắt phần đầu, got:\n%s", got)
	}
}

// `# ` trong khối mã là chú thích của một ngôn ngữ khác, không phải mục của trang.
func TestOutlineSkipsFencedCode(t *testing.T) {
	page := "# Trang\n\n```sh\n# đây là chú thích shell\n```\n\n## Mục thật\n\nRuột.\n"
	got := outline(page)
	if strings.Contains(got, "chú thích shell") {
		t.Errorf("heading giả trong khối mã lọt vào dàn ý:\n%s", got)
	}
	if !strings.Contains(got, "## Mục thật") {
		t.Errorf("mất mục thật:\n%s", got)
	}
}

// Bộ chọn chỉ đọc INDEX. Chở ruột mọi ứng viên lên cho model là một prompt phình theo cả
// kho, mỗi lượt người một lần — test này là hàng rào cho việc đó.
func TestPickerPromptCarriesNoPageBodies(t *testing.T) {
	root := writeStore(t, map[string]string{"working/a.md": focusPage, "personal/self.md": selfPage})
	llm := &fakeLLM{reply: "working/a.md"}
	c, err := newMem(t, root, llm).give(context.Background(), snap(), turnSession(t, 2))
	if err != nil {
		t.Fatal(err)
	}
	if llm.calls != 1 {
		t.Errorf("phải đúng 1 lượt gọi, got %d", llm.calls)
	}
	for _, body := range []string{"Ruột trang focus", "Ruột trang self"} {
		if strings.Contains(llm.user, body) {
			t.Errorf("ruột trang lọt vào prompt của bộ chọn: %q\n%s", body, llm.user)
		}
	}
	// Index thì phải có — đó là toàn bộ thứ bộ chọn được đọc.
	if !strings.Contains(llm.system, "- working/a.md") {
		t.Errorf("bộ chọn không thấy index:\n%s", llm.system)
	}
	if !strings.Contains(c.Usr, "working/a.md") {
		t.Errorf("trang được chọn phải mở ra:\n%s", c.Usr)
	}
}

// Một hợp đồng, không hai: một hình đòi tóm tắt thì phải bỏ câu "never explain", và hai
// lệnh chọi nhau trong một lời dặn là chỗ model nhỏ chọn nhầm cái để nghe.
func TestSelectorContractHasOneShape(t *testing.T) {
	root := writeStore(t, map[string]string{"working/a.md": focusPage, "personal/self.md": selfPage})
	llm := &fakeLLM{reply: "∅"}
	newMem(t, root, llm).Contribute(context.Background(), snap(), turnSession(t, 2))
	for _, want := range []string{"comma separated", "Never explain", "at most two"} {
		if !strings.Contains(llm.system, want) {
			t.Errorf("hợp đồng thiếu %q:\n%s", want, llm.system)
		}
	}
	if strings.Contains(llm.system, "the path alone on its line") {
		t.Errorf("hợp đồng còn dấu vết đường tóm tắt:\n%s", llm.system)
	}
}
