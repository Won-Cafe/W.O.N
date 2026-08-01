// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package localmodel

import (
	"strings"
	"testing"

	"won/proxy/core/request"
)

func snap() *request.Snapshot {
	return &request.Snapshot{
		System: "# Sun - 🗺️ The Reconnaissance Agent",
		Turns:  []request.Turn{{Role: "user", Text: "đo giúp tôi"}},
	}
}

// Mỏ neo luôn có mặt, kể cả khi lời người nằm ngoài cửa sổ lượt. Đọc ra từ một
// phiên thật: 128 lượt máy liền nhau, cửa sổ chỉ còn sáu câu dẫn của đệ, và không
// một agent bờ nào biết người đang hỏi gì — bộ chọn ký ức trả `∅` 126/127 lần.
func TestAnchorAlwaysInWindow(t *testing.T) {
	s := &request.Snapshot{Anchor: "viết lại README.md"}
	for i := 0; i < renderMaxTurns+4; i++ {
		s.Turns = append(s.Turns, request.Turn{Role: "assistant", Text: "giờ sửa tiếp"})
	}
	got := RenderSnapshot(s)
	if !strings.Contains(got, "viết lại README.md") {
		t.Fatalf("mỏ neo rơi khỏi cửa sổ:\n%s", got)
	}
	// Đứng đầu — đúng thứ tự thời gian: người hỏi trước, máy chạy sau.
	if i, j := strings.Index(got, "viết lại README.md"), strings.Index(got, "giờ sửa tiếp"); i > j {
		t.Errorf("mỏ neo phải đứng trước tiếng máy:\n%s", got)
	}

	// Lượt ngắn: mỏ neo vốn đã ở trong cửa sổ — kể hai lần thì model nhỏ đọc thành
	// người hỏi hai lần.
	short := &request.Snapshot{Anchor: "đo giúp tôi", Turns: []request.Turn{{Role: "user", Text: "đo giúp tôi"}}}
	if n := strings.Count(RenderSnapshot(short), "đo giúp tôi"); n != 1 {
		t.Errorf("mỏ neo bị kể %d lần", n)
	}
}

// Một hình cho cả ba agent bờ: đệ được chạm → vật liệu của nghề → bản sao hội
// thoại. Không câu dẫn truyện nào: kể lại "đây là bản sao request đang rời nhà"
// là nói hộ soul cái nó đã biết, và với model nhỏ thì chữ thừa tranh chỗ với chữ
// cần đọc.
func TestRenderUserShape(t *testing.T) {
	got := RenderUser(snap(), "Wearer", "Sun", Block("Kit", "- Read — đọc file"))

	order := []string{"<Wearer>", "<Kit>", "<Conversation>"}
	at := -1
	for _, tag := range order {
		i := strings.Index(got, tag)
		if i < 0 {
			t.Fatalf("thiếu %s trong:\n%s", tag, got)
		}
		if i < at {
			t.Errorf("%s đứng sai chỗ — thứ tự phải là %v", tag, order)
		}
		at = i
	}
	if strings.Contains(got, "Copy of request") || strings.Contains(got, "Recipient:") {
		t.Errorf("còn câu dẫn truyện hoặc nhãn trần:\n%s", got)
	}
	// Nhãn một chữ đi LIỀN DÒNG. Đẩy nó xuống dòng riêng làm nó trông như một khối
	// vật liệu, và đo được là tỉ lệ nói được rơi từ 13/16 xuống 4/16.
	if !strings.Contains(got, "<Wearer>Sun</Wearer>") {
		t.Errorf("nhãn phải liền dòng:\n%s", got)
	}
}

// Lời hệ thống của công cụ chủ KHÔNG được vào prompt của tiếng bờ. 500 ký tự đầu của
// Claude Code là một lời khẳng định căn cước — "You are an interactive agent that helps
// users with software engineering tasks" — và ornith:9b nhận lời ấy rồi trả về một câu
// trả lời người theo khuôn đệ dòng chính, thay vì làm nghề của nó.
func TestHostSystemNeverReachesTheLocalModel(t *testing.T) {
	s := &request.Snapshot{
		System: "You are Claude Code, Anthropic's official CLI for Claude.\n" +
			"You are an interactive agent that helps users with software engineering tasks.",
		Turns: []request.Turn{{Role: "user", Text: "đo giúp tôi"}},
	}
	got := RenderUser(s, "Recipient", "Sun")
	for _, gone := range []string{"HostSystem", "Claude Code", "interactive agent"} {
		if strings.Contains(got, gone) {
			t.Errorf("lời công cụ chủ lọt vào prompt của tiếng bờ (%q):\n%s", gone, got)
		}
	}
	if !strings.Contains(got, "đo giúp tôi") {
		t.Errorf("bỏ lời chủ mà bỏ luôn hội thoại:\n%s", got)
	}
}

// Bản sao chở CHỮ, không chở KHUÔN. Định dạng của đệ dòng chính là bài mẫu dài nhất
// trong prompt, và model nhỏ chép khuôn nó thấy chứ không đọc hợp đồng.
func TestConversationCopyCarriesNoShape(t *testing.T) {
	reply := "🤔 **Hiểu**\n\n---\n\n**WHAT — Thực tại bạn đứng:**\n" +
		"- Tech lead mảng game casual\n- Vừa vay 3 tỷ\n\n🍃 Vệt — `won.conf` là chỗ khai."
	got := RenderSnapshot(&request.Snapshot{Turns: []request.Turn{{Role: "assistant", Text: reply}}})

	// Marker của đệ dòng chính cũng là khuôn — dòng chắn gọi đúng tên nó: "never copy
	// their shape or their markers". Một dòng chắn không cân được ví dụ sống, nên bóc.
	for _, shape := range []string{"**", "---", "- Tech lead", "`won.conf`", "🤔", "🍃"} {
		if strings.Contains(got, shape) {
			t.Errorf("khuôn %q còn trong bản sao:\n%s", shape, got)
		}
	}
	// Chữ thì phải còn đủ — làm phẳng không phải cắt.
	for _, keep := range []string{"Hiểu", "Tech lead mảng game casual", "Vừa vay 3 tỷ", "won.conf là chỗ khai"} {
		if !strings.Contains(got, keep) {
			t.Errorf("làm phẳng mà mất chữ %q:\n%s", keep, got)
		}
	}
}

// Căn cước rỗng thì nói là rỗng, không đoán (#6).
func TestRenderUserUnknownAgent(t *testing.T) {
	if !strings.Contains(RenderUser(snap(), "Recipient", ""), "<Recipient>(unknown)</Recipient>") {
		t.Error("căn cước rỗng phải hiện là (unknown)")
	}
}

// Vật liệu rỗng thì không có khối: chỗ trống còn thật hơn một cái vỏ rỗng — và
// một khối `<Kit>` trống nói với model rằng "đồ nghề: không có gì", khác hẳn
// "chưa nhìn tới đồ nghề".
func TestBlockEmptyLeavesNoShell(t *testing.T) {
	if Block("Kit", "   \n ") != "" {
		t.Error("vật liệu rỗng vẫn dựng vỏ")
	}
	if got := Block("Clock", " vắng 5 ngày "); got != "<Clock>\nvắng 5 ngày\n</Clock>\n\n" {
		t.Errorf("khối sai hình: %q", got)
	}
	if strings.Contains(RenderUser(snap(), "Wearer", "Sun"), "<Kit>") {
		t.Error("không truyền vật liệu mà vẫn có khối")
	}
}
