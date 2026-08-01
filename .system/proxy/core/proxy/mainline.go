// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package proxy

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"won/proxy/core/plugin"
	"won/proxy/core/proxy/debug"
	"won/proxy/core/request"
	"won/proxy/core/session"
	"won/proxy/core/usage"
)

const (
	houseTag  = "House" // bản đồ hệ
	groundTag = "W.O.N" // đất mọi đệ đứng, gộp từ các file khai trong `ground`
)

// agentFor — ai đứng lên request này, và nếu không ai thì vì sao. Ba đường, HAI HẠNG:
//
//   - header `X-WON-Agent` — lời khai của người. Tin, không đo: người đã nói đây là lượt của
//     đệ nào thì lõi không đo lại lời khai ấy.
//   - tiêu đề soul trong lời hệ thống, rồi `default_agent` — lõi TỰ nhận ra. Nhiều công cụ
//     chủ không có chỗ chọn đệ (Claude Code), và bắt người dán trọn soul mỗi lần gọi là thủ
//     tục rườm rà cho việc lẽ ra tự chạy. Nhưng cái lõi tự nhận ra chỉ được bước vào lượt có
//     HÌNH một lượt của đệ.
//
// Ba cửa, ba câu hỏi: có lời dạy không, có cuộc nào đang chảy không, và lời đáp xin về là cho
// người hay cho máy (§ Lượt việc nhà). Công cụ chủ gọi lượt việc nhà — dò quota, đặt tiêu đề,
// dò treo, chấm điểm — và chèn đất vào đó là 180KB đổi lấy không gì. Đắt hơn tiền: một thân
// KHÔNG PHẢI hội thoại ấy đi vào phiên là sổ ghim mất chỗ bám, và lượt thật kế tiếp hết đường
// tái dựng (§ Session).
//
// Rỗng = không ai đứng, và lượt ấy đi qua nguyên bản.
func agentFor(d Deps, header string, body *request.Body) (agent, why string) {
	if declared := d.Identity.Resolve(header, ""); declared != "" {
		return declared, ""
	}
	if agent = d.Identity.Resolve("", body.SystemText()); agent == "" {
		// Mỏ neo lời giao việc: cuộc này do một đệ GIAO, không phải cuộc người mở. Cuộc được
		// giao mà không khai nổi căn cước thì nó là tay việc của dòng điều phối, không phải
		// một đệ đứng trước người — gán đệ mặc định vào đó là dạy một tay việc rằng nó là
		// người điều phối, chọi thẳng việc nó đang làm.
		//
		// Chỉ chặn NHÁNH NÀY. Đường tiêu đề soul ở trên vẫn chạy: đệ được giao mà mang soul
		// thật trong lời hệ thống thì vẫn nhận đúng bản sắc của nó — và phải thế, vì lượt
		// được giao cũng là lượt của một đệ.
		if body.DispatchMarked(d.FrameRules) {
			return "", "dispatched turn"
		}
		agent = d.DefaultAgent
	}
	if agent == "" {
		return "", "no agent"
	}
	if !body.AgentTurnShaped(d.FrameRules) || !body.ConversationShaped() || body.MachineReply() {
		return "", "housekeeping turn"
	}
	return agent, ""
}

// MsgRunStart — bản ghi đầu tiên của một lần chạy dòng chính. Export để console tách khối
// theo nó; lõi chỉ nói ra tên, main nối hai đầu (§ Quan sát được).
const MsgRunStart = "proxy: mainline"

// serveMainline — dòng chính. Mọi nhánh hỏng rơi về forwardRaw (#2).
func (p *Proxy) serveMainline(w http.ResponseWriter, r *http.Request, rp *httputil.ReverseProxy, target *url.URL, format request.Format) {
	start := time.Now()
	slog.Debug(MsgRunStart, "method", r.Method, "path", r.URL.Path, "format", format, "target", target.Host)

	dbg := debug.NewCollector(p.debug, start, r, format, target.Host, r.Header.Get(request.HeaderAgent))

	raw, err := io.ReadAll(r.Body)
	r.Body.Close()
	dbg.Stage("read_body", start, func(s *debug.Stage) { s.BytesIn = len(raw) })
	// Chụp trước mọi nhánh: hỏng ở chặng nào thì "công cụ chủ gửi lên cái gì" vẫn còn.
	dbg.SnapshotIn(raw)
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		dbg.Fail("read body: " + err.Error())
		p.debug.Write(dbg)
		return
	}

	// Sổ usage đi theo request; vòi ở ModifyResponse ghi vào đây, mainline đọc sau khi forward.
	us := usage.New(p.debug != nil)
	r = r.WithContext(usage.NewContext(r.Context(), us))
	dbg.Usage = us

	t := time.Now()
	body, err := request.ParseBody(raw, format)
	if err != nil {
		dbg.Stage("parse", t, func(s *debug.Stage) { s.Failed = true })
		dbg.Fail("parse: " + err.Error())
		p.debug.Write(dbg)
		p.forwardRaw(w, r, rp, raw, "parse error", err)
		return
	}
	dbg.Stage("parse", t, nil)

	// Resolve trước khi cắt khung — cần system text gốc để khớp tiêu đề soul.
	t = time.Now()
	agent, why := agentFor(p.d, r.Header.Get(request.HeaderAgent), body)
	dbg.Agent = agent
	dbg.Stage("identity", t, nil)

	// Vắng đệ → chuyển tiếp nguyên bản: đất là chỗ đệ đứng, vắng đệ thì không ai đứng lên
	// nó (#6).
	if agent == "" {
		slog.Debug("proxy: passthrough", "reason", why, "path", r.URL.Path, "bytes", len(raw))
		p.forwardRaw(w, r, rp, raw, "", nil)
		return
	}

	// Khung công cụ chủ: luôn ĐỌC. Cắt khi có plugin làm chủ lời hệ thống — cắt lời dặn
	// của công cụ chủ mà KHÔNG đặt gì vào chỗ đó là lỗ ròng: đệ mất hướng dẫn và không
	// được gì. Cắt cái gì thì người giữ hệ khai (strip_tags/unwrap_tags): lõi không tự
	// biết chữ nào của công cụ chủ là nhiễu (#6).
	t = time.Now()
	active := p.activePlugins()
	frame := body.HostFrame(p.d.FrameRules)
	owned := plugin.SystemOwned(active)
	modified := false
	if owned {
		modified = body.StripFrame(frame)
	}
	dbg.Stage("host_frame", t, nil)
	dbg.Framed, dbg.SystemOwned = frame.Present, owned

	snap := body.Snapshot(p.d.FrameRules)
	snap.Agent = agent

	t = time.Now()
	// Vân tay mảng message chụp TRƯỚC khi lõi chèn gì: đây là hội thoại của công cụ chủ, cái
	// duy nhất so được giữa hai lần chạy. Khoá nói hai hội thoại này CÓ THỂ là một — nó dẫn
	// xuất từ câu mở đầu; chuỗi này nói chúng có THẬT là một không (§ Session).
	declared := r.Header.Get(request.HeaderSession)
	reach := session.Reach{
		Declared: declared != "",
		Marks:    body.MessageMarks(),
		Text:     body.MessageTextAt,
	}
	key, fallback := session.Key(declared, snap.Agent, snap.FirstUser, snap.FirstAssistant)
	snap.SessionKey = key
	sess := p.d.Store.Touch(key, fallback, snap.Agent, reach, snap.HumanTurns, start)
	dbg.Session, dbg.PrevSession = key, fallback
	dbg.Opened = sess.FirstSeen().Format("20060102-150405")
	dbg.Stage("session", t, nil)

	t = time.Now()
	ctx, cancel := context.WithTimeout(r.Context(), p.d.TotalBudget)
	if p.debug != nil {
		ctx = plugin.WithTracing(ctx) // prompt vào / lời đáp ra của model nền
	}
	contribs, details := plugin.Gather(ctx, active, snap, sess)
	cancel()
	dbg.Stage("gather", t, func(s *debug.Stage) { s.Plugins = details })

	t = time.Now()
	// Chèn theo ĐÚNG thứ tự đọc, vào cuối vùng lời hệ thống: W.O.N → House → Soul →
	// Memory, tất cả sau khối của công cụ chủ (§ Format wire). Kiểm "đã có chưa" trên
	// snap.System — ảnh chụp trước khi ta chèn gì, nên một lần làm phẳng đủ cho cả hai.
	ground := p.appendBlock(body, snap.System, groundTag, p.d.Ground)
	house := p.appendBlock(body, snap.System, houseTag, p.houseText(sess.FirstSeen()))
	if house || ground {
		modified = true
	}
	placed := plugin.Apply(body, contribs, sess, active)
	// Khối ghim từ lượt trước cũng là sửa thân, nên `modified` phải hỏi cả nó: lượt không
	// có đóng góp mới mà bỏ qua Marshal thì thân đi ra rụng sạch chữ đã ghim.
	if len(contribs) > 0 || len(placed) > 0 {
		modified = true
	}
	dbg.Stage("apply", t, func(s *debug.Stage) { s.Inserted = len(contribs) })
	dbg.Ground, dbg.House = ground, house
	dbg.ReadBody(body, placed, p.d.FrameRules)

	// Body không đổi → chuyển tiếp raw, bỏ hẳn Marshal (tối ưu, không phá lossless #7).
	out := raw
	if modified {
		t = time.Now()
		nb, err := body.Marshal()
		if err != nil {
			dbg.Stage("marshal", t, func(s *debug.Stage) { s.Failed = true })
			dbg.Fail("marshal: " + err.Error())
			p.debug.Write(dbg)
			p.forwardRaw(w, r, rp, raw, "marshal error", err)
			return
		}
		dbg.Stage("marshal", t, func(s *debug.Stage) { s.BytesOut = len(nb) })
		out = nb
	}
	dbg.SnapshotOut(out)

	t = time.Now()
	r.Body = io.NopCloser(bytes.NewReader(out))
	r.ContentLength = int64(len(out))
	r.Header.Del("Content-Length")
	rp.ServeHTTP(w, r)
	dbg.Stage("forward", t, nil)
	// Lượt đứng ở chuỗi này đã có lời đáp → chuỗi khép lại, và vân tay của chính lời ấy được
	// giữ. Đúng chuỗi ấy quay lại sau là một hội thoại KHÁC mở bằng đúng câu cũ, không phải
	// lượt đi tiếp, và nó phải có phiên riêng; còn khi hai nhánh đứng chung một chuỗi thì vân
	// tay này là cái tách chúng ra ở lượt sau (§ Session). Đích lỗi hoặc client ngắt thì để
	// ngỏ: lần gửi lại y nguyên là một lần thử lại của chính cuộc này.
	if us.Status >= 200 && us.Status < 400 && r.Context().Err() == nil {
		sess.Replied(session.ReplyMark(us.Text()))
	}
	// Client ngắt giữa dòng thì ErrorHandler im — đúng, vì đó là chuyện thường của streaming
	// chứ không phải lỗi của đích. Nhưng im ở CẢ nhật ký thì bản ghi còn đúng một lượt dài
	// không có lời đáp và không có lý do; đọc lên y như proxy nuốt mất lời.
	dbg.Cut(r.Context().Err())

	// Một dòng cho một LẦN CHẠY: ai, ở đâu, chèn gì, và các con số usage. Số đọc được sau
	// khi forward xong, nên dòng này phải nằm ở đây — không phải trước.
	logRun(snap, sess, contribs, target.Host, start, us)
	p.debug.Write(dbg)
}

// logRun — dòng info duy nhất của một LẦN CHẠY, không phải một lượt: hàm này chạy mỗi
// request, và một lượt người sinh nhiều lần chạy (§ Lượt người và lần chạy).
//
// Thân nén thì không đọc được số, và lúc đó nó khai ra: một con số 0 giả tệ hơn không số.
func logRun(snap *request.Snapshot, sess *session.Session, contribs []plugin.Contribution, target string, start time.Time, u *usage.Stats) {
	args := []any{
		"agent", snap.Agent, "session", snap.SessionKey,
		"turns", sess.Turns(), "runs", sess.Runs(),
		"inserted", contribNames(contribs), "upstream", target,
		"took", time.Since(start).Milliseconds(),
	}
	switch {
	case u.Seen:
		args = append(args, "input", u.Input, "output", u.Output)
	case u.Encoding != "":
		args = append(args, "usage", "unreadable", "encoding", u.Encoding)
	}
	slog.Info("proxy: run", args...)
}

const houseTime = "02/01/2006 - 15:04:05"

// houseText — chỗ đứng và lúc mở phiên: đệ không tự biết thư mục nào, cũng không có
// đồng hồ. openedAt là mốc MỞ PHIÊN, không phải lúc này: khối nhà đi lại ở mọi request,
// nên đồng hồ chạy sẽ đổi tiền tố cache mỗi lượt. Quãng sống thì Wayfarer đo từng lượt.
func (p *Proxy) houseText(openedAt time.Time) string {
	if p.d.House == "" && p.d.Workspace == "" {
		return ""
	}
	var sb strings.Builder
	if p.d.Workspace != "" {
		sb.WriteString("Workspace: " + p.d.Workspace + "\n")
	}
	sb.WriteString("Session opened: " + openedAt.Format(houseTime) + "\n\n")
	sb.WriteString(p.d.House)
	return sb.String()
}

// appendBlock đặt một khối có tên vào cuối lời hệ thống. Không có chữ, hoặc khối đã
// có mặt trong lời hệ thống đang tới → không chạm.
func (p *Proxy) appendBlock(body *request.Body, systemText, tag, text string) bool {
	if text == "" || strings.Contains(systemText, "<"+tag+">") {
		return false
	}
	body.AppendSystem(request.Wrap(tag, text))
	return true
}

func contribNames(cs []plugin.Contribution) string {
	if len(cs) == 0 {
		return "silent"
	}
	names := make([]string, 0, len(cs))
	for _, c := range cs {
		names = append(names, c.Plugin)
	}
	return strings.Join(names, ",")
}
