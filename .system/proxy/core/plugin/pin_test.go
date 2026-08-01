// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package plugin

import (
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"won/proxy/core/request"
	"won/proxy/core/session"
)

// turn dựng một lần chạy: parse thân công cụ chủ gửi lên, Apply với sổ ghim của phiên, trả
// về mảng message đi ra dưới dạng chữ đọc được.
func turn(t *testing.T, sess *session.Session, host string, cs ...Contribution) []string {
	t.Helper()
	body, err := request.ParseBody([]byte(host), request.FormatOpenAI)
	if err != nil {
		t.Fatal(err)
	}
	Apply(body, cs, sess, speaking(cs))

	out, err := body.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(out, &f); err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(f.Messages))
	for i, m := range f.Messages {
		got[i] = m.Role + ":" + m.Content
	}
	return got
}

// appended — b có phải phép NỐI của a không, và chỗ đầu tiên lệch nếu không phải.
func appended(a, b []string) (bool, int) {
	for i := range a {
		if i >= len(b) || a[i] != b[i] {
			return false, i
		}
	}
	return true, len(a)
}

func loiterer(line string) Contribution {
	return Contribution{Plugin: "loiterer", Kind: KindMarker, Tag: "Loiterer", Text: "🚶 loiterer: " + line}
}

// Lỗi bắt được trên nhật ký thật: công cụ chủ gửi lại lịch sử nguyên bản mỗi lượt, còn
// thân đi ra thì mất khối đã chèn ở lượt trước — đệ trả lời trong ngữ cảnh có khối ấy rồi
// lời đáp bị khâu thẳng vào lượt người. Ghim rồi thì lượt sau phải là phép NỐI của lượt trước.
func TestPinnedBlocksMakeEachTurnAnAppend(t *testing.T) {
	sess := freshSession()

	one := turn(t, sess, `{"messages":[{"role":"user","content":"hỏi 1"}]}`, loiterer("kìa"))
	two := turn(t, sess, `{"messages":[
		{"role":"user","content":"hỏi 1"},
		{"role":"assistant","content":"đáp 1"},
		{"role":"user","content":"hỏi 2"}]}`, loiterer("lại kìa"))

	ok, at := appended(one, two)
	if !ok {
		t.Fatalf("lượt 2 phải là phép nối của lượt 1, lệch ở [%d]:\n  lượt 1: %q\n  lượt 2: %q", at, one, two)
	}
	// Khối của lượt 1 phải nằm ĐÚNG chỗ cũ — ngay sau lượt người đầu, không trôi xuống cuối.
	if len(two) != 5 || !strings.Contains(two[1], "kìa") || two[2] != "assistant:đáp 1" {
		t.Fatalf("khối lượt 1 không về đúng chỗ:\n%q", two)
	}
	if !strings.Contains(two[4], "lại kìa") {
		t.Errorf("khối của lượt này phải ở cuối: %q", two)
	}
}

// Giữa vòng tool, không có khối MỚI — nhưng khối cũ vẫn phải về, không thì lần chạy trong
// vòng tool lệch với lần chạy mở lượt ngay tại chỗ khối ấy đứng.
func TestPinnedBlocksReturnInsideToolLoop(t *testing.T) {
	sess := freshSession()

	one := turn(t, sess, `{"messages":[{"role":"user","content":"hỏi"}]}`, loiterer("kìa"))
	loop := turn(t, sess, `{"messages":[
		{"role":"user","content":"hỏi"},
		{"role":"assistant","content":"gọi tool"},
		{"role":"tool","content":"kết quả"}]}`, loiterer("không được nói"))

	if ok, at := appended(one, loop); !ok {
		t.Fatalf("lần chạy trong vòng tool phải nối tiếp lần mở lượt, lệch ở [%d]:\n  %q\n  %q", at, one, loop)
	}
	if strings.Contains(strings.Join(loop, "|"), "không được nói") {
		t.Errorf("giữa vòng tool không được chèn khối mới: %q", loop)
	}
}

// Công cụ chủ viết lại lịch sử (nén phiên, sửa lời cũ) thì message mang mỏ neo biến mất,
// và khối bám vào nó hết chỗ đúng để về. Bỏ khối ấy là cách hỏng đúng (#2).
func TestPinnedBlocksDroppedWhenHostRewritesHistory(t *testing.T) {
	sess := freshSession()

	turn(t, sess, `{"messages":[{"role":"user","content":"hỏi 1"}]}`, loiterer("kìa"))
	after := turn(t, sess, `{"messages":[
		{"role":"user","content":"tóm tắt phiên trước"},
		{"role":"user","content":"hỏi 2"}]}`)

	if strings.Contains(strings.Join(after, "|"), "kìa") {
		t.Errorf("lịch sử đã bị viết lại mà khối cũ vẫn về: %q", after)
	}
	if len(after) != 2 {
		t.Errorf("không còn sổ thì thân đi ra đúng bằng thân nhận vào: %q", after)
	}
}

// Cú gãy bắt được khi chạy thật: công cụ chủ thêm một mục vào lời hệ thống của nó, mà
// đường OpenAI chở lời hệ thống trong chính mảng messages — mọi chỉ số trượt một nấc, và
// ghim theo số đếm thì cả sổ rụng sạch.
func TestPinnedBlocksSurviveAShiftAtTheHead(t *testing.T) {
	sess := freshSession()

	turn(t, sess, `{"messages":[{"role":"user","content":"hỏi 1"}]}`, loiterer("kìa"))
	// Lượt sau: một message system chen vào ĐẦU mảng, đẩy mọi thứ xuống một nấc.
	two := turn(t, sess, `{"messages":[
		{"role":"system","content":"lời dạy công cụ chủ vừa thêm"},
		{"role":"user","content":"hỏi 1"},
		{"role":"assistant","content":"đáp 1"},
		{"role":"user","content":"hỏi 2"}]}`, loiterer("lại kìa"))

	if !strings.Contains(strings.Join(two, "|"), "kìa") {
		t.Fatalf("đầu mảng trượt một nấc là cả sổ ghim rụng: %q", two)
	}
	// Khối cũ phải bám đúng message cũ, tức đứng NGAY SAU "hỏi 1" ở chỗ mới của nó.
	if len(two) != 6 || two[1] != "user:hỏi 1" || !strings.Contains(two[2], "kìa") {
		t.Fatalf("khối cũ không bám đúng message của nó:\n%q", two)
	}
	if two[3] != "assistant:đáp 1" || !strings.Contains(two[5], "lại kìa") {
		t.Errorf("phần còn lại của mảng sai chỗ:\n%q", two)
	}
}

// Mất một vài mỏ neo là công cụ chủ vừa sửa lịch sử. Mất SẠCH thì câu đọc được khác hẳn:
// thân này không phải hội thoại ấy — một lượt việc nhà của công cụ chủ (đặt tiêu đề, tóm
// tắt) rơi trúng khoá phiên là đủ. Xoá sổ theo nó thì lượt THẬT kế tiếp hết đường tái dựng
// và thân đi ra thành bản SỬA, im lặng, không ai kêu.
func TestLedgerSurvivesABodyThatIsNotTheConversation(t *testing.T) {
	sess := freshSession()

	one := turn(t, sess, `{"messages":[{"role":"user","content":"hỏi 1"}]}`, loiterer("kìa"))
	// Một thân lạ đi qua cùng phiên: không mỏ neo nào của sổ có mặt trong nó.
	turn(t, sess, `{"messages":[{"role":"user","content":"đặt tiêu đề cho phiên"}]}`)
	two := turn(t, sess, `{"messages":[
		{"role":"user","content":"hỏi 1"},
		{"role":"assistant","content":"đáp 1"},
		{"role":"user","content":"hỏi 2"}]}`, loiterer("lại kìa"))

	if ok, at := appended(one, two); !ok {
		t.Fatalf("thân lạ đi qua đã xoá sổ ghim — lượt thật kế tiếp không còn là phép nối, lệch ở [%d]:\n  lượt 1: %q\n  lượt 2: %q", at, one, two)
	}
}

// Công tắc phải tắt được thật: plugin tắt giữa phiên thì chữ cũ của nó cũng thôi về, và
// vặn lại thì nó về đúng chỗ cũ — sổ chỉ bị BỎ QUA, không bị xoá.
func TestPinnedBlocksFollowTheToggle(t *testing.T) {
	sess := freshSession()
	host := `{"messages":[{"role":"user","content":"hỏi"}]}`

	if on := turn(t, sess, host, loiterer("kìa")); len(on) != 2 {
		t.Fatalf("lượt đầu phải có khối: %q", on)
	}
	// Không đóng góp và không plugin nào đang bật = plugin đã tắt.
	if off := turn(t, sess, host); len(off) != 1 {
		t.Errorf("plugin tắt mà chữ cũ vẫn về: %q", off)
	}
	back := turn(t, sess, host, loiterer("kìa lần nữa"))
	if len(back) != 3 {
		t.Errorf("vặn lại thì khối cũ phải về, got %q", back)
	}
}

// #4 chạy TRƯỚC khi ghim: một đóng góp vô danh không được vào sổ, không thì nó bị chặn một
// lần rồi tự về ở mọi lượt sau.
func TestAnonymousContributionNeverGetsPinned(t *testing.T) {
	slog.SetDefault(slog.New(slog.NewTextHandler(&strings.Builder{}, nil)))
	sess := freshSession()
	host := `{"messages":[{"role":"user","content":"hỏi"}]}`

	turn(t, sess, host, Contribution{Plugin: "rogue", Kind: KindMarker, Tag: "Loiterer", Text: "lời vô danh"})
	again := turn(t, sess, host, Contribution{Plugin: "rogue", Kind: KindMarker, Tag: "Loiterer", Text: "lời vô danh"})

	if strings.Contains(strings.Join(again, "|"), "lời vô danh") {
		t.Errorf("đóng góp vô danh lọt vào sổ ghim: %q", again)
	}
}

// Một lượt người có thể chạy hai lần với đúng cùng một thân: đích trả 502 hoặc client
// ngắt thì lần gửi lại y nguyên là một lần THỬ LẠI của chính cuộc ấy (§ Session). Cả hai
// lần đều `HumanSpokeLast`, nên cả hai lần đều đi qua cửa ghim.
func TestRetryOfTheSameTurnDoesNotPinTwice(t *testing.T) {
	sess := freshSession()
	const host = `{"messages":[{"role":"user","content":"câu của người"}]}`

	first := turn(t, sess, host, loiterer("một lời"))
	again := turn(t, sess, host, loiterer("một lời"))

	if len(again) != len(first) {
		t.Errorf("thử lại nhân đôi khối đã ghim: %d → %d\n%v", len(first), len(again), again)
	}
	if n := len(sess.Placed()); n != 1 {
		t.Errorf("sổ ghim phải giữ MỘT khối, got %d", n)
	}
}
