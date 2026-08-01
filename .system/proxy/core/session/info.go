// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package session

import (
	"sort"
	"time"
)

// Info — ảnh chụp một phiên cho Control API, chỉ metadata. recent không rời Session (#5).
type Info struct {
	Key       string    `json:"key"`
	Agent     string    `json:"agent"`
	Turns     int       `json:"turns"`
	Runs      int       `json:"runs"`
	FirstSeen time.Time `json:"first_seen"`
	IdleS     int64     `json:"idle_s"`
	Said      int       `json:"said,omitempty"`   // số dòng agent bờ đã nói, không phải nội dung (#5)
	Opened    int       `json:"opened,omitempty"` // số trang ký ức đã mở
}

func (s *Session) info(now time.Time) Info {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Info{
		Key:       s.key,
		Agent:     s.agent,
		Turns:     s.turns,
		Runs:      s.runs,
		FirstSeen: s.firstSeen,
		IdleS:     int64(now.Sub(s.lastSeen).Seconds()),
		Said:      len(s.said),
		Opened:    len(s.opened),
	}
}

// Infos — ảnh chụp mọi phiên, sắp theo khoá rồi theo mốc mở cho output ổn định. Một khoá giữ
// được nhiều nhánh, nên khoá không đủ để xếp: hai hội thoại mở bằng cùng một câu đứng chung
// một khoá và chỉ mốc mở tách chúng ra.
func (st *Store) Infos(now time.Time) []Info {
	st.mu.Lock()
	sessions := make([]*Session, 0, len(st.sessions))
	for _, branches := range st.sessions {
		sessions = append(sessions, branches...)
	}
	st.mu.Unlock()

	out := make([]Info, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, s.info(now))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Key != out[j].Key {
			return out[i].Key < out[j].Key
		}
		return out[i].FirstSeen.Before(out[j].FirstSeen)
	})
	return out
}
