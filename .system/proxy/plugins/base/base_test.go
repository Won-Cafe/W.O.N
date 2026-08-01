// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package base

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"won/proxy/core/plugin"
	"won/proxy/core/session"
)

type opts struct {
	MinTurns int `json:"min_turns"`
}

type countingLLM struct {
	calls int
	reply string
}

func (c *countingLLM) Chat(context.Context, string, string) (string, error) {
	c.calls++
	return c.reply, nil
}

// Cùng một câu hỏi thì không hỏi lại — lời đáp không thể mới. Đo trên một phiên
// thật: giữa vòng tool, cửa sổ hội thoại đứng yên và Loiterer bị hỏi đúng một câu
// 14 lần liên tiếp, mỗi lần trả bằng 2–3 giây nằm trên đường đi của đệ.
func TestAskOncePerQuestionPerSession(t *testing.T) {
	st := session.NewStore(filepath.Join(t.TempDir(), "state.json"))
	sess := st.Touch("k", "", "Tzu", session.Reach{}, 1, time.Now())
	llm := &countingLLM{reply: "🚶 Loiterer: kìa"}
	b := Base{Name: "loiterer"}

	for i := 0; i < 5; i++ {
		out, err := b.Ask(context.Background(), sess, llm, "soul", "cùng một lời hỏi")
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 && out == "" {
			t.Fatal("lần đầu phải hỏi thật")
		}
		if i > 0 && out != "" {
			t.Errorf("hỏi lại câu cũ phải im, got %q", out)
		}
	}
	if llm.calls != 1 {
		t.Errorf("năm lượt cùng một câu → một lần gọi, got %d", llm.calls)
	}

	// Lời hỏi đổi thì là câu hỏi khác — mỏ neo nằm trong lời hỏi.
	if _, err := b.Ask(context.Background(), sess, llm, "soul", "lời hỏi khác"); err != nil {
		t.Fatal(err)
	}
	if llm.calls != 2 {
		t.Errorf("câu hỏi mới phải được hỏi, got %d lượt gọi", llm.calls)
	}

	// Soul đổi mà lời hỏi không đổi thì vẫn là câu hỏi cũ: khuôn mặt Loiterer xoay
	// mỗi lượt, đổi áo không phải hỏi câu khác.
	if _, err := b.Ask(context.Background(), sess, llm, "soul KHÁC", "cùng một lời hỏi"); err != nil {
		t.Fatal(err)
	}
	if llm.calls != 2 {
		t.Errorf("đổi soul không mở lại một câu đã hỏi, got %d", llm.calls)
	}

	// Sổ theo từng plugin: hai nghề hỏi cùng một câu là hai câu hỏi.
	other := Base{Name: "outfitter"}
	if _, err := other.Ask(context.Background(), sess, llm, "soul", "cùng một lời hỏi"); err != nil {
		t.Fatal(err)
	}
	if llm.calls != 3 {
		t.Errorf("mỗi plugin một sổ riêng, got %d", llm.calls)
	}
}

// Khoá lạ trong options là lỗi rõ, không nuốt im lặng: nuốt thì người khai tưởng
// núm đã xoay, mà plugin đang chạy bằng mặc định. Cùng một luật với khoá lạ ở
// tầng core (config.parseConf).
func TestParseOptionsRejectsUnknownKey(t *testing.T) {
	var o opts
	err := New(plugin.Env{Options: []byte(`{"min_turnss":5}`)}).ParseOptions(&o)
	if err == nil {
		t.Fatal("khoá lạ phải là lỗi")
	}
	if !strings.Contains(err.Error(), "min_turnss") {
		t.Errorf("lỗi phải gọi tên khoá sai: %v", err)
	}
	if o.MinTurns != 0 {
		t.Errorf("không được nhận nửa vời, got %d", o.MinTurns)
	}
}

func TestParseOptionsAcceptsKnownKeyAndEmpty(t *testing.T) {
	var o opts
	if err := New(plugin.Env{Options: []byte(`{"min_turns":5}`)}).ParseOptions(&o); err != nil {
		t.Fatal(err)
	}
	if o.MinTurns != 5 {
		t.Errorf("muốn 5, got %d", o.MinTurns)
	}
	// Không khai options → không chạm gì, không lỗi.
	var empty opts
	if err := New(plugin.Env{}).ParseOptions(&empty); err != nil || empty.MinTurns != 0 {
		t.Errorf("options rỗng phải là no-op: %v %d", err, empty.MinTurns)
	}
}

// Marker do LÕI đeo, không do model. Bất biến #4 nói marker là căn cước, và căn cước
// không được đứng trên việc model có chịu tuân khuôn hay không.
func TestSayStampsTheMarker(t *testing.T) {
	got := Say("🧰", "Outfitter", "với `grep` cho chỗ này, ba lượt rồi bạn mò bằng mắt")
	want := "🧰 Outfitter: với `grep` cho chỗ này, ba lượt rồi bạn mò bằng mắt"
	if got != want {
		t.Errorf("Say = %q, muốn %q", got, want)
	}
}

// Ca im lặng: `∅`, rỗng, hoặc chỉ có khoảng trắng.
func TestSaySilence(t *testing.T) {
	for _, in := range []string{"∅", "  ∅  ", "∅ (nothing to add)", "", "   \n\n  "} {
		if got := Say("🚶", "Loiterer", in); got != "" {
			t.Errorf("Say(%q) = %q, muốn im lặng", in, got)
		}
	}
}

// Lời ĐÚNG nhưng thiếu marker: trước đây bị bỏ trọn — lời tốt mất vì cái vỏ. Giờ lõi
// dán vỏ vào. Đây là ca chính mà đổi thiết kế để cứu.
func TestSayRescuesUnmarkedLine(t *testing.T) {
	if got := Say("🛣️", "Wayfarer", "Đây là lượt thứ 40 trong phiên mở 3 giờ trước."); got == "" {
		t.Error("lời không marker phải được đeo marker, không bị bỏ")
	}
}

// Model tự đeo marker/tên thì không đeo lần hai — cả dạng trần lẫn dạng có khuôn mặt.
func TestSayDoesNotDoubleStamp(t *testing.T) {
	cases := map[string]string{
		"🧰 Outfitter: đặt `write` xuống": "🧰 Outfitter: đặt `write` xuống",
		"Outfitter: đặt `write` xuống":   "🧰 Outfitter: đặt `write` xuống",
		`🧰 Outfitter: "đặt xuống"`:       "🧰 Outfitter: đặt xuống",
	}
	for in, want := range cases {
		if got := Say("🧰", "Outfitter", in); got != want {
			t.Errorf("Say(%q) = %q, muốn %q", in, got, want)
		}
	}
	// Khuôn mặt nằm trong tên do plugin dựng; model đeo lại thì vẫn bóc đúng.
	got := Say("🚶", "Loiterer (Anh xe ôm)", "🚶 Loiterer (Anh xe ôm): hắn lại bám Notion rồi")
	if got != "🚶 Loiterer (Anh xe ôm): hắn lại bám Notion rồi" {
		t.Errorf("tên có khuôn mặt bị đeo hai lần: %q", got)
	}
}

// Dấu hai chấm của người viết KHÔNG phải dấu tên: bóc nó là cắt mất nửa câu.
func TestSayKeepsOrdinaryColon(t *testing.T) {
	in := "ba lượt liền chỉ đọc, chưa ghi gì: `grep` sẽ nhanh hơn mắt"
	if got := Say("🧰", "Outfitter", in); got != "🧰 Outfitter: "+in {
		t.Errorf("dấu hai chấm giữa câu bị hiểu thành dấu tên: %q", got)
	}
}

// Khối suy nghĩ đứng trước lời nói — chỉ đọc phần sau khối đó.
func TestSaySkipsThinkingBlock(t *testing.T) {
	out := "```\nđể xem, tay này đang đọc nhiều...\n```\nđặt `write` xuống đã"
	if got := Say("🧰", "Outfitter", out); got != "🧰 Outfitter: đặt `write` xuống đã" {
		t.Errorf("khối suy nghĩ lọt vào lời: %q", got)
	}
}

// Một fence ở CUỐI không phải khối suy nghĩ. Luật cũ lấy `LastIndex` nên nó ném sạch câu
// trả lời nằm trước fence và chỉ giữ ruột fence — đo trên một lượt thật: trọn lời của
// Outfitter mất, nhật ký ghi `silent`, và nhìn từ ngoài y như soul chọn im.
func TestSayKeepsTheLineWhenAFenceComesLast(t *testing.T) {
	out := "Bạn vác `write` cả phiên mà chưa với tới nó lần nào.\n\nSửa:\n```\nVệt"
	want := "🧰 Outfitter: Bạn vác `write` cả phiên mà chưa với tới nó lần nào."
	if got := Say("🧰", "Outfitter", out); got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// Model nào tự đeo marker thì dòng ấy là dòng nó chủ ý nói — lấy đúng dòng ấy, không
// lấy dòng lảm nhảm đứng trước.
func TestSayPrefersTheMarkedLine(t *testing.T) {
	out := "Here is my line:\n🧰 Outfitter: `grep` cho chỗ này"
	if got := Say("🧰", "Outfitter", out); got != "🧰 Outfitter: `grep` cho chỗ này" {
		t.Errorf("không ưu tiên dòng có marker: %q", got)
	}
}

// Một TIÊU ĐỀ không phải một lời. Model nhỏ chép khuôn của đệ dòng chính nên nó trả về
// tiêu đề rồi mới tới thân; luật cũ "lấy dòng có chữ đầu tiên" lấy đúng cái tiêu đề và
// bỏ mất thân. Hai hình dưới đây là hai lượt THẬT đã đo được.
func TestSayTakesTheSentenceNotTheHeading(t *testing.T) {
	for _, tc := range []struct {
		name, out, want string
	}{{
		// Model đeo đúng marker, nhưng đeo lên một tiêu đề.
		"marker trên tiêu đề",
		"🛣️ **Đường**\n\nBạn đã chạm ba thứ, thêm nữa là bịa.",
		"🛣️ Wayfarer: Bạn đã chạm ba thứ, thêm nữa là bịa.",
	}, {
		// Model chép marker của đệ khác (`🤔` là nhãn của Tzu) lên một tiêu đề.
		"marker của người khác trên tiêu đề",
		"🤔 **Hiểu**\n\nVắng 1 giờ 21 phút trước phiên này.",
		"🛣️ Wayfarer: Vắng 1 giờ 21 phút trước phiên này.",
	}, {
		"tiêu đề Markdown",
		"## Đường\nPhiên mở ba phút, chưa có mốc nào.",
		"🛣️ Wayfarer: Phiên mở ba phút, chưa có mốc nào.",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Say("🛣️", "Wayfarer", tc.out); got != tc.want {
				t.Errorf("got  %q\nwant %q", got, tc.want)
			}
		})
	}

	// Toàn bộ đầu ra chỉ là nhãn → không có lời nào → im. Thà im hơn gửi đi một tiêu đề.
	if got := Say("🛣️", "Wayfarer", "🛣️ **Đường**"); got != "" {
		t.Errorf("chỉ có nhãn mà vẫn nói: %q", got)
	}
}

// Dòng gửi đi chỉ được mang MỘT dấu. Model nhỏ chép marker của đệ dòng chính (`🤔` là
// nhãn của Tzu) vào đầu câu; để nguyên thì lời chèn thành hai dấu chồng nhau.
func TestSayDropsAForeignMarker(t *testing.T) {
	out := "🤔 Hiểu — bạn hỏi Memory Tzu đang sống gì, không phải \"có\" mà \"đang\"."
	got := Say("🛣️", "Wayfarer", out)
	want := "🛣️ Wayfarer: Hiểu — bạn hỏi Memory Tzu đang sống gì, không phải \"có\" mà \"đang\"."
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
	if strings.Contains(got, "🤔") {
		t.Errorf("dấu của đệ khác còn trong lời chèn: %q", got)
	}
}

// Sổ giữ CHỮ, request giữ chữ CÓ DẤU. Ghi cả dấu vào sổ thì lượt sau model đọc
// `🛣️ Wayfarer: …` như tiền lệ của chính nó rồi đeo dấu lần nữa.
func TestLineIsTheLedgerFormAndSayIsTheWireForm(t *testing.T) {
	out := "🛣️ Vắng 4 giờ trước phiên này."
	content := Line("🛣️", "Wayfarer", out)
	if content != "Vắng 4 giờ trước phiên này." {
		t.Errorf("dạng vào sổ còn dấu: %q", content)
	}
	if got := Say("🛣️", "Wayfarer", out); got != Stamp("🛣️", "Wayfarer", content) {
		t.Errorf("hai dạng lệch nhau: %q vs %q", got, Stamp("🛣️", "Wayfarer", content))
	}
	if Line("🛣️", "Wayfarer", Silent) != "" || Stamp("🛣️", "Wayfarer", "") != "" {
		t.Error("im lặng phải ra rỗng ở cả hai dạng")
	}
}

// Trần chữ ở base, không ở từng plugin.
func TestSayCapsLength(t *testing.T) {
	long := strings.Repeat("dài ", lineCap)
	got := Say("🛣️", "Wayfarer", long)
	if len([]rune(got)) > lineCap+len("🛣️ Wayfarer: ")+3 {
		t.Errorf("dòng không bị cắt ở trần: %d rune", len([]rune(got)))
	}
}

// Hợp đồng KHÔNG còn đòi marker: khuôn nào đã bỏ khỏi prompt thì không còn là chỗ
// model lạc được. Nó vẫn phải nói rõ hai dạng và `∅`.
func TestContractDropsMarkerShape(t *testing.T) {
	c := Contract("Outfitter", "🧰", "im lặng.", "cửa cuối")
	if strings.Contains(c, "🧰") {
		t.Errorf("hợp đồng không được đòi marker nữa:\n%s", c)
	}
	for _, want := range []string{Silent, "one of two forms", "the line itself", "FINAL GATE"} {
		if !strings.Contains(c, want) {
			t.Errorf("hợp đồng thiếu %q:\n%s", want, c)
		}
	}
}
