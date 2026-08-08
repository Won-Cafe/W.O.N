// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package request

import (
	"strings"
	"testing"
)

var identityRules = FrameRules{Identity: []string{"You are "}}

// Gỡ đúng CÂU. Phần còn lại của cùng dòng, và cả sách hướng dẫn, phải nguyên vẹn.
func TestDropIdentityRemovesOnlyTheClaimSentence(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{
			"câu khẳng định vai rồi câu vận hành, cùng đoạn",
			"You are Gemini CLI, an interactive CLI agent. You are currently operating in **Default** mode. Your primary goal is to help users.",
			"Your primary goal is to help users.",
		},
		{
			"khối chỉ có đúng câu ấy thì ra rỗng, bên gọi bỏ khối",
			"You are an interactive agent that helps users with software engineering tasks.",
			"",
		},
		{
			"khớp ở đầu DÒNG, không chỉ đầu khối",
			"## Who are you\nYou are Grok, built by xAI.\n\n# Core Mandates\ntuân thủ",
			"## Who are you\n\n\n# Core Mandates\ntuân thủ",
		},
		{
			"giữa câu thì KHÔNG khớp — đó là chữ bàn về vai, không phải gán vai",
			"Never tell the user what You are running on.",
			"Never tell the user what You are running on.",
		},
		{
			"không có dấu chấm thì hết dòng là hết câu",
			"You are a helpful assistant\n# Tool Usage\ndùng replace",
			"# Tool Usage\ndùng replace",
		},
	}
	for _, c := range cases {
		if got := identityRules.dropIdentity(c.in); got != c.want {
			t.Errorf("%s:\n  got  %q\n  want %q", c.name, got, c.want)
		}
	}
}

// Sách hướng dẫn phải sống qua việc gỡ, bất kể công cụ chủ gói nó thành mấy khối và phần đầu
// khối dài bao nhiêu. Hai hình dưới là hai đầu của phổ đo được trên 62 lời hệ thống thật:
// phần đầu 197 ký tự, và phần đầu 18.513 ký tự.
func TestDropIdentitySizeIsIndependentOfHostShape(t *testing.T) {
	body := strings.Repeat("hướng dẫn thật cần giữ. ", 800)
	for _, pre := range []string{
		"You are Gemini CLI, an interactive CLI agent.",
		"You are Kimi.\n\n" + strings.Repeat("phần đầu dài mà vẫn là lời dạy. ", 500),
	} {
		in := pre + "\n\n# Core Mandates\n" + body
		got := identityRules.dropIdentity(in)
		if strings.Contains(got, "You are ") {
			t.Error("còn sót lời khẳng định vai")
		}
		if !strings.Contains(got, "# Core Mandates") || !strings.Contains(got, "hướng dẫn thật cần giữ") {
			t.Error("sách hướng dẫn bị mất")
		}
		// Mất đúng một câu, không hơn.
		if lost := len(in) - len(got); lost > 160 {
			t.Errorf("gỡ %d byte, một câu khẳng định vai chỉ tới ~158", lost)
		}
	}
}

// Cắt theo mục và bỏ vỏ tag vẫn chạy sau khi gỡ câu — ba luật đứng cạnh nhau, không đè nhau.
func TestCleanSystemCombinesRules(t *testing.T) {
	rules := FrameRules{
		Identity: []string{"You are "},
		Sections: []string{"Tone and style"},
		Strip:    []string{"system-reminder"},
	}
	in := "You are Claude Code.\n\n# Using your tools\ngiữ\n\n# Tone and style\ncắt\n\n" +
		"<system-reminder>bỏ</system-reminder>"
	got := rules.cleanSystem(in)
	for _, gone := range []string{"You are Claude Code", "Tone and style", "system-reminder"} {
		if strings.Contains(got, gone) {
			t.Errorf("còn sót %q:\n%s", gone, got)
		}
	}
	if !strings.Contains(got, "Using your tools") {
		t.Errorf("mục dạy dùng đồ nghề phải giữ:\n%s", got)
	}
}

// Khai rỗng thì không gỡ gì — rỗng là một trạng thái hợp lệ.
func TestDropIdentityNoRulesNoChange(t *testing.T) {
	in := "You are Gemini CLI. # Core Mandates"
	if got := (FrameRules{}).dropIdentity(in); got != in {
		t.Errorf("không khai gì mà vẫn gỡ: %q", got)
	}
}
