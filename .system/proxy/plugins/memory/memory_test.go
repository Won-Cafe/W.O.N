// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package memory

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"won/proxy/core/paths"
	"won/proxy/core/plugin"
	"won/proxy/core/request"
	"won/proxy/core/session"
)

// fakeLLM trả lời cho sẵn — bộ chọn phải kiểm được mà không gọi mạng.
// reply/err là shorthand: trả cho mọi call khi replies/errs rỗng. replies/errs là
// per-call: replies[i] cho call thứ i, errs[i] cho call thứ i.
type fakeLLM struct {
	reply   string
	replies []string
	err     error
	errs    []error
	calls   int
	system  string
	user    string
}

func (f *fakeLLM) Chat(_ context.Context, system, user string) (string, error) {
	i := f.calls
	f.calls++
	f.system = system
	f.user = user
	if len(f.replies) > 0 || len(f.errs) > 0 {
		var r string
		if i < len(f.replies) {
			r = f.replies[i]
		}
		var e error
		if i < len(f.errs) {
			e = f.errs[i]
		}
		return r, e
	}
	return f.reply, f.err
}

func writeStore(t *testing.T, pages map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for path, body := range pages {
		full := filepath.Join(root, ".system", "memory", filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

const focusPage = "# Focus — ship W.O.N\n\n*Tác vụ đang mở — chưa đóng.*\n\nRuột trang focus.\n"
const selfPage = "# Memory — Self\n\n*Ký ức bền về bạn.*\n\nRuột trang self.\n"

func newMem(t *testing.T, root string, llm chatter) *Memory {
	t.Helper()
	opts, _ := json.Marshal(map[string]any{})
	p, err := New(plugin.Env{Paths: paths.Tree{Root: root}, Services: plugin.NewHub(), Options: opts})
	if err != nil {
		t.Fatal(err)
	}
	m := p.(*Memory)
	m.llm = llm
	return m
}

func turnSession(t *testing.T, n int) *session.Session {
	t.Helper()
	return turnAt(t, turnStore(t), n)
}

// turnStore — sổ phiên thật, giữ lại được để đẩy CÙNG một phiên sang lượt người kế.
func turnStore(t *testing.T) *session.Store {
	t.Helper()
	return session.NewStore(filepath.Join(t.TempDir(), "state.json"))
}

// turnAt đưa phiên `k` tới lượt người thứ n. Cùng khoá → cùng phiên, nên gọi lại với
// n lớn hơn là đẩy phiên đang có sang lượt sau, không phải mở một phiên khác.
func turnAt(t *testing.T, st *session.Store, n int) *session.Session {
	t.Helper()
	var s *session.Session
	for i := 1; i <= n; i++ {
		s = st.Touch("k", "", "Tzu", session.Reach{}, i, time.Now())
	}
	return s
}

// gathered — hai nhịp của memory tách ra. Sys là khối vào lời hệ thống (index + nhắc),
// Usr là khối xuống cuối mảng messages (trang đã mở). Text là cả hai nối lại, cho test
// chỉ quan tâm nội dung; test nào quan tâm CHỖ CHÈN thì đọc Sys/Usr.
type gathered struct {
	Text string
	Sys  string
	Usr  string
	Kind plugin.Kind // của khối hệ thống
	Tag  string
	N    int
}

// gather gộp lời đáp của Contribute. Không đóng góp nào → nil, vì im lặng phải đọc ra
// là im lặng chứ không phải một khối rỗng.
func gather(cs []*plugin.Contribution, err error) (*gathered, error) {
	if err != nil || len(cs) == 0 {
		return nil, err
	}
	g := &gathered{N: len(cs)}
	for _, c := range cs {
		switch c.Kind {
		case plugin.KindSystem:
			g.Sys, g.Kind, g.Tag = c.Text, c.Kind, c.Tag
		case plugin.KindMarker:
			g.Usr = c.Text
		}
	}
	g.Text = g.Sys
	if g.Usr != "" {
		g.Text += "\n" + g.Usr
	}
	return g, nil
}

func (p *Memory) give(ctx context.Context, sn *request.Snapshot, sess *session.Session) (*gathered, error) {
	return gather(p.Contribute(ctx, sn, sess))
}

func snap() *request.Snapshot { return snapAsking("hỏi") }

// snapAsking — lượt người hỏi một câu cụ thể. Câu hỏi phải khác nhau ở các lượt
// khác nhau: base.Ask chỉ hỏi model MỘT LẦN cho mỗi câu hỏi trong phiên, nên phát
// lại y nguyên một lượt là dựng một cảnh không có thật.
// HumanSpokeLast luôn true: mở trang là chèn mới, và chèn mới chỉ xảy ra ở lượt của
// người. Ca giữa vòng tool có test riêng bên dưới.
func snapAsking(text string) *request.Snapshot {
	return &request.Snapshot{Agent: "Tzu", Anchor: text, HumanSpokeLast: true,
		Turns: []request.Turn{{Role: "user", Text: text}}}
}

// Lượt đầu chỉ có index: đệ chưa có gì để một trang phải chạm vào, nên chưa tốn
// một lượt gọi model nào.
func TestFirstTurnIndexOnly(t *testing.T) {
	root := writeStore(t, map[string]string{"working/a.md": focusPage, "personal/self.md": selfPage})
	llm := &fakeLLM{reply: "working/a.md"}
	c, err := newMem(t, root, llm).give(context.Background(), snap(), turnSession(t, 1))
	if err != nil || c == nil {
		t.Fatalf("phải có index: %v %v", c, err)
	}
	if llm.calls != 0 {
		t.Errorf("lượt đầu không được gọi model, gọi %d lần", llm.calls)
	}
	if !strings.Contains(c.Text, "- working/a.md — Focus — ship W.O.N · Tác vụ đang mở") {
		t.Errorf("dòng index thiếu tiêu đề hoặc mô tả:\n%s", c.Text)
	}
	if strings.Contains(c.Text, "Ruột trang focus") {
		t.Errorf("lượt đầu không được mở trang:\n%s", c.Text)
	}
	if c.Kind != plugin.KindSystem || c.Tag != "Memory" {
		t.Errorf("phải là khối hệ thống mang tag Memory, got %v %q", c.Kind, c.Tag)
	}
}

// GIỮA VÒNG TOOL không được mở trang. Mở trang là CHÈN MỚI vào lời hệ thống, và lời
// hệ thống phải đứng yên trong một lượt người: mốc cache của công cụ chủ nằm sau chữ
// ta chèn, nên khối `<Memory>` phồng lên giữa lượt là cả hội thoại trả lại phí ghi.
//
// Khối index thì VẪN phải đi — nhà cung cấp không giữ phiên, khối nào không gửi lại là
// khối không tồn tại. Nên Memory không phải TurnVoice; chỉ đường dựng mới bị đóng.
func TestNoNewPageInsideToolLoop(t *testing.T) {
	root := writeStore(t, map[string]string{"working/a.md": focusPage, "personal/self.md": selfPage})
	llm := &fakeLLM{reply: "working/a.md"}
	sess := turnSession(t, 2)
	loop := snapAsking("giữa vòng tool")
	loop.HumanSpokeLast = false

	c, err := newMem(t, root, llm).give(context.Background(), loop, sess)
	if err != nil || c == nil {
		t.Fatalf("khối index vẫn phải đi ở mọi lần chạy: %v %v", c, err)
	}
	if llm.calls != 0 {
		t.Errorf("giữa vòng tool mà vẫn hỏi model chọn trang: %d lượt gọi", llm.calls)
	}
	if len(sess.Opened()) != 0 {
		t.Errorf("trang bị mở giữa vòng tool: %v", sess.Opened())
	}
	if !strings.Contains(c.Text, "- working/a.md") {
		t.Errorf("index phải còn nguyên:\n%s", c.Text)
	}
}

// Từ lượt hai: model chọn trang, trang được rót TRỌN, kèm đường dẫn làm nguồn.
func TestOpensPageFromTurnTwo(t *testing.T) {
	root := writeStore(t, map[string]string{"working/a.md": focusPage, "personal/self.md": selfPage})
	llm := &fakeLLM{reply: "working/a.md"}
	sess := turnSession(t, 2)
	c, _ := newMem(t, root, llm).give(context.Background(), snap(), sess)
	if llm.calls != 1 {
		t.Fatalf("phải hỏi model đúng một lần, got %d", llm.calls)
	}
	if !strings.Contains(c.Text, "## working/a.md") || !strings.Contains(c.Text, "Ruột trang focus") {
		t.Errorf("trang chưa được mở trọn:\n%s", c.Text)
	}
	if !sess.HasOpened("working/a.md") {
		t.Error("trang đã mở phải nằm trong phiên")
	}
	// Ứng viên đưa cho model phải kèm mô tả — thiếu nó thì đo được là model bịa.
	if !strings.Contains(llm.system, "· Tác vụ đang mở") {
		t.Errorf("index gửi cho bộ chọn thiếu dòng mô tả:\n%s", llm.system)
	}
}

// Trang đã mở thì ĐÓNG BĂNG: file đổi giữa phiên cũng không đọc lại, vì khối hệ thống
// đổi là tự đốt cache của chính nó. Và trong CÙNG một lượt người, bộ chọn không được
// hỏi lại dù lần chạy sau model muốn mở trang khác.
func TestOpenedPageFrozenAndPickerStandsDown(t *testing.T) {
	root := writeStore(t, map[string]string{"working/a.md": focusPage, "personal/self.md": selfPage})
	llm := &fakeLLM{reply: "working/a.md"}
	m := newMem(t, root, llm)
	sess := turnSession(t, 2)
	m.give(context.Background(), snap(), sess)

	path := filepath.Join(root, ".system", "memory", "working", "a.md")
	if err := os.WriteFile(path, []byte("# Focus — ship W.O.N\n\n*Tác vụ đang mở.*\n\nRUỘT MỚI.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	llm.calls, llm.reply = 0, "personal/self.md" // model muốn mở trang khác
	c, _ := m.give(context.Background(), snapAsking("hỏi tiếp"), sess)

	if strings.Contains(c.Text, "RUỘT MỚI") {
		t.Errorf("trang đã mở phải đóng băng, không đọc lại file:\n%s", c.Text)
	}
	if !strings.Contains(c.Text, "Ruột trang focus") {
		t.Errorf("mất bản đã đóng băng:\n%s", c.Text)
	}
	if llm.calls != 0 {
		t.Errorf("phiên đã mở trang mà bộ chọn vẫn được hỏi: %d lượt gọi", llm.calls)
	}
	if len(sess.Opened()) != 1 {
		t.Errorf("mở thêm trang sau khi kho đã chốt: %d", len(sess.Opened()))
	}
}

// Tên bịa thì bỏ, không đi tìm hộ. Đây là vành đai duy nhất chặn model nền chèn
// một đường dẫn không có thật.
func TestInventedPathDropped(t *testing.T) {
	root := writeStore(t, map[string]string{"working/a.md": focusPage})
	llm := &fakeLLM{reply: "working/khong-co-that.md, moments/bia.md"}
	sess := turnSession(t, 2)
	c, _ := newMem(t, root, llm).give(context.Background(), snap(), sess)
	if strings.Contains(c.Text, "khong-co-that") || len(sess.Opened()) != 0 {
		t.Errorf("tên bịa phải bị bỏ:\n%s", c.Text)
	}
}

// Model nền ngăn tên bằng phẩy, bằng xuống dòng, và bằng cả khoảng trắng — bắt
// được khi chạy thật, và lần đó trang đáng mở đã không mở.
func TestPicksSurviveEverySeparator(t *testing.T) {
	cand := []string{"working/a.md", "personal/self.md"}
	for _, out := range []string{
		"working/a.md, personal/self.md",
		"working/a.md\npersonal/self.md",
		"personal/ personal/ working/a.md personal/self.md", // nguyên văn một lượt trả về thật
		"`working/a.md`, `personal/self.md`.",
	} {
		got := parsePicks(out, cand, 2)
		if len(got) != 2 || got[0] != "working/a.md" || got[1] != "personal/self.md" {
			t.Errorf("đọc hụt %q → %v", out, got)
		}
	}
	if got := parsePicks("working/a.md, working/a.md", cand, 2); len(got) != 1 {
		t.Errorf("trùng thì lấy một, got %v", got)
	}
}

func TestNoneAndErrorStayIndexOnly(t *testing.T) {
	root := writeStore(t, map[string]string{"working/a.md": focusPage})
	for _, llm := range []*fakeLLM{{reply: "NONE"}, {reply: "∅"}, {err: errors.New("đứt")}} {
		sess := turnSession(t, 2)
		c, err := newMem(t, root, llm).give(context.Background(), snap(), sess)
		if err != nil {
			t.Fatalf("model im hay hỏng đều không phải lỗi của dòng chính: %v", err)
		}
		if len(sess.Opened()) != 0 || strings.Contains(c.Text, "Ruột trang") {
			t.Errorf("không được mở trang nào: %q", llm.reply)
		}
		if !strings.Contains(c.Text, "- working/a.md") {
			t.Error("index vẫn phải còn — đó là đường chữa khi bộ chọn im")
		}
	}
}

// fourMoments — kho bốn trang cùng vùng, đủ để trần lượt cắt mà vẫn còn ứng viên.
func fourMoments(t *testing.T) (root string, all []string) {
	t.Helper()
	store := map[string]string{}
	for _, n := range []string{"a", "b", "c", "d"} {
		store["moments/"+n+".md"] = "# Moment — " + n + "\n\n*Chưa kiểm chứng.*\n\nRuột " + n + ".\n"
		all = append(all, "moments/"+n+".md")
	}
	return writeStore(t, store), all
}

// Bộ chọn chạy MỖI LƯỢT NGƯỜI: index chốt ở đầu phiên nên nó có giới hạn, và những
// lần mở sau là phần bù. Trần mỗi lượt vẫn cắt trong từng lượt.
func TestPickRunsEachHumanTurn(t *testing.T) {
	root, all := fourMoments(t)
	llm := &fakeLLM{reply: strings.Join(all, ", ")} // model đòi mở cả bốn, mọi lượt
	m := newMem(t, root, llm)
	st := turnStore(t)
	sess := turnAt(t, st, 2)

	c, _ := m.give(context.Background(), snapAsking("hỏi 1"), sess)
	if n := len(sess.Opened()); n != 2 {
		t.Fatalf("trần mỗi lượt là 2, mở %d", n)
	}
	// Khai đường đi tiếp — index vẫn nêu mọi trang, kể cả trang chưa mở.
	if !strings.Contains(c.Text, "đọc thẳng theo đường dẫn") {
		t.Errorf("không khai đường đi tiếp:\n%s", c.Text)
	}
	for _, p := range all {
		if !strings.Contains(c.Text, "- "+p) {
			t.Errorf("index thiếu %s:\n%s", p, c.Text)
		}
	}

	// Lượt người kế: mở tiếp hai trang còn lại.
	sess = turnAt(t, st, 3)
	if _, err := m.give(context.Background(), snapAsking("hỏi 2"), sess); err != nil {
		t.Fatal(err)
	}
	if n := len(sess.Opened()); n != 4 {
		t.Errorf("lượt sau phải mở tiếp, tổng muốn 4, got %d", n)
	}
}

// Khối của mỗi lượt chỉ kể trang mở TRONG lượt ấy. Lõi ghim khối của lượt trước lại đúng
// chỗ nó (§ Cache), nên kể dồn là mỗi khối chở lại trọn phần khối trước đã chở — trang mở
// ở lượt 2 sẽ nằm trong khối lượt 2, lượt 3, lượt 4, mãi.
func TestOpenedBlockCarriesOnlyThisTurnsPages(t *testing.T) {
	root, all := fourMoments(t)
	m := newMem(t, root, &fakeLLM{reply: strings.Join(all, ", ")})
	st := turnStore(t)

	two, _ := m.give(context.Background(), snapAsking("hỏi 1"), turnAt(t, st, 2))
	first := openedPaths(t, two.Usr)
	if len(first) != 2 {
		t.Fatalf("lượt 2 phải mở 2 trang, khối kể %v", first)
	}

	three, _ := m.give(context.Background(), snapAsking("hỏi 2"), turnAt(t, st, 3))
	second := openedPaths(t, three.Usr)
	if len(second) != 2 {
		t.Fatalf("lượt 3 phải kể đúng 2 trang của lượt ấy, khối kể %v", second)
	}
	for _, p := range first {
		if slices.Contains(second, p) {
			t.Errorf("trang %s của lượt 2 bị kể lại ở khối lượt 3: %v", p, second)
		}
	}
}

// openedPaths — các đường dẫn khối trang đã mở đang kể, đọc từ dòng tiêu đề của từng trang.
func openedPaths(t *testing.T, block string) []string {
	t.Helper()
	var out []string
	for _, ln := range strings.Split(block, "\n") {
		if p, ok := strings.CutPrefix(ln, "## "); ok && strings.HasSuffix(p, ".md") {
			out = append(out, p)
		}
	}
	return out
}

// Một lượt NGƯỜI kéo hàng chục lần chạy, và bộ chọn chỉ được hỏi một lần trong lượt
// ấy. Không có cửa này thì trần phiên cháy hết trong một lượt.
func TestPickAsksOncePerTurnNotPerRun(t *testing.T) {
	root, all := fourMoments(t)
	llm := &fakeLLM{reply: all[0]}
	m := newMem(t, root, llm)
	sess := turnSession(t, 2)

	if _, err := m.give(context.Background(), snapAsking("hỏi 1"), sess); err != nil {
		t.Fatal(err)
	}
	first := llm.calls
	// Cùng lượt người, lần chạy thứ hai và thứ ba.
	for i := 0; i < 2; i++ {
		if _, err := m.give(context.Background(), snapAsking("chạy lại"), sess); err != nil {
			t.Fatal(err)
		}
	}
	if llm.calls != first {
		t.Errorf("cùng một lượt người mà hỏi model %d lần (muốn %d)", llm.calls, first)
	}
}

// `max_index_per_zone` là núm, không phải hằng chôn: hạ nó xuống thì index ngắn lại,
// giữ trang MỚI nhất, và số bị bỏ phải khai ra chứ không im lặng biến mất.
func TestIndexPerZoneIsConfigurable(t *testing.T) {
	root, all := fourMoments(t)
	// Bốn trang cùng vùng `moments/`, mtime tăng dần theo tên.
	dir := filepath.Join(root, ".system", "memory", "moments")
	base := time.Now().Add(-96 * time.Hour)
	for i, n := range []string{"a", "b", "c", "d"} {
		ts := base.Add(time.Duration(i) * time.Hour)
		if err := os.Chtimes(filepath.Join(dir, n+".md"), ts, ts); err != nil {
			t.Fatal(err)
		}
	}
	m := newMemOpt(t, root, &fakeLLM{reply: "∅"}, map[string]any{"max_index_per_zone": 2})
	c, err := m.give(context.Background(), snap(), turnSession(t, 1))
	if err != nil || c == nil {
		t.Fatal(err)
	}
	// Giữ hai trang đặt bút gần nhất: c và d.
	for _, want := range []string{"- moments/c.md", "- moments/d.md"} {
		if !strings.Contains(c.Sys, want) {
			t.Errorf("index thiếu %s:\n%s", want, c.Sys)
		}
	}
	for _, gone := range []string{"- moments/a.md", "- moments/b.md"} {
		if strings.Contains(c.Sys, gone) {
			t.Errorf("trần 2 mà %s vẫn còn trong index:\n%s", gone, c.Sys)
		}
	}
	if !strings.Contains(c.Sys, "+2 trang nữa không nêu") {
		t.Errorf("số trang bị trần bỏ phải khai ra:\n%s", c.Sys)
	}
	_ = all
}

// Trần index cũng là trần của việc mở: bộ chọn chỉ chọn được trong index, nên trang bị
// trần bỏ thì không đường nào mở được nó — kể cả khi model gọi đúng tên.
func TestIndexCeilingBoundsWhatCanOpen(t *testing.T) {
	root, all := fourMoments(t)
	dir := filepath.Join(root, ".system", "memory", "moments")
	base := time.Now().Add(-96 * time.Hour)
	for i, n := range []string{"a", "b", "c", "d"} {
		ts := base.Add(time.Duration(i) * time.Hour)
		if err := os.Chtimes(filepath.Join(dir, n+".md"), ts, ts); err != nil {
			t.Fatal(err)
		}
	}
	llm := &fakeLLM{reply: strings.Join(all, ", ")} // model đòi cả bốn, kể cả trang đã bị trần bỏ
	m := newMemOpt(t, root, llm, map[string]any{"max_index_per_zone": 2, "max_open_per_turn": 4})
	sess := turnSession(t, 2)
	if _, err := m.give(context.Background(), snap(), sess); err != nil {
		t.Fatal(err)
	}
	if n := len(sess.Opened()); n != 2 {
		t.Fatalf("chỉ mở được trang có trong index, muốn 2, got %d", n)
	}
	for _, p := range sess.Opened() {
		if p.Path == "moments/a.md" || p.Path == "moments/b.md" {
			t.Errorf("mở được trang đã bị trần index bỏ: %s", p.Path)
		}
	}
}

// Không có model nền → còn index, không lỗi.
func TestNoLLMStillGivesIndex(t *testing.T) {
	root := writeStore(t, map[string]string{"working/a.md": focusPage})
	c, err := newMem(t, root, nil).give(context.Background(), snap(), turnSession(t, 3))
	if err != nil || c == nil || !strings.Contains(c.Text, "- working/a.md") {
		t.Fatalf("vắng model nền vẫn phải có index: %v %v", c, err)
	}
}

// Kho rỗng KHÔNG im trọn: nói đúng MỘT lời mời, đúng MỘT lần mỗi phiên.
//
// Vì sao luật cũ ("kho rỗng thì im hẳn") phải đổi: lời mời viết trang đầu sống trong
// `renderUse`, mà `renderUse` chỉ dựng khi kho ĐÃ có trang; `nudge` cũng nằm sau cùng cái
// cửa ấy. Mồi nằm bên trong cửa nó phải mở, nên kho rỗng thì rỗng mãi — đo trên kho thật:
// memory im 223/223 lần chạy, và `procedural/` rỗng trọn qua 80 phiên.
//
// Đây là ngoại lệ CÓ KHAI của #3, không phải một lỗ thủng: #3 đòi mọi byte chèn thêm phải
// có lý do, và lý do ở đây đo được. Ba cửa của `takeNote` giữ nó không thành tiếng ồn —
// lượt của người, đã qua lượt đầu, và tiêu một lần là thôi.
func TestEmptyStoreInvitesOncePerSession(t *testing.T) {
	empty := newMem(t, t.TempDir(), nil)
	st := turnStore(t)

	// Lượt đầu: đệ chưa có gì để lời mời phải chạm vào — im, cùng cửa với mọi lời gọi việc.
	if c, err := gather(empty.Contribute(context.Background(), snap(), turnAt(t, st, 1))); c != nil || err != nil {
		t.Errorf("lượt đầu phải im, got %v %v", c, err)
	}

	sess := turnAt(t, st, 2)
	c, err := gather(empty.Contribute(context.Background(), snapAsking("lượt hai"), sess))
	if err != nil || c == nil {
		t.Fatalf("kho rỗng phải mời một lời ở lượt hai: %v %v", c, err)
	}
	// Kho rỗng thì KHÔNG có index để chèn — chỉ một khối đi ở nhịp lượt, không khối hệ thống.
	if c.N != 1 || c.Sys != "" {
		t.Errorf("chỉ một khối nhịp lượt, got N=%d sys=%q", c.N, c.Sys)
	}
	// #4 kiểm trên text TRẦN, trước khi lõi bọc tag — lời mời phải tự mang tên plugin.
	if !strings.HasPrefix(c.Usr, "## Memory") {
		t.Errorf("lời mời phải tự mang tên plugin, got %q", c.Usr)
	}

	// Tiêu một lần là thôi: cùng phiên, lần chạy sau im.
	if c2, err := gather(empty.Contribute(context.Background(), snapAsking("lượt hai nữa"), sess)); c2 != nil || err != nil {
		t.Errorf("lời mời đã tiêu thì phiên này thôi nhắc, got %v %v", c2, err)
	}
}

// Index dựng MỘT LẦN mỗi phiên. Hai điều nó phải giữ, và cả hai đo được ở đây: kho
// ghi thêm giữa phiên thì khối index đứng yên, và đĩa không bị đọc lại — nên xoá sạch
// kho giữa chừng cũng không làm khối ấy rỗng đi.
func TestIndexBuiltOncePerSession(t *testing.T) {
	root := writeStore(t, map[string]string{"working/a.md": focusPage})
	m := newMem(t, root, nil)
	sess := turnSession(t, 1)

	first, err := m.give(context.Background(), snap(), sess)
	if err != nil || first == nil {
		t.Fatalf("lượt đầu: %v %v", first, err)
	}

	// Bút của Shu đặt xuống giữa phiên.
	extra := filepath.Join(root, ".system", "memory", "personal", "self.md")
	if err := os.MkdirAll(filepath.Dir(extra), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(extra, []byte(selfPage), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := m.give(context.Background(), snapAsking("lượt sau"), sess)
	if err != nil || second == nil {
		t.Fatalf("lượt sau: %v %v", second, err)
	}
	if second.Text != first.Text {
		t.Errorf("index đổi giữa phiên\n--- đầu ---\n%s\n--- sau ---\n%s", first.Text, second.Text)
	}

	// Kho biến mất mà khối vẫn nguyên = lượt sau không chạm đĩa.
	if err := os.RemoveAll(filepath.Join(root, ".system", "memory")); err != nil {
		t.Fatal(err)
	}
	third, err := m.give(context.Background(), snapAsking("lượt nữa"), sess)
	if err != nil || third == nil || third.Text != first.Text {
		t.Errorf("index phải sống độc lập với đĩa sau khi chốt: %v %v", third, err)
	}
}

// Phiên MỚI dựng lại index — chốt là chốt trong một phiên, không phải chốt vĩnh viễn.
func TestNewSessionSeesNewPages(t *testing.T) {
	root := writeStore(t, map[string]string{"working/a.md": focusPage})
	m := newMem(t, root, nil)
	if _, err := m.give(context.Background(), snap(), turnSession(t, 1)); err != nil {
		t.Fatal(err)
	}
	extra := filepath.Join(root, ".system", "memory", "personal", "self.md")
	if err := os.MkdirAll(filepath.Dir(extra), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(extra, []byte(selfPage), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := m.give(context.Background(), snap(), turnSession(t, 1))
	if err != nil || c == nil || !strings.Contains(c.Text, "personal/self.md") {
		t.Errorf("phiên mới phải thấy trang mới: %v %v", c, err)
	}
}

// Thứ tự trong một vùng theo LẦN SỬA CUỐI, cũ trước mới sau — vì cái trần cắt đầu
// bảng, nên thứ tự này quyết trang nào rơi. Tên đứng đầu bảng chữ cái mà vừa được
// đặt bút thì phải nằm cuối, không phải đầu.
func TestZoneOrderFollowsLastWritten(t *testing.T) {
	root := writeStore(t, map[string]string{
		"moments/a-cu.md":  focusPage,
		"moments/z-moi.md": selfPage,
	})
	dir := filepath.Join(root, ".system", "memory", "moments")
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(filepath.Join(dir, "z-moi.md"), old, old); err != nil {
		t.Fatal(err)
	}
	pages, _ := newMem(t, root, nil).list()
	if len(pages) != 2 {
		t.Fatalf("muốn 2 trang, got %d", len(pages))
	}
	if pages[0].Path != "moments/z-moi.md" {
		t.Errorf("trang sửa lâu nhất phải đứng đầu (rơi trước khi trần cắt), got %s", pages[0].Path)
	}
}

// Trần `max_open_per_turn` phải tới TỚI hợp đồng. Chép cứng số vào hợp đồng thì núm
// chỉ hạ được (bộ lọc lời đáp cắt), còn nâng lên thì model vẫn nghe con số cũ.
func TestLimitReachesTheContract(t *testing.T) {
	root := writeStore(t, map[string]string{
		"working/a.md": focusPage, "working/b.md": focusPage,
		"moments/c.md": selfPage, "personal/self.md": selfPage,
	})
	for _, tc := range []struct {
		perTurn int
		want    string
	}{{1, "at most one"}, {2, "at most two"}, {3, "at most three"}} {
		llm := &fakeLLM{reply: "∅"}
		m := newMem(t, root, llm)
		m.perTurn = tc.perTurn
		if _, err := m.give(context.Background(), snapAsking("hỏi "+tc.want), turnSession(t, 2)); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(llm.system, tc.want) {
			t.Errorf("perTurn=%d: hợp đồng phải nói %q, got:\n%s", tc.perTurn, tc.want, llm.system)
		}
	}
}

// Hợp đồng không xin nhiều trang hơn số trang đang có: xin bốn khi kho có một là một
// ô trống mời điền, và ô trống là chỗ model bịa đường dẫn.
func TestLimitClampedToCandidates(t *testing.T) {
	root := writeStore(t, map[string]string{"personal/self.md": selfPage})
	llm := &fakeLLM{reply: "∅"}
	m := newMem(t, root, llm)
	m.perTurn = 5
	if _, err := m.give(context.Background(), snap(), turnSession(t, 2)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(llm.system, "at most one") {
		t.Errorf("kho một trang thì hợp đồng phải xin một, got:\n%s", llm.system)
	}
}

// HAI NHỊP, HAI KHỐI. Index đứng cả phiên nên nó vào lời hệ thống, chỗ tiền tố cache
// làm phần lặp lại rẻ đi. Trang mở đổi theo lượt nên nó xuống cuối mảng messages, vai
// người — sau mốc cache, nên chèn thêm không ghi lại gì.
func TestOpenedPagesGoToTheTurnBlock(t *testing.T) {
	root := writeStore(t, map[string]string{"working/a.md": focusPage, "personal/self.md": selfPage})
	llm := &fakeLLM{reply: "working/a.md"}
	c, err := newMem(t, root, llm).give(context.Background(), snap(), turnSession(t, 2))
	if err != nil || c == nil {
		t.Fatalf("give: %v %v", c, err)
	}
	if c.N != 2 {
		t.Fatalf("phải hai khối (index + trang mở), got %d", c.N)
	}
	if strings.Contains(c.Sys, "Ruột trang focus") {
		t.Errorf("trang mở lọt vào lời hệ thống — mỗi lượt mở là một lần dựng lại tiền tố:\n%s", c.Sys)
	}
	if !strings.Contains(c.Usr, "Ruột trang focus") {
		t.Errorf("trang mở phải nằm ở khối của lượt:\n%s", c.Usr)
	}
	if !strings.Contains(c.Sys, "- working/a.md") {
		t.Errorf("index phải ở lời hệ thống:\n%s", c.Sys)
	}
	if strings.Contains(c.Usr, "- personal/self.md") {
		t.Errorf("index không được lặp lại ở khối lượt:\n%s", c.Usr)
	}
}

// Cưỡng chế #4 kiểm trên text TRẦN, trước khi lõi bọc tag. Ruột khối lượt là chữ của
// chính người dùng nên không quy về ai được — thiếu dòng tự khai thì lõi bỏ cả khối.
func TestTurnBlockCarriesAttribution(t *testing.T) {
	root := writeStore(t, map[string]string{"working/a.md": focusPage})
	llm := &fakeLLM{reply: "working/a.md"}
	c, _ := newMem(t, root, llm).give(context.Background(), snap(), turnSession(t, 2))
	if !strings.Contains(strings.ToLower(c.Usr), "memory") {
		t.Errorf("khối lượt thiếu căn cước — lõi sẽ bỏ nó kèm warn:\n%s", c.Usr)
	}
}

// Chưa mở trang nào thì chỉ một khối. Một khối lượt rỗng là một message thừa trong
// mảng, và lõi phải bỏ nó — thà đừng sinh ra.
func TestNoTurnBlockWhenNothingOpened(t *testing.T) {
	root := writeStore(t, map[string]string{"working/a.md": focusPage})
	c, _ := newMem(t, root, &fakeLLM{reply: "∅"}).give(context.Background(), snap(), turnSession(t, 2))
	if c.N != 1 || c.Usr != "" {
		t.Errorf("chưa mở trang mà vẫn sinh khối lượt: N=%d usr=%q", c.N, c.Usr)
	}
}

// Khối chèn mang tên plugin — bất biến #4 kiểm trên text gốc.
func TestContributionIsAttributed(t *testing.T) {
	root := writeStore(t, map[string]string{"working/a.md": focusPage})
	c, _ := newMem(t, root, nil).give(context.Background(), snap(), turnSession(t, 1))
	if !strings.Contains(strings.ToLower(c.Text), "memory") {
		t.Error("thiếu căn cước trong lời chèn (#4)")
	}
}

// stubPage — chữ THẬT của `personal/self.md` đã gây chuyện: đủ tiêu đề và mô tả nên nó qua
// được cửa "rỗng ruột", vào index, rồi bộ chọn mở nó ra để chở về đúng chữ "chưa có".
const stubPage = "# Memory — Self\n\n*Ký ức bền về bạn — sở thích, thói quen, thiên hướng.*\n\n---\n\n*(chưa có — chờ moments củng cố)*\n"

// Trang mới có khung thì không vào index: mở ra nó không đưa thêm gì so với chính dòng
// index, mà lại chiếm một suất trong trần mỗi lượt — và chiếm vĩnh viễn, vì dòng mô tả
// của một trang khung hợp với gần như mọi lượt.
func TestFrameOnlyPageStaysOutOfTheIndex(t *testing.T) {
	root := writeStore(t, map[string]string{"working/a.md": focusPage, "personal/self.md": stubPage})
	llm := &fakeLLM{reply: "personal/self.md"}
	c, _ := newMem(t, root, llm).give(context.Background(), snap(), turnSession(t, 2))
	if strings.Contains(c.Sys, "personal/self.md") {
		t.Errorf("trang chỉ có khung lọt vào index:\n%s", c.Sys)
	}
	if strings.Contains(c.Text, "chưa có — chờ moments") {
		t.Errorf("ruột trang khung bị chở vào ngữ cảnh:\n%s", c.Text)
	}
	if !strings.Contains(c.Sys, "- working/a.md") {
		t.Errorf("trang có ruột phải còn:\n%s", c.Sys)
	}
}

// Ranh của cửa trên: tiêu đề THỨ HAI đã là ruột — trang ấy có dàn ý để mở.
func TestSecondHeadingCountsAsBody(t *testing.T) {
	outline := "# Memory — Self\n\n*Mô tả.*\n\n## Thói quen\n\n## Thiên hướng\n"
	if !hasBody(outline) {
		t.Error("trang có dàn ý phải tính là có ruột")
	}
	if hasBody(stubPage) {
		t.Error("trang chỉ có khung không được tính là có ruột")
	}
}

// Giữa vòng tool lõi không đặt khối mới xuống, nên dựng khối ở đó là dựng một thứ chắc
// chắn bị bỏ. Đo trên một lượt thật: 16 lần chạy, 15 lần dựng thừa — và nhật ký đọc ra
// như thể trang được chèn lại mỗi lần.
func TestTurnBlockNotRebuiltInsideToolLoop(t *testing.T) {
	root := writeStore(t, map[string]string{"working/a.md": focusPage, "personal/self.md": selfPage})
	llm := &fakeLLM{reply: "working/a.md"}
	m := newMem(t, root, llm)
	st := turnStore(t)
	sess := turnAt(t, st, 2)

	c, _ := m.give(context.Background(), snap(), sess)
	if c.Usr == "" {
		t.Fatal("lượt của người phải dựng khối")
	}
	loop := snapAsking("giữa vòng tool")
	loop.HumanSpokeLast = false
	c2, _ := m.give(context.Background(), loop, sess)
	if c2.Usr != "" {
		t.Errorf("dựng lại khối giữa vòng tool:\n%s", c2.Usr)
	}
	if c2.N != 1 {
		t.Errorf("giữa vòng tool chỉ còn khối index, got %d khối", c2.N)
	}
	// Trang vẫn đang mở trong phiên — cửa này chỉ đóng đường DỰNG, khối cũ về chỗ đã ghim
	// bằng sổ phiên, không qua đây.
	if !sess.HasOpened("working/a.md") {
		t.Error("trang đã mở phải còn trong phiên")
	}
}

// File mẫu không phải trang: nó nằm cùng thư mục nhưng không phải ký ức của ai. Luật
// một nhà (paths.IsPage) — bản chép thứ hai ở kẻ đo đường đã từng lệch.
func TestTemplatesStayOutOfTheIndex(t *testing.T) {
	root := writeStore(t, map[string]string{
		"working/a.md":                focusPage,
		"working/template-working.md": "# Focus — mẫu\n\n*Mẫu để chép.*\n",
	})
	c, _ := newMem(t, root, nil).give(context.Background(), snap(), turnSession(t, 1))
	if strings.Contains(c.Sys, "template-") {
		t.Errorf("file mẫu lọt vào index:\n%s", c.Sys)
	}
	if !strings.Contains(c.Sys, "- working/a.md") {
		t.Errorf("trang thật phải còn:\n%s", c.Sys)
	}
}

// Khối phải khai BA việc đệ làm được với kho — đọc, theo working/, giao người cầm bút —
// rồi hai vành đai cơ học. Thiếu chúng thì khối đọc ra là một cuốn mục lục chỉ-đọc: đo
// được trên kho thật, 80 phiên không sinh một trang nào.
//
// Ba việc ấy nằm ở plugin chứ không ở soul, và đó là ràng buộc chứ không phải khẩu vị:
// soul là bản thể mười một trục, không mang cơ học. Chép vào soul thì tắt plugin xong
// soul vẫn dặn đi qua một cái cửa không còn ở đó.
func TestBlockDeclaresWhatTheAgentCanDo(t *testing.T) {
	root := writeStore(t, map[string]string{"working/a.md": focusPage})
	c, _ := newMem(t, root, nil).give(context.Background(), snap(), turnSession(t, 1))
	for _, want := range []string{
		"về NGƯỜI dùng hệ này",             // đọc — kho nói về ai
		"không phải trụ của họ",            // đọc — ranh với trang của chính người
		"mở thẳng được theo đường dẫn",     // đọc — đường tự đi, không đợi bộ chọn
		"dấu để nhận ra một điều vừa nghe", // phát hiện — việc của đệ khi đang nghe
		"`working/` — việc đang theo",      // phát hiện — dấu của vùng việc
		"đóng khi người nói xong",          // phát hiện — luật đóng, không phải tắt máy
		"`moments/` — vừa nảy",             // phát hiện — dấu của vùng bọt
		"`personal/` — điều đã lặp",        // phát hiện — dấu của vùng trầm tích
		"thì nói ra",                       // làm gì — lời chung cho MỌI vai
		"bút của kho nằm ở đệ Shu",         // bút có địa chỉ, nêu như một sự thật
		"`# tiêu đề`", "*mô tả*",           // vành đai: hình một trang
		"PUT /plugins/memory/update", // vành đai: cửa sỏi
	} {
		if !strings.Contains(c.Sys, want) {
			t.Errorf("khối thiếu %q:\n%s", want, c.Sys)
		}
	}
	// Khối rót cho MỌI đệ, nên không câu nào được là việc chỉ một vai thi hành được, và
	// không câu nào được giả định người thật đang ở đầu kia: một đệ mở ra như subagent chỉ
	// thấy lời giao của Tzu ở vai người.
	for _, wrong := range []string{
		"giao đệ Shu",    // chỉ Tzu điều phối được; đệ không gọi đệ
		"gọi đệ Shu",     // như trên
		"việc của bút",   // lời gọi không có địa chỉ
		"người đang nói", // subagent không có người thật trong hội thoại
	} {
		if strings.Contains(c.Sys, wrong) {
			t.Errorf("câu này không đúng cho mọi vai: %q\n%s", wrong, c.Sys)
		}
	}
}

// Cùng phép thử cho sổ kho: nó cũng tới tay mọi đệ, ở chỗ model đọc sau chót.
func TestNoteWordingFitsEveryRole(t *testing.T) {
	sess := sessionAt(t, 6, map[string]int{"personal/self.md": 6})
	pages := []page{
		{Path: "moments/2026-07-10-alpha.md", Head: "Alpha"},
		{Path: "personal/self.md", Head: "Self"},
	}
	got := nudge(pages, sess, 10, "Shu")
	if !strings.Contains(got, "là việc của đệ Shu") {
		t.Errorf("sổ kho phải gọi tên người cầm bút:\n%s", got)
	}
	for _, wrong := range []string{"việc của bút", "giao đệ Shu", "gọi đệ Shu", "hãy "} {
		if strings.Contains(got, wrong) {
			t.Errorf("sổ kho ra lệnh cho một vai cụ thể: %q\n%s", wrong, got)
		}
	}
}

// Đường tới cửa sỏi dựng từ chính tên plugin, và KHÔNG mang host:port: địa chỉ Control
// API là cấu hình của lõi — plugin không biết nó, nên không được đoán (#6).
func TestScoreRouteCarriesNoGuessedAddress(t *testing.T) {
	root := writeStore(t, map[string]string{"working/a.md": focusPage})
	m := newMem(t, root, nil)
	if got := m.scoreRoute(); !strings.Contains(got, m.Name()) {
		t.Errorf("đường phải dựng từ tên plugin, got %q", got)
	}
	c, _ := m.give(context.Background(), snap(), turnSession(t, 1))
	for _, guess := range []string{"127.0.0.1", "localhost", "7777", "http://"} {
		if strings.Contains(c.Sys, guess) {
			t.Errorf("đang đoán địa chỉ của lõi: %q\n%s", guess, c.Sys)
		}
	}
}

// Scorer đổi qua config thì lời khai đổi theo — chép cứng "Shu" là một bản nói sai khi
// núm được vặn, và không ai báo.
func TestWritePathNamesTheConfiguredScorer(t *testing.T) {
	root := writeStore(t, map[string]string{"working/a.md": focusPage})
	opts, _ := json.Marshal(map[string]any{"scorer": "Han"})
	p, err := New(plugin.Env{Paths: paths.Tree{Root: root}, Services: plugin.NewHub(), Options: opts})
	if err != nil {
		t.Fatal(err)
	}
	c, _ := p.(*Memory).give(context.Background(), snap(), turnSession(t, 1))
	if !strings.Contains(c.Sys, "đệ Han") || strings.Contains(c.Sys, "đệ Shu") {
		t.Errorf("lời khai phải theo núm scorer:\n%s", c.Sys)
	}
}
