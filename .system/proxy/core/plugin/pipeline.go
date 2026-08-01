// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package plugin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"won/proxy/core/request"
	"won/proxy/core/session"
)

// Sáu kết cục. skipped và silent tách nhau: chưa hỏi model, và hỏi rồi không ra lời.
const (
	StatusError       = "error"
	StatusPanic       = "panic"
	StatusSilent      = "silent"  // đã chạy xong, không ra lời dùng được
	StatusSkipped     = "skipped" // cổng tiếng-của-lượt đóng — KHÔNG chạy, không tốn gì
	StatusTimeout     = "timeout"
	StatusContributed = "contributed"
)

// PluginDetail — kết quả một plugin trong Gather, cho debug log.
type PluginDetail struct {
	Name    string `json:"name"`
	Ms      int64  `json:"ms"`
	Status  string `json:"status"`
	Kind    string `json:"kind,omitempty"`
	Text    string `json:"text,omitempty"` // lời chèn; bản sắc chỉ đếm, không in
	TextLen int    `json:"text_len,omitempty"`
	Calls   []Call `json:"calls,omitempty"`
	Err     string `json:"err,omitempty"`
}

// Gather chạy plugin song song, mỗi plugin một ngân sách. Kết quả giữ thứ tự danh
// sách vào để chèn tất định. Lỗi, panic, quá giờ → im lặng, có log (#2).
func Gather(ctx context.Context, plugins []Plugin, snap *request.Snapshot, sess *session.Session) ([]Contribution, []PluginDetail) {
	results := make([][]*Contribution, len(plugins))
	details := make([]PluginDetail, len(plugins))
	var wg sync.WaitGroup
	for i, p := range plugins {
		// Tiếng của lượt: người chưa nói lại thì không gọi. Cửa đóng TRƯỚC goroutine
		// vì chỗ tốn kém nằm bên trong (§ TurnVoice). Không đọc được hình request →
		// không chặn: cửa này để bớt việc, không phải để phán (#2).
		if snap != nil && !snap.HumanSpokeLast && turnVoice(p) {
			details[i] = PluginDetail{Name: p.Name(), Status: StatusSkipped}
			continue
		}
		wg.Add(1)
		go func(i int, p Plugin) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					slog.Warn("plugin: panic — falls back to silence", "plugin", p.Name(), "panic", r)
					details[i] = PluginDetail{Name: p.Name(), Status: StatusPanic, Err: fmt.Sprintf("%v", r)}
				}
			}()
			pctx, cancel := budgetFor(ctx, p)
			defer cancel() // hoãn: panic thì ctx vẫn phải được huỷ
			var tr *Trace
			if tracing(ctx) {
				tr = &Trace{}
				pctx = withTrace(pctx, tr)
			}
			pluginStart := time.Now()
			cs, err := p.Contribute(pctx, snap, sess)
			d := PluginDetail{Name: p.Name(), Ms: time.Since(pluginStart).Milliseconds(), Calls: tr.Calls()}
			switch {
			case err != nil:
				d.Status, d.Err = StatusError, err.Error()
				if timedOut(err) {
					d.Status = StatusTimeout
				}
				// Trạng thái đi vào attr, không vào msg: msg dựng bằng nối chuỗi thì mỗi
				// trạng thái đẻ một chữ khác nhau cho cùng một sự kiện, và không grep được.
				slog.Warn("plugin: falls back to silence", "plugin", p.Name(), "status", d.Status, "err", err, "took", d.Ms)
			default:
				// Phần tử nil hoặc rỗng chữ không tới được body, nên đếm nó là khai một
				// đóng góp không có. Một plugin nói ở hai nhịp thì cả hai cùng mang tên nó.
				var kept []*Contribution
				var kinds []string
				for _, c := range cs {
					if c == nil || c.Text == "" {
						continue
					}
					c.Plugin = p.Name()
					kept = append(kept, c)
					kinds = append(kinds, KindName(c.Kind))
					d.TextLen += len(c.Text)
					// Bản sắc chỉ đếm: trọn soul file, in ra là lấp nhật ký.
					if c.Kind != KindSystem {
						d.Text += c.Text
					}
				}
				if len(kept) == 0 {
					// Chạy xong mà không ra lời là `silent`, kể cả khi không hỏi model lần nào:
					// "có hỏi model không" là sự thật khác, và nó nằm ở `calls`.
					d.Status = StatusSilent
					slog.Debug("plugin: silent", "plugin", p.Name(), "calls", len(d.Calls), "took", d.Ms)
					break
				}
				results[i] = kept
				d.Status, d.Kind = StatusContributed, strings.Join(kinds, "+")
				slog.Debug("plugin: contributed", "plugin", p.Name(), "kind", d.Kind, "took", d.Ms)
			}
			details[i] = d
		}(i, p)
	}
	wg.Wait()
	var out []Contribution
	for _, cs := range results {
		for _, c := range cs {
			out = append(out, *c)
		}
	}
	return out, details
}

// Apply áp đóng góp lên body và trả về chữ đã đặt vào messages (nhật ký cần biết chữ
// nào là của lõi). Cưỡng chế #4: text không mang tên plugin thì bỏ; KindSystem miễn
// kiểm. Thứ tự khối system = thứ tự contribs = thứ tự plugin được dựng, và đảo nó là
// đổi tiền tố cache cả phiên (config.EnabledPlugins).
//
// Tiếng của lượt được GHIM: khối chèn ở lượt trước về lại đúng chỗ cũ ở mọi lần chạy sau.
// Công cụ chủ không giữ chữ ta chèn, nên không ghim thì thân đi ra là bản SỬA của lần
// trước chứ không phải bản NỐI. Sổ ghim sống ở phiên (§ Cache).
func Apply(body *request.Body, contribs []Contribution, sess *session.Session, active []Plugin) []string {
	// Nhịp phiên — vào cuối vùng lời hệ thống, sau khối của công cụ chủ. Duyệt THUẬN:
	// chèn cuối thì thứ tự nằm lại đúng thứ tự contribs, không cần đảo chiều.
	for _, c := range contribs {
		if c.Kind == KindSystem && c.Text != "" {
			body.AppendSystem(request.Wrap(c.Tag, c.Text))
		}
	}
	if sess == nil {
		return nil
	}
	// Nhịp lượt — mỗi tiếng một message riêng, vai `user`, ngay sau lượt người. Một hình,
	// một chỗ, một vai (§ Chỗ đứng của tiếng lượt). Cửa ở lõi, không chỉ ở plugin: plugin
	// quên khai `TurnVoice` cũng không chèn được vào giữa một vòng tool. Cửa này chặn khối
	// MỚI; khối cũ vẫn về chỗ nó, vì nó đã ở đó rồi.
	//
	// Mỏ neo lấy TRƯỚC khi chèn: message cuối lúc này là lượt của người (HumanSpokeLast),
	// chèn xong thì message cuối đã là khối của chính ta.
	if body.HumanSpokeLast() {
		anchor := body.MessageMark(body.MessageCount() - 1)
		// Kiểm #4 chạy TRƯỚC khi bọc: tag là căn cước thứ hai, không phải chỗ trú cho một
		// đóng góp vô danh — bọc trước rồi kiểm thì mọi lời đều "có nguồn" nhờ chính cái tag.
		for _, c := range contribs {
			if c.Kind != KindMarker || c.Text == "" || !attributed(c.Plugin, c.Text) {
				continue
			}
			sess.Place(anchor, c.Plugin, request.Wrap(c.Tag, c.Text))
		}
	}
	return body.PlaceMessages(resolve(body, sess, names(active)))
}

// resolve dò lại chỗ đứng của từng khối trong sổ ghim, và dọn sổ theo đúng cái dò được.
// Quét THUẬN một lượt vì sổ và mỏ neo cùng một thứ tự; nó cũng là cách xử đúng khi hai
// message trùng byte — khối sau bám vào lần xuất hiện sau, không nhảy ngược.
func resolve(body *request.Body, sess *session.Session, speaking map[string]bool) []request.Placement {
	ledger := sess.Placed()
	if len(ledger) == 0 {
		return nil
	}
	marks := body.MessageMarks()
	places := make([]request.Placement, 0, len(ledger))
	keep := make([]session.Placed, 0, len(ledger))
	at := 0
	for _, p := range ledger {
		i := indexFrom(marks, p.Anchor, at)
		if i < 0 {
			continue // message nó đứng sau không còn — khối này hết chỗ đúng để về
		}
		at = i
		keep = append(keep, p)
		// Plugin đang tắt thì chữ cũ của nó cũng thôi về — công tắc phải tắt được thật. Vẫn
		// giữ trong sổ, nên vặn lại là chữ về đúng chỗ cũ.
		if p.Plugin != "" && !speaking[p.Plugin] {
			continue
		}
		places = append(places, request.Placement{After: i + 1, Text: p.Text})
	}
	// Dọn sổ chỉ khi CÒN dò ra thứ gì. Mất một vài mỏ neo là công cụ chủ vừa sửa lịch sử —
	// những khối ấy hết chỗ đúng để về, bỏ là đúng. Mất SẠCH thì câu đọc được không phải
	// "hội thoại vừa bị viết lại trọn" mà là "thân này không phải hội thoại ấy": một lượt
	// việc nhà của công cụ chủ rơi trúng khoá phiên là đủ. Xoá theo nó là lượt thật kế tiếp
	// hết đường tái dựng, và thân đi ra thành bản SỬA — im lặng, không ai kêu.
	//
	// Giữ lại thì tệ nhất là vài khối nằm chết trong sổ: mỏ neo không còn thì chúng không
	// bao giờ dò ra chỗ nào, tức không bao giờ chèn được. Hỏng về phía im lặng (#2).
	if len(keep) > 0 {
		sess.KeepPlaced(keep)
	}
	return places
}

// indexFrom — chỗ đầu tiên từ `from` trở đi mang vân tay này; -1 nếu không còn.
func indexFrom(marks []uint64, want uint64, from int) int {
	for i := from; i < len(marks); i++ {
		if marks[i] == want {
			return i
		}
	}
	return -1
}

func names(active []Plugin) map[string]bool {
	out := make(map[string]bool, len(active))
	for _, p := range active {
		out[p.Name()] = true
	}
	return out
}

// timedOut — quá giờ khác lỗi thật: ngân sách hết là nhịp, lỗi là mạch.
func timedOut(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

// attributed kiểm #4 bằng cơ chế: text phải chứa tên plugin đóng góp.
func attributed(name, text string) bool {
	if strings.Contains(strings.ToLower(text), strings.ToLower(name)) {
		return true
	}
	// Debug chứ không warn: lõi đã đeo marker (`base.Say`) nên đây chỉ là lưới đỡ cho plugin
	// tự dựng chữ (#4), và chỉ tác giả plugin sửa được.
	slog.Debug("plugin: contribution missing attribution — dropped", "plugin", name)
	return false
}

// KindName — tên đọc được của một Kind, cho nhật ký.
func KindName(k Kind) string {
	switch k {
	case KindSystem:
		return "system"
	case KindMarker:
		return "marker"
	default:
		return "unknown"
	}
}
