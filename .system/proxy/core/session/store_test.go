// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestKeyDerivation(t *testing.T) {
	if k, fb := Key("client-key", "tzu", "hi", "reply"); k != "client-key" || fb != "" {
		t.Fatalf("header must win: %q %q", k, fb)
	}
	base, fb := Key("", "tzu", "hi", "")
	if fb != "" {
		t.Fatalf("no assistant turn means no fallback: %q", fb)
	}
	refined, fb2 := Key("", "tzu", "hi", "reply")
	if refined == base || fb2 != base {
		t.Fatalf("key upgrade wrong: refined=%q base=%q fb=%q", refined, base, fb2)
	}
	other, _ := Key("", "tzu", "hi", "other-reply")
	if other == refined {
		t.Fatal("different assistant turns must yield different keys")
	}
}

// Hội thoại có lượt trả lời đầu tiên → session dời sang khoá thăng cấp, mang theo
// state đã đặt; hội thoại mới cùng câu chào mở phiên riêng, không dính state cũ.
func TestTouchPromotesKey(t *testing.T) {
	st := NewStore(filepath.Join(t.TempDir(), "state.json"))
	now := time.Now()

	base, _ := Key("", "tzu", "hi", "")
	s1 := st.Touch(base, "", "tzu", Reach{}, 1, now)
	s1.Say("voice", "🛣️ vắng 4 giờ.")

	refined, fb := Key("", "tzu", "hi", "reply")
	s2 := st.Touch(refined, fb, "tzu", Reach{}, 1, now)
	if s2 != s1 {
		t.Fatal("session must migrate on key upgrade, not open new")
	}
	if s2.Key() != refined || len(s2.Said("voice")) != 1 {
		t.Fatalf("migrate missing state: key=%q said=%v", s2.Key(), s2.Said("voice"))
	}

	s3 := st.Touch(base, "", "tzu", Reach{}, 1, now)
	if s3 == s2 || len(s3.Said("voice")) != 0 {
		t.Fatal("new conversation with same greeting must get a clean session")
	}
}

// greet — lời hệ thống của công cụ chủ + câu chào. Hai cửa sổ mở bằng cùng một câu gửi lên
// đúng chuỗi này, từng byte một.
var greet = []uint64{7, 11}

// convo — hình một request: vân tay từng message, và chữ của message theo chỉ số.
func convo(marks []uint64, textAt map[int]string) Reach {
	return Reach{Marks: marks, Text: func(n int) string { return textAt[n] }}
}

// Lỗi bắt được khi chạy thật với nhiều phiên một lúc: hai hội thoại mở bằng cùng một câu ra cùng một
// khoá, nên cuộc sau nhận trọn sổ ghim của cuộc trước — đệ nghe lời một agent bờ của hội
// thoại KHÁC, trong khi agent ấy im ở chính hội thoại này. Khoá chỉ nói hai cuộc CÓ THỂ là
// một; chuỗi đã có lời đáp là chỗ nói chúng không phải.
func TestAnsweredChainOpensANewSessionForANewConversation(t *testing.T) {
	st := NewStore(filepath.Join(t.TempDir(), "state.json"))
	now := time.Now()
	key, _ := Key("", "Tzu", "Chào Tzu", "")

	a := st.Touch(key, "", "Tzu", convo(greet, nil), 1, now)
	a.Place(11, "loiterer", "🚶 Loiterer: lời chỉ thuộc hội thoại A")
	a.Replied(ReplyMark("Chào. Tôi là Tzu, đang đứng cửa chính."))

	b := st.Touch(key, "", "Tzu", convo(greet, nil), 1, now.Add(time.Minute))
	if b == a {
		t.Fatal("hội thoại mới mở bằng đúng câu chào cũ phải có phiên riêng")
	}
	if got := b.Placed(); len(got) != 0 {
		t.Errorf("sổ ghim của hội thoại A chảy sang B: %v", got)
	}
	if len(a.Placed()) != 1 {
		t.Errorf("hội thoại A phải giữ nguyên sổ ghim của nó: %v", a.Placed())
	}
}

// Đích lỗi (câu 502 ở đích local) thì lượt ấy CHƯA có lời đáp, và công cụ chủ gửi lại y
// nguyên. Đó là một lần THỬ LẠI của chính cuộc này — không thì mỗi lần đích hỏng là một lần
// phiên bị chẻ đôi, mất sổ ghim và trang ký ức giữa hội thoại.
func TestResendWithoutAReplyIsARetryNotANewConversation(t *testing.T) {
	st := NewStore(filepath.Join(t.TempDir(), "state.json"))
	now := time.Now()

	a := st.Touch("k", "", "Tzu", convo(greet, nil), 1, now)
	if again := st.Touch("k", "", "Tzu", convo(greet, nil), 1, now.Add(time.Second)); again != a {
		t.Fatal("gửi lại một lượt chưa có lời đáp là thử lại, không phải cuộc mới")
	}

	// Có lời đáp rồi thì lượt kế NỐI DÀI chuỗi — vẫn là cuộc ấy đi tiếp.
	a.Replied(ReplyMark("đáp 1"))
	next := st.Touch("k", "", "Tzu", convo([]uint64{7, 11, 13}, nil), 1, now.Add(time.Minute))
	if next != a {
		t.Fatal("chuỗi nối dài là cuộc ấy đi tiếp, không phải cuộc mới")
	}
}

// Người dùng rewind sau mỗi lần thử, và công cụ chủ cũng tự viết lại lịch sử của nó (sửa lời cũ,
// nén phiên). Chuỗi ngắn đi hay lệch đi vẫn là cuộc ấy: chỉ chuỗi Y NGUYÊN sau khi đã có lời
// đáp mới là cuộc khác.
func TestRewindAndRewrittenHistoryStayInTheSameSession(t *testing.T) {
	st := NewStore(filepath.Join(t.TempDir(), "state.json"))
	now := time.Now()

	a := st.Touch("k", "", "Tzu", convo([]uint64{7, 11, 13, 17}, nil), 2, now)
	a.Replied(ReplyMark("đáp 2"))
	if back := st.Touch("k", "", "Tzu", convo(greet, nil), 1, now.Add(time.Minute)); back != a {
		t.Error("rewind về lượt cũ phải ở lại phiên ấy")
	}
	if edited := st.Touch("k", "", "Tzu", convo([]uint64{7, 11, 99}, nil), 2, now.Add(2*time.Minute)); edited != a {
		t.Error("công cụ chủ viết lại lịch sử của chính nó vẫn là cuộc ấy")
	}
}

// Sau một lần tách, hai nhánh đứng chung một chuỗi và chuỗi thôi tách được chúng. Lời đáp thì
// tách được: request tới mang theo lời đáp của lượt trước, và lời ấy mỗi nhánh một khác. Đây
// là chỗ giữ được CẢ HAI — ranh giới giữa hai cuộc, và tái dựng đủ ở mọi lượt.
func TestAmbiguousBranchIsResolvedByItsOwnReply(t *testing.T) {
	st := NewStore(filepath.Join(t.TempDir(), "state.json"))
	now := time.Now()

	a := st.Touch("k", "", "Tzu", convo(greet, nil), 1, now)
	a.Place(11, "loiterer", "🚶 Loiterer: lời của A")
	a.Replied(ReplyMark("Chào, tôi là lời đáp của A."))

	b := st.Touch("k", "", "Tzu", convo(greet, nil), 1, now.Add(time.Minute))
	b.Replied(ReplyMark("Chào, tôi là lời đáp của B."))
	if b == a {
		t.Fatal("hai cuộc phải là hai nhánh")
	}

	// Lượt hai của A: lời đáp của A nằm ở message thứ 2 — ngay sau chuỗi dài 2 của nhánh A.
	next := st.Touch("k", "", "Tzu",
		convo([]uint64{7, 11, 13, 17}, map[int]string{2: "Chào, tôi là lời đáp của A."}),
		2, now.Add(2*time.Minute))
	if next != a {
		t.Fatal("nhánh nhận ra lời của chính mình phải là nhánh nhận request")
	}
	if len(next.Placed()) != 1 {
		t.Errorf("về đúng nhánh thì sổ ghim còn nguyên — lượt này vẫn tái dựng được: %v", next.Placed())
	}
}

// Không nhánh nào nhận ra lời của mình — đích chỉ gọi tool nên không có chữ nào, hoặc lời đáp
// dài quá khúc vòi giữ. Lúc ấy lõi không nhận vơ: mở phiên mới. Mất một lượt tái dựng, không
// mất ranh giới (#2).
func TestAmbiguousBranchWithoutAReplyDoesNotGuess(t *testing.T) {
	st := NewStore(filepath.Join(t.TempDir(), "state.json"))
	now := time.Now()

	a := st.Touch("k", "", "Tzu", convo(greet, nil), 1, now)
	a.Replied(ReplyMark("lời đáp của A"))
	b := st.Touch("k", "", "Tzu", convo(greet, nil), 1, now.Add(time.Minute))
	b.Replied(ReplyMark("lời đáp của B"))

	next := st.Touch("k", "", "Tzu", convo([]uint64{7, 11, 13}, nil), 2, now.Add(2*time.Minute))
	if next == a || next == b {
		t.Error("không đọc được lời đáp thì phải mở phiên mới, không chọn bừa một nhánh")
	}
}

// `X-WON-Session` là LỜI KHAI, và lõi không dò lại lời khai. Đúng thân này, đúng khoá này, đã
// có lời đáp rồi — đường DÒ tách nó ra hai phiên (xem TestAnsweredChainOpensANewSession…),
// còn đường KHAI thì không, vì công cụ chủ đã nói nó là cuộc nào. Cả bộ dò chỉ tồn tại để bù
// cho chỗ không ai nói; nói rồi mà vẫn dò thì chỉ còn đường sai.
func TestDeclaredSessionIsNeverSplitByTheCoreGuesses(t *testing.T) {
	st := NewStore(filepath.Join(t.TempDir(), "state.json"))
	now := time.Now()

	key, fallback := Key("phiên-của-người-dùng", "Tzu", "Chào Tzu", "đáp 1")
	if key != "phiên-của-người-dùng" || fallback != "" {
		t.Fatalf("khoá khai phải thắng và không có khoá lui: %q %q", key, fallback)
	}
	said := Reach{Declared: true, Marks: greet}

	a := st.Touch(key, fallback, "Tzu", said, 1, now)
	a.Place(11, "loiterer", "🚶 Loiterer: lời của cuộc này")
	a.Replied(ReplyMark("đáp 1"))

	b := st.Touch(key, fallback, "Tzu", said, 1, now.Add(time.Minute))
	if b != a {
		t.Fatal("khoá khai mà lõi vẫn tách phiên — lời khai đang bị dò lại")
	}
	if len(b.Placed()) != 1 {
		t.Errorf("sổ ghim của phiên khai phải còn nguyên: %v", b.Placed())
	}

	// Lịch sử lệch hẳn cũng không tách: khoá khai là khoá khai.
	c := st.Touch(key, fallback, "Tzu", Reach{Declared: true, Marks: []uint64{99, 98, 97}}, 2, now.Add(2*time.Minute))
	if c != a {
		t.Error("thân lệch không được làm phiên khai tách ra")
	}
}

// Số lượt của phiên trước được chốt khi cùng đệ mở phiên khác — đó là cái "thường"
// để soul đo cái "bất thường". Lõi chỉ đếm, không phán.
func TestPastTurnsCarryToNextSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	st := NewStore(path)
	t0 := time.Date(2026, 7, 25, 9, 0, 0, 0, time.UTC)

	for i := 1; i <= 3; i++ { // hội thoại dài dần: 1, 2, rồi 3 lượt người
		st.Touch("k1", "", "Tzu", Reach{}, i, t0.Add(time.Duration(i)*time.Minute))
	}
	s2 := st.Touch("k2", "", "Tzu", Reach{}, 1, t0.Add(time.Hour))
	if got := s2.PastTurns(); len(got) != 1 || got[0] != 3 {
		t.Fatalf("phiên trước 3 lượt, got %v", got)
	}
	if got := st.Touch("k1b", "", "Mo", Reach{}, 1, t0.Add(2*time.Hour)).PastTurns(); len(got) != 0 {
		t.Errorf("đệ khác không thừa hưởng nhịp của Tzu, got %v", got)
	}

	// Sống qua restart: lịch sử nằm trong state.json.
	st.Flush()
	if got := NewStore(path).Touch("k3", "", "Tzu", Reach{}, 1, t0.Add(3*time.Hour)).PastTurns(); len(got) != 1 || got[0] != 3 {
		t.Errorf("nhịp phải sống qua restart, got %v", got)
	}
}

// Sổ trang ký ức: mở bao nhiêu lần, từ phiên nào — con số biến việc chuyển vùng của kho
// (củng cố · phai · lắng đọng) thành phép đo. Đếm theo PHIÊN, không theo đồng hồ: số
// nguyên không có múi giờ để lệch. Chỉ con số, không một chữ nội dung (#5).
func TestPageStatsCountOpensAcrossSessions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	st := NewStore(path)
	now := time.Now()

	s1 := st.Touch("k1", "", "Tzu", Reach{}, 1, now)
	s1.SeePages([]string{"personal/self.md", "moments/x.md"})
	s1.Open("personal/self.md", "ruột")
	st.Flush()

	s2 := st.Touch("k2", "", "Tzu", Reach{}, 1, now)
	stats, no := s2.PageStats()
	if no != 2 {
		t.Fatalf("phiên thứ hai phải là số 2, got %d", no)
	}
	if got := stats["personal/self.md"]; got.Opens != 1 || got.FirstSession != 1 || got.LastOpen != 1 {
		t.Errorf("sổ trang đã mở: %+v", got)
	}
	if got := stats["moments/x.md"]; got.Opens != 0 || got.FirstSession != 1 {
		t.Errorf("trang chỉ thấy chứ chưa mở: %+v", got)
	}

	s2.Open("personal/self.md", "ruột")
	st.Flush()

	// Sống qua restart: sổ nằm trong state.json.
	fresh := NewStore(path)
	stats3, no3 := fresh.Touch("k3", "", "Tzu", Reach{}, 1, now).PageStats()
	if no3 != 3 {
		t.Errorf("số phiên phải sống qua restart, got %d", no3)
	}
	if got := stats3["personal/self.md"]; got.Opens != 2 || got.LastOpen != 2 {
		t.Errorf("số lần mở phải sống qua restart: %+v", got)
	}
}

// Tắt proxy rồi mở lại giữa một hội thoại: khoá dẫn xuất từ nội dung nên nó về đúng khoá cũ,
// nhưng sổ phiên trong RAM đã trắng. Không có sổ danh tính thì cuộc ấy nhận mốc mở MỚI — thư
// mục nhật ký chẻ đôi theo ngày, và khối House trong lời hệ thống đổi chữ giữa hội thoại, tức
// tiền tố cache gãy ở mọi đích. Đo trên chính đường đi thật: Flush → Store mới → gửi tiếp.
func TestSessionIdentitySurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	t0 := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)

	st1 := NewStore(path)
	a := st1.Touch("k", "", "Tzu", convo(greet, nil), 1, t0)
	_, no1 := a.PageStats()
	a.Replied(ReplyMark("đáp 1"))
	st1.Flush()

	// Restart: tiến trình mới, sổ phiên trong RAM trắng, hội thoại cũ gửi lượt kế.
	st2 := NewStore(path)
	b := st2.Touch("k", "", "Tzu", convo([]uint64{7, 11, 13}, nil), 2, t0.Add(time.Hour))
	if !b.FirstSeen().Equal(a.FirstSeen()) {
		t.Errorf("mốc mở phải sống qua restart: %v ≠ %v", b.FirstSeen(), a.FirstSeen())
	}
	if _, no2 := b.PageStats(); no2 != no1 {
		t.Errorf("số phiên phải sống qua restart: %d ≠ %d", no2, no1)
	}
}

// Sổ danh tính không được xoá đúng cái ranh giới mốc mở sinh ra để giữ: hai hội thoại mở bằng
// cùng một câu chào ra cùng một khoá, và cuộc thứ hai phải có mốc mở RIÊNG. Chỉ khi khoá không
// còn nhánh nào sống — tức tiến trình vừa khởi động lại — mốc cũ mới được nhặt lại.
func TestIdentityIsNotHandedToADifferentConversation(t *testing.T) {
	st := NewStore(filepath.Join(t.TempDir(), "state.json"))
	t0 := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)

	a := st.Touch("k", "", "Tzu", convo(greet, nil), 1, t0)
	a.Replied(ReplyMark("đáp của A"))

	b := st.Touch("k", "", "Tzu", convo(greet, nil), 1, t0.Add(time.Minute))
	if b == a {
		t.Fatal("hội thoại mới mở bằng đúng câu chào cũ phải có phiên riêng")
	}
	if b.FirstSeen().Equal(a.FirstSeen()) {
		t.Error("cuộc mới nhặt phải mốc mở của cuộc cũ — hai cuộc sẽ dồn vào một thư mục nhật ký")
	}
	_, noA := a.PageStats()
	_, noB := b.PageStats()
	if noA == noB {
		t.Errorf("hai cuộc phải là hai số phiên, cả hai đều %d", noA)
	}
}

// Khoá thăng cấp thì danh tính đi theo, không nằm lại dưới khoá cũ: một hội thoại KHÁC mở
// bằng đúng câu chào ấy sẽ hỏi đúng khoá cũ, và nhặt được mốc mở của cuộc này.
func TestIdentityFollowsThePromotedKey(t *testing.T) {
	st := NewStore(filepath.Join(t.TempDir(), "state.json"))
	t0 := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)

	base, _ := Key("", "Tzu", "chào", "")
	s1 := st.Touch(base, "", "Tzu", Reach{}, 1, t0)

	refined, fb := Key("", "Tzu", "chào", "đáp")
	s2 := st.Touch(refined, fb, "Tzu", Reach{}, 1, t0.Add(time.Minute))
	if s2 != s1 || !s2.FirstSeen().Equal(s1.FirstSeen()) {
		t.Fatal("thăng cấp khoá phải giữ nguyên phiên và mốc mở của nó")
	}

	s3 := st.Touch(base, "", "Tzu", Reach{}, 1, t0.Add(time.Hour))
	if s3.FirstSeen().Equal(s1.FirstSeen()) {
		t.Error("danh tính nằm lại dưới khoá chưa thăng cấp — cuộc mới nhặt phải mốc mở cũ")
	}
}

func TestFlushPersistsAndFailsOpen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	st := NewStore(path)
	st.Touch("k", "", "tzu", Reach{}, 1, time.Now())
	st.Flush()
	b, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(b), "tzu") {
		t.Fatalf("Flush did not write state: err=%v content=%s", err, b)
	}

	// State hỏng → bắt đầu trắng, không panic, không chặn.
	if err := os.WriteFile(path, []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	st2 := NewStore(path)
	if st2.Touch("k2", "", "tzu", Reach{}, 1, time.Now()) == nil {
		t.Fatal("store with broken state must still run")
	}
}

// Flush phải tự tạo thư mục cha khi chưa có: lần chạy đầu, .system/proxy/run
// chưa tồn tại thì os.WriteFile lỗi nếu không tạo trước. Test khoá regression này.
func TestFlushCreatesMissingParentDir(t *testing.T) {
	// Thư mục cha chưa tồn tại — t.TempDir() tạo gốc nhưng không tạo con.
	dir := filepath.Join(t.TempDir(), "nested", "run")
	path := filepath.Join(dir, "state.json")

	st := NewStore(path)
	st.Touch("k", "", "tzu", Reach{}, 1, time.Now())
	st.Flush()

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Flush did not create parent dir and write state: %v", err)
	}
	if !strings.Contains(string(b), "tzu") {
		t.Fatalf("state file missing expected content, got %s", b)
	}
}

// Quãng vắng đo theo dấu thời gian của ĐỆ đang mở phiên, không phải global:
// khoảng cách giữa hai đệ khác nhau là một con số vô nghĩa. Wayfarer cắm mốc
// "quay lại sau vắng dài" từ con số này, nên nó phải là quãng vắng của đúng đệ đó.
func TestGapAtStartPerAgent(t *testing.T) {
	st := NewStore(filepath.Join(t.TempDir(), "state.json"))
	t0 := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

	// Đệ A chạm lần đầu — không có dấu trước, gap=0.
	sA1 := st.Touch("ka", "", "A", Reach{}, 1, t0)
	if g := sA1.GapAtStart(); g != 0 {
		t.Fatalf("A first touch: gap must be 0, got %v", g)
	}

	// Đệ B chạm lần đầu tại t1 = t0 + 48h — B chưa từng xuất hiện, fallback
	// global (lượt A). Gap B = t1 - t0 = 48h (đây là fallback global, không
	// phải per-đệ B, vì B chưa có dấu riêng).
	t1 := t0.Add(48 * time.Hour)
	sB1 := st.Touch("kb", "", "B", Reach{}, 1, t1)
	if g := sB1.GapAtStart(); g != 48*time.Hour {
		t.Fatalf("B first touch (global fallback): gap must be 48h, got %v", g)
	}

	// B chạm lại tại t2 = t1 + 72h — B đã có dấu tại t1, gap per-đệ = 72h.
	// Đây là chỗ phân biệt: không phải t2-t0 (120h global), mà t2-t1 (72h).
	t2 := t1.Add(72 * time.Hour)
	st.Flush()
	sB2 := st.Touch("kb2", "", "B", Reach{}, 1, t2)
	if g := sB2.GapAtStart(); g != 72*time.Hour {
		t.Fatalf("B second touch (per-agent): gap must be 72h, got %v — must measure against LastSeenAgent[B], not global", g)
	}

	// A chạm lại tại t3 = t2 + 1h — A có dấu tại t0, gap per-đệ = t3-t0.
	// Không bị B (chạm gần hơn) ảnh hưởng — đây là cốt per-đệ.
	t3 := t2.Add(1 * time.Hour)
	sA2 := st.Touch("ka2", "", "A", Reach{}, 1, t3)
	wantA := t3.Sub(t0)
	if g := sA2.GapAtStart(); g != wantA {
		t.Fatalf("A second touch (per-agent): gap must be %v, got %v — not overwritten by B (more recent)", wantA, g)
	}
}

// Gap per-đệ sống qua restart: Flush → Store mới → LastSeenAgent load lại →
// con số vẫn đúng. Mất nó thì mỗi lần khởi động lại là một lần quên quãng vắng.
func TestGapAtStartPerAgentSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	t0 := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	st1 := NewStore(path)
	st1.Touch("ka", "", "A", Reach{}, 1, t0)
	st1.Touch("kb", "", "B", Reach{}, 1, t0.Add(48*time.Hour))
	st1.Flush()

	// Restart: Store mới load state.json — LastSeenAgent phải còn.
	st2 := NewStore(path)
	t2 := t0.Add(48*time.Hour + 72*time.Hour) // 120h sau t0
	sB := st2.Touch("kb2", "", "B", Reach{}, 1, t2)
	// B có dấu tại t0+48h → gap per-đệ = 72h (t2 - (t0+48h)).
	if g := sB.GapAtStart(); g != 72*time.Hour {
		t.Fatalf("after restart: B per-agent gap must be 72h, got %v — LastSeenAgent not surviving restart?", g)
	}
}

// Hai con số, không một. Đo trên một phiên thật:
// MỘT tin nhắn của người đẩy số lần chạy lên 6, vì đệ vào vòng tool. Trộn hai cái ấy
// làm mọi ngưỡng "đã đủ lượt chưa" đọc sai, và soul nghe "lượt thứ 6" khi người mới
// nói một câu.
func TestTurnsCountHumanSpeechRunsCountPasses(t *testing.T) {
	st := NewStore(filepath.Join(t.TempDir(), "state.json"))
	t0 := time.Date(2026, 7, 27, 22, 0, 0, 0, time.UTC)

	var s *Session
	for i := 0; i < 6; i++ { // sáu lần chạy, hội thoại vẫn chỉ có MỘT lượt người
		s = st.Touch("k", "", "Tzu", Reach{}, 1, t0.Add(time.Duration(i)*time.Second))
	}
	if s.Turns() != 1 {
		t.Errorf("một tin nhắn người = 1 lượt, got %d", s.Turns())
	}
	if s.Runs() != 6 {
		t.Errorf("sáu lần proxy chạy = 6 run, got %d", s.Runs())
	}

	s = st.Touch("k", "", "Tzu", Reach{}, 2, t0.Add(time.Minute)) // người nói lần hai
	if s.Turns() != 2 || s.Runs() != 7 {
		t.Errorf("turns=%d runs=%d, muốn 2 và 7", s.Turns(), s.Runs())
	}
}

// REWIND: hội thoại bị cắt ngắn thì số lượt phải ĐI XUỐNG. Một bộ đếm chỉ biết cộng
// thì số ở lại chỗ cao mãi, và mọi ngưỡng đọc lượt người sai từ đó tới hết phiên.
// Rewind sau mỗi lần thử là nếp dùng thật, nên đây là đường đi thật, không phải ca giả định.
func TestRewindLowersTurnCount(t *testing.T) {
	st := NewStore(filepath.Join(t.TempDir(), "state.json"))
	t0 := time.Date(2026, 7, 28, 23, 0, 0, 0, time.UTC)

	s := st.Touch("k", "", "Tzu", Reach{}, 4, t0)
	if s.Turns() != 4 {
		t.Fatalf("bốn lượt người, got %d", s.Turns())
	}
	s = st.Touch("k", "", "Tzu", Reach{}, 2, t0.Add(time.Minute)) // rewind về lượt hai
	if s.Turns() != 2 {
		t.Errorf("rewind về 2 lượt mà số vẫn %d — bộ đếm không có đường về", s.Turns())
	}
	if s.Runs() != 2 {
		t.Errorf("rewind vẫn là một lần chạy: runs=%d, muốn 2", s.Runs())
	}
}

// Lượt việc nhà của công cụ chủ đội số lên ĐÚNG lần chạy ấy, rồi lượt thật kế tiếp
// kéo về đúng. Cộng dồn thì cái +1 ấy ở lại vĩnh viễn: `base.Ask` lấy vân tay trên lời
// hỏi mà số lượt nằm trong lời hỏi, nên luật "cùng câu thì không hỏi lại" không bao
// giờ nổ, và soul nghe sai số lượt tới hết phiên.
func TestHousekeepingTurnDoesNotStick(t *testing.T) {
	st := NewStore(filepath.Join(t.TempDir(), "state.json"))
	t0 := time.Date(2026, 7, 28, 23, 0, 0, 0, time.UTC)

	st.Touch("k", "", "Tzu", Reach{}, 2, t0)                     // lượt người thứ hai
	st.Touch("k", "", "Tzu", Reach{}, 3, t0.Add(time.Second))    // host chèn một chỉ thị vào ô người
	s := st.Touch("k", "", "Tzu", Reach{}, 2, t0.Add(time.Hour)) // lượt thật kế: vẫn đang ở lượt hai
	if s.Turns() != 2 {
		t.Errorf("số lượt phải về 2 sau lượt việc nhà, got %d", s.Turns())
	}
}

// Sổ bền ghi lượt NGƯỜI: đó là "cái thường" Wayfarer đem so. Ghi lần chạy thì một
// phiên vòng tool dài hiện ra như một phiên khổng lồ — state cũ có phiên ghi 103.
func TestPastTurnsRecordsHumanTurns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	st := NewStore(path)
	t0 := time.Date(2026, 7, 27, 22, 0, 0, 0, time.UTC)

	for i := 0; i < 20; i++ { // 20 lần chạy, hội thoại vẫn một lượt người
		st.Touch("k1", "", "Tzu", Reach{}, 1, t0.Add(time.Duration(i)*time.Second))
	}
	if got := st.Touch("k2", "", "Tzu", Reach{}, 1, t0.Add(time.Hour)).PastTurns(); len(got) != 1 || got[0] != 1 {
		t.Errorf("phiên trước phải ghi 1 lượt người, got %v", got)
	}
}

// Host gửi lại CÙNG một hội thoại thì không phải lượt mới. Đo trên phiên thật: bảy
// request, cả bảy đều có lời người ở cuối (không vòng tool), nhưng người chỉ nói một
// lần — cờ "người vừa nói" đúng cả bảy, nên nó không đủ để đếm lượt.
func TestResendSameUtteranceIsNotANewTurn(t *testing.T) {
	st := NewStore(filepath.Join(t.TempDir(), "state.json"))
	t0 := time.Date(2026, 7, 27, 22, 0, 0, 0, time.UTC)

	var s *Session
	for i := 0; i < 7; i++ {
		s = st.Touch("k", "", "Tzu", Reach{}, 1, t0.Add(time.Duration(i)*time.Second))
	}
	if s.Turns() != 1 {
		t.Errorf("một câu gửi bảy lần = 1 lượt người, got %d", s.Turns())
	}
	if s.Runs() != 7 {
		t.Errorf("bảy lần chạy = 7 run, got %d", s.Runs())
	}
}

// Không lời người nào trong request → giữ số cũ, không hạ về 0. Lượt việc nhà không
// mang hội thoại nào không được xoá số lượt của phiên đang chảy.
func TestNoHumanTurnKeepsCount(t *testing.T) {
	st := NewStore(filepath.Join(t.TempDir(), "state.json"))
	now := time.Now()

	if s := st.Touch("k", "", "Tzu", Reach{}, 0, now); s.Turns() != 0 || s.Runs() != 1 {
		t.Errorf("phiên trắng: turns=%d runs=%d, muốn 0 và 1", s.Turns(), s.Runs())
	}
	st.Touch("k", "", "Tzu", Reach{}, 3, now.Add(time.Second))
	if s := st.Touch("k", "", "Tzu", Reach{}, 0, now.Add(2*time.Second)); s.Turns() != 3 {
		t.Errorf("số lượt bị xoá bởi một request không có lời người: %d", s.Turns())
	}
}

// Sổ "đã nói" chia theo GIỌNG. Khoá là chuỗi bất kỳ — test dùng tên chung, vì cái đang
// test là phép chia khoá, không phải một plugin nào: lõi không biết có những giọng nào.
// Dồn một chỗ thì một giọng đọc được lời giọng khác và nhận làm lời mình, và giọng nào
// bày trọn sổ ra cho model xem thì bày cả lời nó chưa từng nói.
func TestSaidIsScopedPerVoice(t *testing.T) {
	st := NewStore(filepath.Join(t.TempDir(), "state.json"))
	s := st.Touch("k", "", "Tzu", Reach{}, 1, time.Now())

	s.Say("voice-a", "dòng của giọng thứ nhất")
	s.Say("voice-b", "dòng của giọng thứ hai")

	if got := s.Said("voice-a"); len(got) != 1 || got[0] != "dòng của giọng thứ nhất" {
		t.Errorf("giọng này chỉ được thấy lời của chính nó: %v", got)
	}
	if got := s.Said("voice-b"); len(got) != 1 || got[0] != "dòng của giọng thứ hai" {
		t.Errorf("giọng này chỉ được thấy lời của chính nó: %v", got)
	}
	if got := s.Said("voice-c"); len(got) != 0 {
		t.Errorf("giọng chưa nói gì phải thấy sổ trống: %v", got)
	}
	// Info giữ SỐ, không giữ nội dung (#5) — và số ấy là tổng của mọi giọng.
	if n := s.info(time.Now()).Said; n != 2 {
		t.Errorf("tổng số dòng đã nói: %d, muốn 2", n)
	}
}
