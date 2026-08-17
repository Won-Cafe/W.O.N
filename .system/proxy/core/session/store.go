// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package session

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Ngưỡng dọn phiên trong RAM và nhịp ghi state xuống đĩa.
const (
	sweepAt      = 256
	idleLimit    = 24 * time.Hour
	persistEvery = 2 * time.Second // debounce — dòng chính không đợi disk I/O
)

// idsKept — trần sổ danh tính, dọn cũ trước. Rộng hơn sweepAt vì mỗi dòng chỉ có ba con
// số, và một khoá bị dọn oan nghĩa là một hội thoại mất mốc mở khi khởi động lại.
const idsKept = 512

// persisted — phần tối thiểu sống qua restart: dấu thời gian để đo quãng vắng,
// số lượt các phiên để biết nhịp thường. Mất file không chặn; tắt đột ngột mất
// nhiều nhất một nhịp ghi cuối.
type persisted struct {
	LastSeenGlobal time.Time            `json:"last_seen_global"`
	LastSeenAgent  map[string]time.Time `json:"last_seen_by_agent"`
	TurnsByAgent   map[string][]int     `json:"session_turns_by_agent,omitempty"`
	SessionsTotal  int                  `json:"sessions_total,omitempty"`
	Pages          map[string]PageStat  `json:"memory_pages,omitempty"`
	Ids            map[string]ident     `json:"session_ids,omitempty"`
}

// ident — DANH TÍNH một phiên, phần duy nhất của phiên sống qua restart.
//
// Ranh giới, và nó là cả cái vá này: TRẠNG THÁI phiên (sổ ghim, chuỗi, trang đã mở) tả một
// mảng đang chảy nên chết theo tiến trình — đúng chủ ý. Còn mốc mở và số thứ tự là sự thật
// về HỘI THOẠI, không phải về tiến trình, nên chúng phải sống. Để chúng chết chung thì tắt
// rồi mở lại là cùng một hội thoại nhận mốc mở mới: thư mục nhật ký chẻ đôi, và khối House
// trong lời hệ thống đổi chữ giữa hội thoại — tức tiền tố cache gãy ở mọi đích (§ Cache).
//
// Ba con số, không một chữ hội thoại (#5).
type ident struct {
	FirstSeen time.Time `json:"first_seen"`
	No        int       `json:"no"`
	LastSeen  time.Time `json:"last_seen"` // chỉ để dọn cũ trước khi tới trần
}

// pastTurnsKept — giữ mấy phiên gần nhất mỗi đệ. Đủ thấy nhịp thường, không thành
// biên niên. Chỉ con số, không nội dung (#5).
const pastTurnsKept = 8

// Store giữ session trong RAM và persist phần tối thiểu xuống đĩa, ghi nền theo
// nhịp thay vì đồng bộ từng request.
type Store struct {
	mu sync.Mutex // bảo vệ mọi field dưới đây

	p       persisted
	dirty   bool
	path    string
	lastKey map[string]string // đệ → khoá phiên gần nhất, để biết phiên nào vừa đóng
	// Một khoá giữ NHIỀU phiên: khoá dẫn xuất từ câu mở đầu, nên hai hội thoại mở bằng cùng
	// một câu về chung một khoá. Chúng là hai nhánh đứng cạnh nhau ở đó, không phải một.
	sessions map[string][]*Session
}

func NewStore(path string) *Store {
	st := &Store{
		path:     path,
		sessions: map[string][]*Session{},
		lastKey:  map[string]string{},
		p: persisted{LastSeenAgent: map[string]time.Time{}, TurnsByAgent: map[string][]int{},
			Pages: map[string]PageStat{}, Ids: map[string]ident{}},
	}
	st.load(path)
	go st.flushLoop() // sống trọn đời tiến trình
	return st
}

// NewEphemeral — sổ phiên KHÔNG bao giờ chạm đĩa và không có vòng ghi. Dùng cho lượt
// chạy khô: một lượt thử phải không đọc được nhịp phiên thật, và tuyệt đối không ghi lên
// nó. Dùng NewStore("") thay cho hàm này thì mỗi lượt thử để lại một goroutine ghi vào
// đường dẫn rỗng, mỗi hai giây một lỗi.
func NewEphemeral() *Store {
	return &Store{
		sessions: map[string][]*Session{},
		lastKey:  map[string]string{},
		p: persisted{LastSeenAgent: map[string]time.Time{}, TurnsByAgent: map[string][]int{},
			Pages: map[string]PageStat{}, Ids: map[string]ident{}},
	}
}

func (st *Store) load(path string) {
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &st.p) // state hỏng → bắt đầu trắng — fail-open
		if st.p.LastSeenAgent == nil {
			st.p.LastSeenAgent = map[string]time.Time{}
		}
		if st.p.TurnsByAgent == nil {
			st.p.TurnsByAgent = map[string][]int{}
		}
		if st.p.Pages == nil {
			st.p.Pages = map[string]PageStat{}
		}
		if st.p.Ids == nil {
			st.p.Ids = map[string]ident{}
		}
	}
}

// Touch tìm hoặc mở session, ghi nhận lượt hiện tại. fallback là khoá chưa
// thăng cấp: hội thoại vừa có lượt trả lời đầu thì session cũ dời sang khoá mới.
// humanTurns là số lượt người đếm được TRONG request này — Store không giữ chữ nào
// của hội thoại (#5), chỉ nhận con số.
//
// in là hình hội thoại của request, chụp TRƯỚC khi lõi chèn gì. Khoá nói hai hội thoại này
// CÓ THỂ là một; hình ấy nói chúng có THẬT là một không (xem pick).
func (st *Store) Touch(key, fallback, agent string, in Reach, humanTurns int, now time.Time) *Session {
	st.mu.Lock()
	// Khoá do người khai: MỘT khoá là MỘT phiên. Không dò, không tách nhánh — và không có
	// khoá chưa thăng cấp để dời sang, vì khai rồi thì khoá không đổi giữa hội thoại.
	s := declaredIn(st.sessions[key], in)
	if s == nil && !in.Declared && fallback != "" {
		if prev := pick(st.sessions[fallback], in); prev != nil {
			st.sessions[fallback] = drop(st.sessions[fallback], prev)
			if len(st.sessions[fallback]) == 0 {
				delete(st.sessions, fallback)
			}
			prev.setKey(key)
			st.sessions[key] = append(st.sessions[key], prev)
			// Danh tính đi theo phiên sang khoá mới. Để lại bản cũ dưới khoá chưa thăng cấp thì
			// một hội thoại KHÁC mở bằng đúng câu chào ấy sẽ nhặt phải mốc mở của cuộc này.
			if id, ok := st.p.Ids[fallback]; ok {
				delete(st.p.Ids, fallback)
				st.p.Ids[key] = id
			}
			s = prev
		}
	}
	if s == nil {
		// Quãng vắng đo theo dấu thời gian của đệ đang mở phiên, không phải global.
		// Chưa thấy đệ → fallback global. Cả hai zero → gap 0 (khởi động lạnh).
		var base time.Time
		if a, ok := st.p.LastSeenAgent[agent]; ok && !a.IsZero() {
			base = a
		} else if !st.p.LastSeenGlobal.IsZero() {
			base = st.p.LastSeenGlobal
		}
		var gap time.Duration
		if !base.IsZero() {
			gap = now.Sub(base)
		}
		// Sổ danh tính chỉ được nhặt khi khoá này KHÔNG còn nhánh nào sống: hết nhánh nghĩa
		// là tiến trình vừa khởi động lại và đây là cuộc cũ đi tiếp. Còn nhánh mà không nhánh
		// nào nhận request này thì đây là hội thoại KHÁC trùng khoá — nó phải có mốc mở riêng,
		// không thì cái vá này xoá đúng ranh giới mà mốc mở sinh ra để giữ (§ debug).
		id, resumed := ident{}, false
		if len(st.sessions[key]) == 0 {
			id, resumed = st.p.Ids[key]
		}
		if !resumed {
			st.p.SessionsTotal++
			id = ident{FirstSeen: now, No: st.p.SessionsTotal}
		}
		s = &Session{key: key, agent: agent, firstSeen: id.FirstSeen, gapAtStart: gap,
			pastTurns: st.closePrevLocked(agent, key),
			no:        id.No, pageStats: st.pagesLocked()}
		st.sessions[key] = append(st.sessions[key], s)
		st.sweepLocked(now)
		st.pruneIdsLocked()
	}
	st.lastKey[agent] = s.key
	st.mergePagesLocked(s)
	// Đóng dấu danh tính ở MỌI lượt, không chỉ lúc mở: firstSeen/no bất biến nên hai field
	// đầu chỉ ghi lại chính nó, còn LastSeen là cái giữ khoá này khỏi bị dọn trong lúc hội
	// thoại còn chảy. Đọc `key` chứ không `s.key` — cùng một giá trị ở cả ba đường vào, và
	// s.key thì phải qua khoá của Session.
	st.p.Ids[key] = ident{FirstSeen: s.firstSeen, No: s.no, LastSeen: now}
	st.p.LastSeenGlobal = now
	if agent != "" {
		st.p.LastSeenAgent[agent] = now
	}
	st.dirty = true
	st.mu.Unlock()

	s.touch(in.Marks, humanTurns, now)
	return s
}

// declaredIn — cửa vào của một khoá: lời khai thì lấy thẳng phiên đang ở đó, không khai thì
// mới dò. Một hàm cho cả hai đường để chỗ gọi không phải nhớ rẽ nhánh.
func declaredIn(branches []*Session, in Reach) *Session {
	if in.Declared {
		return latest(branches) // khai rồi thì bucket chỉ có một; latest là phòng khi khoá khai trùng khoá dò
	}
	return pick(branches, in)
}

// pick chọn nhánh của chuỗi này trong một khoá. Không nhánh nào nhận → nil, và người gọi mở
// phiên mới: đó là đường một hội thoại KHÁC mở bằng đúng câu cũ đi ra.
//
// Nhiều nhánh cùng nhận được thì HỎI LỜI ĐÁP. Hình này có sau một lần tách, khi hai cuộc còn
// đứng chung một chuỗi: chuỗi không tách được chúng, nhưng lời đáp thì có — request tới mang
// theo lời đáp của lượt trước, và lời ấy mỗi nhánh một khác. Không nhánh nào nhận ra lời của
// mình, hoặc hai nhánh cùng nhận, thì mới trả nil: không nhận ra thì không nhận vơ (#2).
func pick(branches []*Session, in Reach) *Session {
	var out *Session
	many := false
	for _, s := range branches {
		if !s.joinable(in.Marks) {
			continue
		}
		if out != nil {
			many = true
			break
		}
		out = s
	}
	if !many {
		return out
	}
	out = nil
	for _, s := range branches {
		if !s.joinable(in.Marks) || !s.produced(in.Text) {
			continue
		}
		if out != nil {
			return nil
		}
		out = s
	}
	return out
}

func drop(branches []*Session, s *Session) []*Session {
	out := branches[:0]
	for _, b := range branches {
		if b != s {
			out = append(out, b)
		}
	}
	return out
}

// latest — nhánh vừa chạy gần nhất trong một khoá; nil nếu khoá rỗng.
func latest(branches []*Session) *Session {
	var out *Session
	for _, s := range branches {
		if out == nil || s.seenAt().After(out.seenAt()) {
			out = s
		}
	}
	return out
}

// closePrevLocked chốt số lượt phiên trước cùng đệ rồi trả lịch sử cho phiên mới.
// "Đóng" là suy ra: một phiên coi như xong khi đệ đó mở phiên khác.
func (st *Store) closePrevLocked(agent, newKey string) []int {
	if prevKey := st.lastKey[agent]; prevKey != "" && prevKey != newKey {
		if prev := latest(st.sessions[prevKey]); prev != nil {
			if n := prev.Turns(); n > 0 {
				h := append(st.p.TurnsByAgent[agent], n)
				if len(h) > pastTurnsKept {
					h = h[len(h)-pastTurnsKept:]
				}
				st.p.TurnsByAgent[agent] = h
			}
		}
	}
	return append([]int(nil), st.p.TurnsByAgent[agent]...)
}

func (st *Store) pagesLocked() map[string]PageStat {
	out := make(map[string]PageStat, len(st.p.Pages))
	for k, v := range st.p.Pages {
		out[k] = v
	}
	return out
}

// mergePagesLocked cộng phần mới của phiên vào sổ bền: trang mới thấy lần đầu thì
// ghi nhận phiên xuất hiện, trang vừa mở thì cộng một lần mở.
func (st *Store) mergePagesLocked(s *Session) {
	seen, opened := s.drainPages()
	if len(seen) == 0 && len(opened) == 0 {
		return
	}
	for _, p := range seen {
		if _, ok := st.p.Pages[p]; !ok {
			st.p.Pages[p] = PageStat{FirstSession: s.no}
		}
	}
	for _, p := range opened {
		stat := st.p.Pages[p]
		if stat.FirstSession == 0 {
			stat.FirstSession = s.no
		}
		stat.Opens++
		stat.LastOpen = s.no
		st.p.Pages[p] = stat
	}
	st.dirty = true
}

func (st *Store) sweepLocked(now time.Time) {
	if len(st.sessions) < sweepAt {
		return
	}
	for k, branches := range st.sessions {
		kept := branches[:0]
		for _, s := range branches {
			if s.IdleFor(now) <= idleLimit {
				kept = append(kept, s)
			}
		}
		if len(kept) == 0 {
			delete(st.sessions, k)
			continue
		}
		st.sessions[k] = kept
	}
}

// pruneIdsLocked giữ sổ danh tính dưới trần, bỏ khoá nằm im lâu nhất trước. Không dọn theo
// idleLimit như phiên trong RAM: dọn RAM là chuyện chỗ chứa, còn ở đây một khoá bị bỏ nghĩa
// là một hội thoại quay lại sau vài ngày mất mốc mở của chính nó — mà mốc ấy vẫn đúng.
func (st *Store) pruneIdsLocked() {
	if len(st.p.Ids) <= idsKept {
		return
	}
	keys := make([]string, 0, len(st.p.Ids))
	for k := range st.p.Ids {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return st.p.Ids[keys[i]].LastSeen.Before(st.p.Ids[keys[j]].LastSeen)
	})
	for _, k := range keys[:len(keys)-idsKept] {
		delete(st.p.Ids, k)
	}
}

func (st *Store) flushLoop() {
	t := time.NewTicker(persistEvery)
	defer t.Stop()
	for range t.C {
		st.Flush()
	}
}

// Flush ghi state xuống đĩa nếu có thay đổi. Ghi hỏng không chặn dòng chính,
// nhưng có log, nhịp sau thử lại.
func (st *Store) Flush() {
	st.mu.Lock()
	defer st.mu.Unlock()
	// Gom sổ trang trước khi kiểm dirty: plugin mở trang SAU Touch, nên phần mới
	// nhất chỉ về tới đây.
	for _, branches := range st.sessions {
		for _, s := range branches {
			st.mergePagesLocked(s)
		}
	}
	if !st.dirty {
		return
	}
	b, err := json.MarshalIndent(st.p, "", "  ")
	if err != nil {
		slog.Error("session: marshal state error", "err", err)
		return
	}
	if err := os.WriteFile(st.path, b, 0o644); err != nil {
		// Thư mục cha có thể chưa có (lần chạy đầu, .system/proxy/run chưa tạo).
		// os.WriteFile không tự tạo thư mục cha, nên tạo rồi thử lại một lần.
		if mkErr := os.MkdirAll(filepath.Dir(st.path), 0o755); mkErr != nil {
			slog.Error("session: write error", "path", st.path, "err", err)
			slog.Error("session: mkdir error", "dir", filepath.Dir(st.path), "err", mkErr)
			return
		}
		if err = os.WriteFile(st.path, b, 0o644); err != nil {
			slog.Error("session: write error", "path", st.path, "err", err)
			return
		}
	}
	st.dirty = false
}

// Key dẫn xuất khoá phiên khi client không gửi header. Mỏ neo: đệ + lượt user
// đầu; khi có lượt trả lời đầu, khoá thăng cấp thêm mỏ neo đó.
func Key(headerKey, agent, firstUser, firstAssistant string) (key, fallback string) {
	if headerKey != "" {
		return headerKey, ""
	}
	base := derive(agent, firstUser)
	if firstAssistant == "" {
		return base, ""
	}
	return derive(agent, firstUser, firstAssistant), base
}

func derive(parts ...string) string {
	h := fnv.New64a()
	for i, p := range parts {
		if i > 0 {
			h.Write([]byte{'|'})
		}
		h.Write([]byte(p))
	}
	return fmt.Sprintf("s-%x", h.Sum64())
}
