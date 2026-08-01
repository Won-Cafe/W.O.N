// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"won/proxy/core/request"
	"won/proxy/core/session"
)

type fake struct {
	name   string
	c      *Contribution
	err    error
	panics bool
}

func (f fake) Name() string { return f.name }
func (f fake) Contribute(context.Context, *request.Snapshot, *session.Session) ([]*Contribution, error) {
	if f.panics {
		panic("boom")
	}
	return One(f.c), f.err
}

// sluggish — plugin tự khai trần riêng rồi TREO trong chính code của nó, không hỏi model
// lần nào. Nó đo đúng chỗ trần riêng phải cắt: từ lúc plugin bắt đầu chạy.
type sluggish struct {
	fake
	budget time.Duration
	hang   time.Duration
	cut    *bool
}

func (s sluggish) Budget() time.Duration { return s.budget }
func (s sluggish) Contribute(ctx context.Context, _ *request.Snapshot, _ *session.Session) ([]*Contribution, error) {
	select {
	case <-time.After(s.hang):
		return nil, nil
	case <-ctx.Done():
		*s.cut = true
		return nil, ctx.Err()
	}
}

// Trần riêng của plugin tính từ lúc plugin CHẠY, không từ lúc nó hỏi model: một plugin
// treo trong code của chính nó phải bị cắt đúng ở con số nó khai. Đây là lý do trần ấy
// sống ở lõi (`plugin.Budgeted`) chứ không ở chỗ gọi model.
func TestOwnBudgetCutsFromWhenThePluginStarts(t *testing.T) {
	cut := false
	p := sluggish{fake: fake{name: "slow"}, budget: 30 * time.Millisecond, hang: 3 * time.Second, cut: &cut}

	start := time.Now()
	_, details := Gather(context.Background(), []Plugin{p}, nil, nil)
	elapsed := time.Since(start)

	if !cut {
		t.Fatal("plugin phải bị cắt bằng ctx, không phải chạy hết 3 giây")
	}
	if elapsed > time.Second {
		t.Errorf("cắt sau %v — trần riêng không ăn từ lúc plugin chạy", elapsed)
	}
	if len(details) != 1 || details[0].Status != StatusTimeout {
		t.Errorf("kết cục = %+v, want timeout", details)
	}
}

// voice — plugin tự khai mình chỉ nói ở lượt của người, và đếm số lần bị GỌI.
type voice struct {
	fake
	calls *int
}

func (v voice) SpeaksOnHumanTurn() bool { return true }
func (v voice) Contribute(ctx context.Context, s *request.Snapshot, sess *session.Session) ([]*Contribution, error) {
	*v.calls++
	return v.fake.Contribute(ctx, s, sess)
}

// Giữa vòng tool, tiếng của lượt KHÔNG ĐƯỢC GỌI — không phải gọi rồi bỏ lời đi. Chỗ
// tốn kém nằm bên trong Contribute: đo trên một phiên thật, một tiếng bên lề tiêu
// 3–7 giây model nền cho một lần chạy mà không ai đọc lời nó.
//
// Plugin không khai TurnVoice thì vẫn chạy: khối lời hệ thống phải đi ở mọi lần chạy
// vì nhà cung cấp không giữ phiên.
func TestTurnVoiceSkippedInToolLoop(t *testing.T) {
	var voiceCalls, plainCalls int
	plugins := []Plugin{
		voice{fake: fake{name: "voice", c: &Contribution{Kind: KindMarker, Text: "🚶 voice: kìa"}}, calls: &voiceCalls},
		countingSystem{name: "plain", calls: &plainCalls},
	}

	loop := &request.Snapshot{HumanSpokeLast: false}
	got, details := Gather(context.Background(), plugins, loop, nil)
	if voiceCalls != 0 {
		t.Errorf("giữa vòng tool mà tiếng của lượt bị gọi %d lần", voiceCalls)
	}
	if plainCalls != 1 {
		t.Errorf("plugin lời hệ thống phải chạy mọi lần: %d", plainCalls)
	}
	if len(got) != 1 || got[0].Plugin != "plain" {
		t.Errorf("chỉ khối hệ thống được chèn, got %+v", got)
	}
	if details[0].Status != StatusSkipped {
		t.Errorf("phải khai skipped để nhật ký đọc ra được, got %q", details[0].Status)
	}

	// Người vừa nói → gọi bình thường.
	if _, _ = Gather(context.Background(), plugins,
		&request.Snapshot{HumanSpokeLast: true}, nil); voiceCalls != 1 {
		t.Errorf("lượt của người phải gọi đúng một lần, got %d", voiceCalls)
	}
}

type countingSystem struct {
	name  string
	calls *int
}

func (c countingSystem) Name() string { return c.name }
func (c countingSystem) Contribute(context.Context, *request.Snapshot, *session.Session) ([]*Contribution, error) {
	*c.calls++
	return One(&Contribution{Kind: KindSystem, Tag: "Plain", Text: "khối đứng cả phiên"}), nil
}

// twoBeat — một plugin nói ở CẢ HAI nhịp: khối đứng cả phiên và khối của lượt.
type twoBeat struct{ name string }

func (t twoBeat) Name() string { return t.name }
func (t twoBeat) Contribute(context.Context, *request.Snapshot, *session.Session) ([]*Contribution, error) {
	return []*Contribution{
		{Kind: KindSystem, Tag: "Two", Text: "khối đứng cả phiên"},
		nil, // phần tử nil bị bỏ, không được thành một đóng góp rỗng
		{Kind: KindMarker, Tag: "Two", Text: "🛣️ " + t.name + ": tiếng của lượt"},
		{Kind: KindMarker, Tag: "Two", Text: ""}, // rỗng chữ cũng bị bỏ
	}, nil
}

// Một plugin nói hai nhịp: cả hai khối cùng ra, cùng mang tên nó, và Apply đặt mỗi khối
// vào đúng chỗ của nhịp ấy — system vào lời hệ thống, marker xuống cuối mảng.
func TestGatherKeepsBothKindsFromOnePlugin(t *testing.T) {
	got, details := Gather(context.Background(), []Plugin{twoBeat{name: "two"}},
		&request.Snapshot{HumanSpokeLast: true}, session.NewEphemeral().Touch("k", "", "Tzu", session.Reach{}, 1, time.Now()))

	if len(got) != 2 {
		t.Fatalf("muốn 2 đóng góp (nil và rỗng bị bỏ), got %d: %+v", len(got), got)
	}
	if got[0].Kind != KindSystem || got[1].Kind != KindMarker {
		t.Errorf("thứ tự nhịp sai: %v %v", got[0].Kind, got[1].Kind)
	}
	for i, c := range got {
		if c.Plugin != "two" {
			t.Errorf("khối %d không mang tên plugin: %q", i, c.Plugin)
		}
	}
	if details[0].Status != StatusContributed {
		t.Errorf("status = %q, muốn contributed", details[0].Status)
	}
	if details[0].Kind != "system+marker" {
		t.Errorf("nhật ký phải kể cả hai nhịp, got %q", details[0].Kind)
	}
}

// Biên fail-open: panic, lỗi đều quy về im lặng; kết quả giữ thứ tự.
func TestGatherFailOpen(t *testing.T) {
	var buf strings.Builder
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	plugins := []Plugin{
		fake{name: "boom", panics: true},
		fake{name: "err", err: errors.New("broken")},
		fake{name: "im", c: nil},
		fake{name: "speak", c: &Contribution{Kind: KindMarker, Text: "🛣️ speak: milestone"}},
	}
	got, _ := Gather(context.Background(), plugins, nil, nil)
	if len(got) != 1 || got[0].Plugin != "speak" {
		t.Fatalf("want exactly one contribution from 'speak', got %+v", got)
	}
	logged := buf.String()
	if !strings.Contains(logged, "boom") || !strings.Contains(logged, "err") {
		t.Fatalf("panic and error must log: %s", logged)
	}
}

// freshSession — phiên trắng cho một lần Apply. Sổ ghim sống ở phiên, nên hai lần Apply
// dùng CHUNG một phiên là hai lượt liên tiếp, còn dùng phiên riêng là hai phiên rời.
func freshSession() *session.Session {
	return session.NewEphemeral().Touch("k", "", "Tzu", session.Reach{}, 1, time.Now())
}

// speaking — danh sách plugin đang bật, dựng từ chính các đóng góp của lượt. Apply lọc sổ
// ghim theo danh sách này, nên test không truyền thì mọi khối ghim đều bị coi là của
// plugin đã tắt.
func speaking(cs []Contribution) []Plugin {
	seen := map[string]bool{}
	var out []Plugin
	for _, c := range cs {
		if c.Plugin == "" || seen[c.Plugin] {
			continue
		}
		seen[c.Plugin] = true
		out = append(out, fake{name: c.Plugin})
	}
	return out
}

// Apply: bản sắc vào cuối lời hệ thống; chữ của lượt đi SAU lượt người, không
// vào lời hệ thống và không vào trong lời người dùng. Các tiếng của lượt giữ đúng
// thứ tự contribs — tức thứ tự plugin được dựng. Bất biến #4 cưỡng chế tại biên.
func TestApplyOrderAndAttribution(t *testing.T) {
	var buf strings.Builder
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	body, err := request.ParseBody([]byte(`{"system":"ROOT","messages":[{"role":"user","content":"hi"}]}`), request.FormatAnthropic)
	if err != nil {
		t.Fatal(err)
	}
	cs := []Contribution{
		{Plugin: "wayfarer", Kind: KindMarker, Text: "🛣️ Wayfarer: milestone"},
		{Plugin: "rogue", Kind: KindMarker, Text: "anonymous line"},
		{Plugin: "memory", Kind: KindMarker, Text: "## Memory"},
		{Plugin: "identity", Kind: KindSystem, Text: "# Soul"},
	}
	Apply(body, cs, freshSession(), speaking(cs))

	if got := body.SystemText(); got != "ROOT\n\n# Soul" {
		t.Fatalf("lời hệ thống chỉ giữ thứ đứng cả phiên, và khối ta đứng SAU, got %q", got)
	}
	out, err := body.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	way := strings.Index(s, "🛣️ Wayfarer: milestone")
	mem := strings.Index(s, "## Memory")
	if way < 0 || mem < 0 || way > mem {
		t.Fatalf("tiếng của lượt phải giữ đúng thứ tự contribs: %s", s)
	}
	if strings.Contains(s, `"content":"hi\n\n`) {
		t.Fatalf("lời người dùng không được bị nối thêm (#1): %s", s)
	}
	if strings.Contains(s, "anonymous line") {
		t.Fatalf("anonymous contribution must be dropped: %s", s)
	}
	// Vẫn phải có tiếng, nhưng ở mức DEBUG: lõi đã đeo marker (`base.Say`) nên cửa này chỉ
	// còn là lưới đỡ cho plugin tự dựng chữ (#4) — người vận hành không sửa được nó, chỉ
	// tác giả plugin sửa được. Cái không sửa được thì không đứng ở console lúc chạy.
	if !strings.Contains(buf.String(), "rogue") {
		t.Fatalf("dropping anonymous contribution must still log at debug: %s", buf.String())
	}
}

// Nhiều khối hệ thống thì thứ tự nằm lại đúng bằng thứ tự contribs — tức thứ tự
// plugin đăng ký, tức thứ tự người đọc. Đệ đọc mình là ai trước, rồi mới tới thứ
// mình biết về người. Và cả nhóm nằm SAU lời của công cụ chủ.
func TestApplySystemBlocksKeepContribOrder(t *testing.T) {
	body, err := request.ParseBody([]byte(`{"system":"ROOT","messages":[]}`), request.FormatAnthropic)
	if err != nil {
		t.Fatal(err)
	}
	cs := []Contribution{
		{Plugin: "identity", Kind: KindSystem, Tag: "Soul", Text: "# Soul"},
		{Plugin: "memory", Kind: KindSystem, Tag: "Memory", Text: "## Memory index"},
	}
	Apply(body, cs, freshSession(), speaking(cs))

	sys := body.SystemText()
	iSoul, iMem, iRoot := strings.Index(sys, "<Soul>"), strings.Index(sys, "<Memory>"), strings.Index(sys, "ROOT")
	if iSoul < 0 || iMem < 0 || iRoot < 0 {
		t.Fatalf("thiếu khối: %q", sys)
	}
	if !(iRoot < iSoul && iSoul < iMem) {
		t.Errorf("thứ tự phải là lời gốc → bản sắc → ký ức:\n%s", sys)
	}
}

// Lỗi bắt được khi chạy thật: chỗ đứng phải quyết MỘT LẦN cho cả lượt. Hỏi lại
// từng lần thì message vừa chèn thành "message cuối không phải của người", và
// tiếng thứ hai rơi vào lời người dùng — đúng chỗ #1 không cho chạm.
func TestApplyDecidesPlacementOnceForTheWholeTurn(t *testing.T) {
	slog.SetDefault(slog.New(slog.NewTextHandler(&strings.Builder{}, nil)))
	body, err := request.ParseBody([]byte(`{"messages":[{"role":"user","content":"lời người"}]}`), request.FormatOpenAI)
	if err != nil {
		t.Fatal(err)
	}
	cs := []Contribution{
		{Plugin: "memory", Kind: KindMarker, Tag: "Memory", Text: "## memory"},
		{Plugin: "outfitter", Kind: KindMarker, Tag: "Outfitter", Text: "🧰 outfitter: với grep"},
	}
	Apply(body, cs, freshSession(), speaking(cs))

	out, err := body.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	var f map[string]json.RawMessage
	if err := json.Unmarshal(out, &f); err != nil {
		t.Fatal(err)
	}
	var msgs []struct{ Role, Content string }
	if err := json.Unmarshal(f["messages"], &msgs); err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 3 {
		t.Fatalf("mỗi tiếng một message riêng — muốn 3, got %d: %+v", len(msgs), msgs)
	}
	if msgs[0].Content != "lời người" {
		t.Errorf("lời người dùng bị nối thêm: %q", msgs[0].Content)
	}
	for i, m := range msgs[1:] {
		if m.Role != request.RoleUser {
			t.Errorf("người vừa nói → chữ đi vai người: message chèn %d là %+v", i, m)
		}
	}
	// Thứ tự: ký ức trước, tiếng bên lề sau.
	if !strings.Contains(msgs[1].Content, "Memory") || !strings.Contains(msgs[2].Content, "Outfitter") {
		t.Errorf("thứ tự Context → Marker bị đảo: %+v", msgs[1:])
	}
}

// Chữ của lượt phải ra CUỐI mảng, ở mọi hình dạng request — đó là chỗ model đọc
// trước khi trả lời. Đọc ra từ một phiên thật: 128 request đều là vòng tool đường
// OpenAI, và cả 20 tiếng của agent bờ bị nối vào câu người mở đầu, cách đó 283
// message. Đệ không thấy gì, mà chữ thì nằm trong lời người dùng.
func TestApplyPutsTurnTextAtTheTail(t *testing.T) {
	slog.SetDefault(slog.New(slog.NewTextHandler(&strings.Builder{}, nil)))
	const line = "🚶 Loiterer: kìa"

	// Một hình, một chỗ, một vai: message MỚI ở cuối, vai `user`, và CHỈ khi người vừa
	// nói. Ba hình request cũ có ba lối — thêm vai `user`, thêm vai `system` (đường
	// OpenAI giữa vòng tool), và nối khối vào chính message chở `tool_result` (đường
	// Anthropic, vì vai phải xen kẽ). Cả ba gộp còn một, vì tiếng của lượt không tồn
	// tại ở lượt không phải của người.
	cases := []struct {
		name      string
		format    request.Format
		body      string
		wantAdded bool
	}{{
		name:      "người vừa nói · đường OpenAI",
		format:    request.FormatOpenAI,
		body:      `{"messages":[{"role":"user","content":"lời người"}]}`,
		wantAdded: true,
	}, {
		name:      "người vừa nói · đường Anthropic, content dạng block",
		format:    request.FormatAnthropic,
		body:      `{"messages":[{"role":"user","content":[{"type":"text","text":"lời người"}]}]}`,
		wantAdded: true,
	}, {
		// Vai cuối là `tool`: máy đang chạy, không ai đọc.
		name:   "giữa vòng tool · đường OpenAI",
		format: request.FormatOpenAI,
		body: `{"messages":[{"role":"user","content":"lời người"},
			{"role":"assistant","content":"","tool_calls":[{"id":"c1","type":"function","function":{"name":"read_file","arguments":"{}"}}]},
			{"role":"tool","tool_call_id":"c1","content":"kết quả"}]}`,
		wantAdded: false,
	}, {
		// `tool_result` nằm dưới vai `user` — vai không nói ra được điều gì, nên cửa
		// đọc `HumanSpokeLast`. Message ấy phải nguyên từng byte: thêm block vào một
		// lượt chỉ có tool_result là 400 khi server tool còn đang chờ.
		name:   "giữa vòng tool · đường Anthropic (tool_result dưới vai user)",
		format: request.FormatAnthropic,
		body: `{"messages":[{"role":"user","content":"lời người"},
			{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"read_file","input":{}}]},
			{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"kết quả"}]}]}`,
		wantAdded: false,
	}, {
		// Vai cuối assistant = "viết tiếp câu này". Chèn vào đó là để model nối lời
		// agent bờ như lời của chính nó.
		name:      "vai cuối là assistant",
		format:    request.FormatAnthropic,
		body:      `{"messages":[{"role":"user","content":"lời người"},{"role":"assistant","content":"đang gọi tool"}]}`,
		wantAdded: false,
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, err := request.ParseBody([]byte(tc.body), tc.format)
			if err != nil {
				t.Fatal(err)
			}
			before := messagesOf(t, body)
			cs := []Contribution{{Plugin: "loiterer", Kind: KindMarker, Text: line}}
			placed := Apply(body, cs, freshSession(), speaking(cs))
			msgs := messagesOf(t, body)

			if !tc.wantAdded {
				if len(msgs) != len(before) || len(placed) != 0 {
					t.Fatalf("lượt không phải của người mà vẫn chèn: %d→%d, placed=%v",
						len(before), len(msgs), placed)
				}
				for i := range before {
					if !bytes.Equal(before[i], msgs[i]) {
						t.Errorf("message[%d] bị sửa:\nvào  %s\nra   %s", i, before[i], msgs[i])
					}
				}
				return
			}

			if len(msgs) != len(before)+1 {
				t.Fatalf("phải thêm ĐÚNG một message: %d→%d", len(before), len(msgs))
			}
			var v struct{ Role string }
			if err := json.Unmarshal(msgs[len(msgs)-1], &v); err != nil {
				t.Fatal(err)
			}
			if v.Role != request.RoleUser {
				t.Errorf("vai message chèn: got %q, muốn %q", v.Role, request.RoleUser)
			}
			if last := string(msgs[len(msgs)-1]); !strings.Contains(last, line) {
				t.Errorf("chữ phải nằm ở message CUỐI:\n%s", last)
			}
			// Mọi message có trước đi qua nguyên TỪNG BYTE (#1).
			for i := range before {
				if !bytes.Equal(before[i], msgs[i]) {
					t.Errorf("message[%d] bị sửa:\nvào  %s\nra   %s", i, before[i], msgs[i])
				}
			}
		})
	}
}

// Chữ lõi đặt vào KHÔNG được đọc thành lượt người. Tiếng của lượt đi vai `user`,
// nên nhật ký sẽ mở một lượt ma ở đó, rồi bản ghi nhịp của đúng request ấy rơi vào
// lượt ma và biến mất ở lần ghi sau — tức đúng những request agent bờ vừa nói là
// những request mất tích khỏi nhật ký.
func TestInjectedTextIsNotReadAsAHumanTurn(t *testing.T) {
	slog.SetDefault(slog.New(slog.NewTextHandler(&strings.Builder{}, nil)))
	for _, tc := range []struct {
		name   string
		format request.Format
		body   string
	}{{
		"đường OpenAI · message mới vai user", request.FormatOpenAI,
		`{"messages":[{"role":"user","content":"lời người"}]}`,
	}, {
		"đường Anthropic · message mới vai user", request.FormatAnthropic,
		`{"messages":[{"role":"user","content":[{"type":"text","text":"lời người"}]}]}`,
	}} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := request.ParseBody([]byte(tc.body), tc.format)
			if err != nil {
				t.Fatal(err)
			}
			cs := []Contribution{
				{Plugin: "loiterer", Kind: KindMarker, Tag: "Loiterer", Text: "🚶 Loiterer: kìa"},
			}
			placed := Apply(body, cs, freshSession(), speaking(cs))
			if len(placed) != 1 {
				t.Fatalf("Apply phải khai chữ nó đặt vào, got %v", placed)
			}

			if humans := body.HumanTexts(placed, request.FrameRules{}); len(humans) != 1 {
				t.Errorf("chỉ có MỘT lượt người trong hội thoại này, đếm được %d: %q", len(humans), humans)
			}
		})
	}
}

func messagesOf(t *testing.T, body *request.Body) []json.RawMessage {
	t.Helper()
	out, err := body.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	var f map[string]json.RawMessage
	if err := json.Unmarshal(out, &f); err != nil {
		t.Fatal(err)
	}
	var msgs []json.RawMessage
	if err := json.Unmarshal(f["messages"], &msgs); err != nil {
		t.Fatal(err)
	}
	return msgs
}
