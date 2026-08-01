// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

// Package session giữ luồng của một đệ trong một hội thoại và phần state
// tối thiểu sống qua restart.
package session

import (
	"hash/fnv"
	"strings"
	"sync"
	"time"
)

// Reach — cái request nói về việc nó thuộc cuộc nào. Hai hạng, và chúng không ngang nhau.
//
// Declared là LỜI KHAI: công cụ chủ tự nói mình là phiên nào (header `X-WON-Session`). Khai
// rồi thì lõi không dò lại — một khoá là một phiên, không nhánh, không chuỗi, không vân tay
// lời đáp. Cùng luật với đệ tường minh ở cửa căn cước.
//
// Marks/Text là cái lõi TỰ ĐỌC khi không ai khai: vân tay từng message, và một cửa hỏi chữ
// của message thứ n. Chụp TRƯỚC khi lõi chèn gì. Cả bộ dò dựng trên hai thứ này chỉ tồn tại
// để bù cho công cụ chủ không nói được mình là ai; nói được rồi thì chúng thành thừa, và một
// phép dò thừa thì chỉ còn đường sai.
//
// Sổ phiên đọc qua đây và không giữ lại chữ nào (#5).
type Reach struct {
	Declared bool
	Marks    []uint64
	Text     func(n int) string
}

// askedMax — trần sổ câu hỏi đã hỏi. Vượt thì quên hết rồi hỏi lại: hỏi thừa
// một lượt còn hơn giữ một cái sổ không đáy.
const askedMax = 512

// Session là state plugin cần. Mọi thứ đổi theo lượt đi qua method có khoá — plugin
// chạy song song.
type Session struct {
	mu sync.Mutex

	// Đặt một lần lúc mở phiên rồi bất biến.
	no         int // số thứ tự phiên, tính từ sổ bền
	key        string
	agent      string
	firstSeen  time.Time
	gapAtStart time.Duration

	// Đổi theo lượt.
	note      string
	said      []string
	lastSeen  time.Time
	pastTurns []int

	// Chuỗi vân tay mảng message của lần chạy gần nhất, và lượt đứng ở chuỗi ấy đã có lời
	// đáp chưa. Khoá phiên dẫn xuất từ câu mở đầu, nên hai hội thoại mở bằng cùng một câu ra
	// cùng một khoá; hai thứ này là chỗ tách chúng ra (xem joinable).
	chain    []uint64
	answered bool
	// replyMark — vân tay lời đáp nhánh này vừa sinh ra ở chuỗi ấy. Lượt sau, lời đáp đó nằm
	// trong chính request công cụ chủ gửi lên, nên đây là cái nói "cuộc đi tiếp này là của
	// tôi" khi hai nhánh cùng khoá đứng chung một chuỗi (xem produced). VÂN TAY, không phải
	// chữ: sổ phiên không giữ lời hội thoại (#5).
	replyMark uint64

	// Hai con số, không một: `runs` là lần proxy chạy, `turns` là lần NGƯỜI nói. Một lượt
	// người kéo nhiều lần chạy, nên trộn chúng làm mọi ngưỡng đọc sai: `min_turns = 3`
	// chạm ngay trong một lượt, và soul nghe "lượt thứ 7" khi người mới nói một câu.
	runs  int
	turns int

	// Kho ký ức: trang đã mở, trang đã thấy, và sổ trang lúc mở phiên.
	opened    []Page
	openPages []string            // trang vừa mở, Store rút ra rồi cộng vào sổ
	seenPages map[string]bool     // trang plugin thấy trong kho lượt này
	pageStats map[string]PageStat // ảnh chụp lúc mở phiên

	// Index kho, CHỐT ở lần dựng đầu của phiên. Quét lại đĩa mỗi lần chạy là hai cái
	// giá: một lượt người kéo hàng chục lần chạy, nên mỗi trang bị đọc hàng chục lần;
	// và kho đổi giữa phiên thì khối index đổi theo, mà nó nằm trong lời hệ thống.
	memLines   string
	memPaths   []string
	memDropped int

	// Lượt NGƯỜI gần nhất bộ chọn đã chạy. Một lượt người kéo hàng chục lần chạy, nên
	// không có con số này thì "mỗi lượt một lần hỏi" thành "mỗi lần chạy một lần hỏi".
	pickedAt int

	// Sổ khối lõi đã chèn vào mảng hội thoại. Công cụ chủ không giữ chữ ta chèn — lượt sau
	// nó gửi lại lịch sử NGUYÊN BẢN — nên không có sổ này thì mỗi lần chạy là một lần quên,
	// và thân đi ra thành bản SỬA của lần trước chứ không phải bản NỐI. Chỉ trong RAM: nó
	// tả một mảng đang chảy, không phải một sự thật sống qua lần khởi động.
	placed []Placed

	asked map[uint64]bool // vân tay câu hỏi đã hỏi model nền
}

// PageStat — sổ một trang ký ức, chỉ con số (#5). Đếm theo PHIÊN, không theo đồng hồ:
// nhịp người dùng đo bằng phiên, và số nguyên không có múi giờ để lệch.
type PageStat struct {
	Opens        int `json:"opens"`
	FirstSession int `json:"first_session"`
	LastOpen     int `json:"last_open,omitempty"`
}

// Page — trang ký ức đã mở, nội dung ĐÓNG BĂNG lúc mở. Không đọc lại file vì trang đã mở
// nằm trong hội thoại tới hết phiên: đọc lại là đổi tiền tố và tự đốt cache. Turn là lượt
// người nó được mở, để khối của mỗi lượt chỉ kể phần của lượt ấy (xem OpenedAt).
type Page struct {
	Path string // "zone/tên.md" — cũng là nguồn gốc truy về được
	Text string
	Turn int
}

func (s *Session) Key() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.key
}

// setKey — chỉ Store gọi khi khoá phiên thăng cấp (có lượt trả lời đầu tiên).
func (s *Session) setKey(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.key = key
}
func (s *Session) Agent() string             { return s.agent }
func (s *Session) FirstSeen() time.Time      { return s.firstSeen }
func (s *Session) GapAtStart() time.Duration { return s.gapAtStart }

// Turns — số lần NGƯỜI nói trong phiên. Đây là con số mọi ngưỡng "đã đủ lượt chưa"
// phải đọc, và là con số soul hiểu khi nghe chữ "lượt".
func (s *Session) Turns() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.turns
}

// Runs — số lần proxy chạy. Đo nhịp máy: một lượt người có thể kéo hàng chục lần.
func (s *Session) Runs() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runs
}

// Opened — trang đã mở, giữ thứ tự mở. Thứ tự này là thứ tự chúng nằm trong khối chèn:
// đảo chỗ là đổi tiền tố, là mất cache.
func (s *Session) Opened() []Page {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Page(nil), s.opened...)
}

// OpenedAt — trang mở TRONG một lượt người. Khối của lượt được ghim lại (§ Cache), nên kể
// dồn là mỗi khối chở lại trọn phần khối trước đã chở.
func (s *Session) OpenedAt(turn int) []Page {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Page
	for _, p := range s.opened {
		if p.Turn == turn {
			out = append(out, p)
		}
	}
	return out
}

// Open ghi một trang vào phiên, đóng dấu lượt người đang chảy. Trả false khi đã mở — mở
// lại không có nghĩa: nó vẫn đang nằm trong hội thoại.
func (s *Session) Open(path, text string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.opened {
		if p.Path == path {
			return false
		}
	}
	s.opened = append(s.opened, Page{Path: path, Text: text, Turn: s.turns})
	s.openPages = append(s.openPages, path)
	return true
}

// SeePages khai những trang plugin thấy trong kho lượt này. Thấy khác mở: có mặt
// trên kệ không phải được cầm lên.
func (s *Session) SeePages(paths []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.seenPages == nil {
		s.seenPages = map[string]bool{}
	}
	for _, p := range paths {
		s.seenPages[p] = true
	}
}

func (s *Session) PageStats() (map[string]PageStat, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]PageStat, len(s.pageStats))
	for k, v := range s.pageStats {
		out[k] = v
	}
	return out, s.no
}

// MemIndex — index kho đã chốt cho phiên. `lines` rỗng nghĩa là chưa dựng lần nào.
func (s *Session) MemIndex() (lines string, paths []string, dropped int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.memLines, append([]string(nil), s.memPaths...), s.memDropped
}

// SetMemIndex chốt index cho trọn phiên. Gọi lần thứ hai không ghi đè: cái đã vào lời
// hệ thống rồi thì đổi nó là đổi tiền tố giữa lượt.
func (s *Session) SetMemIndex(lines string, paths []string, dropped int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.memLines != "" {
		return
	}
	s.memLines = lines
	s.memPaths = append([]string(nil), paths...)
	s.memDropped = dropped
}

// PickedAt — lượt người gần nhất bộ chọn đã chạy; 0 là chưa lần nào.
func (s *Session) PickedAt() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pickedAt
}

// SetPickedAt ghi nhận bộ chọn đã tiêu lần hỏi của lượt này. Không lùi: số lượt đọc
// từ request nên nó co lại được ở một lần chạy, và một cái sổ lùi được thì hỏi lại được.
func (s *Session) SetPickedAt(turn int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if turn > s.pickedAt {
		s.pickedAt = turn
	}
}

// Placed — một khối lõi đã chèn vào mảng hội thoại: chữ, ai nói, và VÂN TAY của message nó
// đứng ngay sau. Chỗ đứng nhận ra bằng vân tay chứ không bằng số đếm — công cụ chủ chen
// thêm một message vào đầu mảng là mọi chỉ số trượt, còn vân tay thì không (§ Cache).
// Plugin để cái công tắc còn là công tắc: tắt một plugin thì chữ cũ của nó cũng thôi về.
type Placed struct {
	Anchor uint64
	Plugin string
	Text   string
}

// Placed — sổ khối đã chèn, theo đúng thứ tự đặt. Bản sao: người gọi dựng lại mảng từ đây
// mỗi lần chạy, và một slice dùng chung thì một lần append của họ ghi đè sổ của phiên.
func (s *Session) Placed() []Placed {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.placed) == 0 {
		return nil
	}
	out := make([]Placed, len(s.placed))
	copy(out, s.placed)
	return out
}

// Place ghi một khối vừa chèn vào sổ. Cùng mỏ neo + cùng plugin + cùng chữ là CÙNG một
// khối, không phải khối thứ hai: một lượt người chạy lại y nguyên khi đích trả 502 hoặc
// client ngắt (§ Chuỗi đã có lời đáp), và cả hai lần đều qua cửa ghim. Không có luật này
// thì mỗi lần thử lại nhân thêm một bản — thân phồng lên và tiền tố gãy ngay chỗ đó.
func (s *Session) Place(anchor uint64, plugin, text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.placed {
		if p.Anchor == anchor && p.Plugin == plugin && p.Text == text {
			return
		}
	}
	s.placed = append(s.placed, Placed{Anchor: anchor, Plugin: plugin, Text: text})
}

// KeepPlaced giữ lại đúng những khối còn chỗ bám. Mất mỏ neo nghĩa là công cụ chủ đã xoá
// message khối ấy đứng sau, và lúc đó nó không còn chỗ nào đúng để về. Bỏ từng khối một
// chứ không quên sạch: một message biến mất không làm những message khác hết nghĩa.
func (s *Session) KeepPlaced(keep []Placed) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.placed = keep
}

// Note — lời nhắc về kho: tính MỘT LẦN mỗi phiên rồi nằm lại tới hết phiên.
func (s *Session) Note() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.note
}

func (s *Session) SetNote(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.note = text
}

// drainPages — Store gọi từ Touch để rút phần mới vào sổ bền.
func (s *Session) drainPages() (seen []string, opened []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for p := range s.seenPages {
		seen = append(seen, p)
	}
	s.seenPages = nil
	opened, s.openPages = s.openPages, nil
	return seen, opened
}

func (s *Session) HasOpened(path string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.opened {
		if p.Path == path {
			return true
		}
	}
	return false
}

// Said — dòng plugin đã nói trong phiên, đi vào prompt để soul thấy mình đã nói gì mà
// đừng nhắc lại. Luật "mỗi mốc một lần" thuộc soul, nên dữ kiện giữ nó phải tới tay soul.
func (s *Session) Said() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.said...)
}

func (s *Session) Say(line string) {
	if line == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.said = append(s.said, line)
}

// Asked — câu này đã hỏi trong phiên chưa; chưa thì ghi sổ rồi trả false. Theo phiên,
// vì phiên là đơn vị của một lần nhớ.
func (s *Session) Asked(plugin, question string) bool {
	h := fnv.New64a()
	h.Write([]byte(plugin))
	h.Write([]byte{'|'})
	h.Write([]byte(question))
	k := h.Sum64()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.asked == nil || len(s.asked) > askedMax {
		s.asked = map[uint64]bool{}
	}
	if s.asked[k] {
		return true
	}
	s.asked[k] = true
	return false
}

// PastTurns — số lượt NGƯỜI của các phiên trước cùng đệ, cũ trước mới sau. Cái
// "thường" để soul đo cái "bất thường"; lõi không tự phán.
func (s *Session) PastTurns() []int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]int(nil), s.pastTurns...)
}

func (s *Session) seenAt() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastSeen
}

// IdleFor — khoảng từ lượt gần nhất tới now. Nhịp trong phiên đo được ở đây, và
// Store dùng chính nó để dọn phiên nhàn rỗi.
func (s *Session) IdleFor(now time.Time) time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return now.Sub(s.lastSeen)
}

// touch cộng một lần chạy, và ĐẶT số lượt người bằng số đếm trong request vừa tới —
// không cộng dồn, vì request mang trọn lịch sử nên con số ấy tự đúng và tự sửa; rewind
// là đường một cái sổ cộng dồn sai mà không có lối về.
// humanTurns=0 → giữ số cũ, không hạ về 0.
func (s *Session) touch(marks []uint64, humanTurns int, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastSeen = now
	s.runs++
	// Chuỗi đổi = hội thoại đã đi tiếp, và lời đáp của chuỗi cũ không nói gì về chuỗi mới.
	if !sameMarks(s.chain, marks) {
		s.chain = append(s.chain[:0:0], marks...)
		s.answered, s.replyMark = false, 0
	}
	if humanTurns > 0 {
		s.turns = humanTurns
	}
}

// joinable — chuỗi hội thoại vừa tới có phải phiên này đi tiếp không. Nối dài, cắt ngắn,
// lệch: đều nhận, vì công cụ chủ có quyền viết lại lịch sử của chính nó (rewind, sửa lời cũ,
// nén phiên) mà vẫn là cuộc ấy.
//
// Một hình KHÔNG nhận: đúng chuỗi cũ quay lại sau khi lượt đứng ở đó ĐÃ có lời đáp. Một lượt
// đi tiếp phải mang lời đáp ấy theo, nên chuỗi y nguyên là một hội thoại KHÁC mở bằng đúng
// câu cũ — và khoá phiên dẫn xuất từ câu mở đầu thì hai cuộc ấy trùng khoá. Cho nó vào đây là
// sổ ghim, trang ký ức và sổ đã-hỏi của cuộc này chảy sang cuộc kia.
func (s *Session) joinable(marks []uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.answered || !sameMarks(s.chain, marks)
}

// Replied — lượt đứng ở chuỗi hiện tại đã có lời đáp thật từ đích, kèm vân tay của chính lời
// ấy. Không gọi khi đích lỗi hoặc client ngắt: chuỗi để ngỏ thì lần gửi lại y nguyên là một
// lần THỬ LẠI của chính cuộc này, và nó phải về đúng phiên cũ chứ không mở phiên mới.
func (s *Session) Replied(mark uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.answered, s.replyMark = true, mark
}

// produced — lời đứng ngay sau chuỗi của nhánh này có phải lời nhánh này vừa nói không.
// Đây là cửa duy nhất tách được hai nhánh đứng chung một chuỗi: request tới mang theo lời
// đáp của lượt trước, mà lời ấy thì mỗi nhánh một khác.
//
// Chưa đọc được lời nào (đích chỉ gọi tool, thân nén kiểu lạ, lời đáp dài quá khúc vòi giữ)
// → false. Không nhận ra thì không nhận vơ: bên gọi sẽ mở phiên mới (#2).
func (s *Session) produced(text func(int) string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.replyMark == 0 || text == nil {
		return false
	}
	return ReplyMark(text(len(s.chain))) == s.replyMark
}

// replyRunes — bao nhiêu ký tự đầu của lời đáp đủ để tách hai nhánh. Không cần cả lời: hai
// lời đáp khác nhau tách nhau từ rất sớm, còn giữ dài thì vòi phải ôm thêm thân.
const replyRunes = 120

// ReplyMark — vân tay khúc đầu một lời đáp. MỘT nhà cho cả hai bên so: lời lõi đọc từ
// response, và lời công cụ chủ gửi lên ở lượt sau. Hai cách tính là hai vân tay không bao
// giờ khớp. Rỗng → 0, và 0 nghĩa là "không biết", không bao giờ là một câu trả lời khớp.
func ReplyMark(text string) uint64 {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	if r := []rune(text); len(r) > replyRunes {
		text = string(r[:replyRunes])
	}
	h := fnv.New64a()
	h.Write([]byte(text))
	if m := h.Sum64(); m != 0 {
		return m
	}
	return 1 // 0 là chỗ dành cho "không biết"
}

func sameMarks(a, b []uint64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
