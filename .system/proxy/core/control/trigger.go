// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package control

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"won/proxy/core/plugin"
	"won/proxy/core/request"
	"won/proxy/core/session"
)

// triggerReq — thân gửi lên. `body` là thân request của công cụ chủ, đúng HÌNH mà dòng
// chính nhận, nên `run/*/debug_input.json` nạp thẳng vào đây được — nhưng để kiểm KHUNG,
// không phải để diễn lại lượt cũ: bản ghi ấy cắt mọi chuỗi dài về hai mép (§ Quan sát
// được, `debug/cut.go`), cố ý, vì một thân đủ chữ đọc lại là ăn trọn cửa sổ ngữ cảnh.
type triggerReq struct {
	Format string          `json:"format"` // anthropic | openai | gemini; rỗng = anthropic
	Agent  string          `json:"agent"`  // rỗng = để lõi tự nhận như dòng chính
	Body   json.RawMessage `json:"body"`
}

// triggerView — cái một lượt chạy khô nói lại. `gates` là phán của ba cửa dòng chính:
// plugin chạy được ở đây mà `gates.pass` là false thì trong thực tế nó không hề được gọi,
// và đó thường là câu trả lời cho "vì sao plugin này im".
type triggerView struct {
	Plugin string              `json:"plugin"`
	Agent  string              `json:"agent"`
	Line   string              `json:"line"`
	Detail plugin.PluginDetail `json:"detail"`
	Gates  gateView            `json:"gates"`
	Note   string              `json:"note"`
}

type gateView struct {
	Pass             bool `json:"pass"`
	AgentTurnShaped  bool `json:"agent_turn_shaped"`
	Conversationable bool `json:"conversation_shaped"`
	MachineReply     bool `json:"machine_reply"`
}

const noteDryRun = "lượt chạy khô: phiên tạm, không ghi state.json, không chạm nhịp phiên thật, không chuyển tiếp lên nhà cung cấp"

// handleTrigger chạy MỘT plugin trên một thân request cho sẵn rồi trả lời nguyên văn.
//
// Ba điều nó KHÔNG làm, và cả ba là chủ đích:
//   - không dùng sổ phiên thật (NewEphemeral) — `Session.Asked` chỉ hỏi mỗi câu một lần
//     trong một phiên, nên chạy thử trên phiên thật là đầu độc lượt thật sau đó
//   - không đi qua ba cửa chặn lượt việc nhà — người gọi muốn xem plugin nói gì, nên chặn
//     ở đây là trả về không gì; thay vào đó phán của ba cửa nằm trong lời đáp
//   - không chuyển tiếp gì lên upstream
func (a *API) handleTrigger(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	target := a.pluginByName(name)
	if target == nil {
		a.writeError(w, http.StatusNotFound,
			"plugin "+name+" chưa dựng trong tiến trình này — khai `"+name+".enable = true` rồi chạy lại")
		return
	}

	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxTriggerBytes))
	if err != nil {
		a.writeError(w, http.StatusBadRequest, "cannot read body: "+err.Error())
		return
	}
	var req triggerReq
	if err := json.Unmarshal(raw, &req); err != nil || len(req.Body) == 0 {
		a.writeError(w, http.StatusBadRequest, `body must be JSON {"body": <request body>, "format": "anthropic"}`)
		return
	}
	format, err := triggerFormat(req.Format)
	if err != nil {
		a.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	body, err := request.ParseBody(req.Body, format)
	if err != nil {
		a.writeError(w, http.StatusBadRequest, "body not parseable as "+req.Format+": "+err.Error())
		return
	}

	// Nhận đệ đúng chuỗi của dòng chính: lời khai tường minh → chữ trong lời hệ thống →
	// đệ mặc định. Đệ quyết soul nào plugin mang, nên chọn khác dòng chính là đo sai người.
	agent := a.d.Identity.Resolve(req.Agent, body.SystemText())
	if agent == "" {
		agent = a.d.DefaultAgent
	}

	snap := body.Snapshot(a.d.FrameRules)
	snap.Agent = agent
	// Thân không có lượt hội thoại nào thì không phải một lượt của công cụ chủ, và plugin
	// nào chạy trên nó cũng im vì đúng thiết kế — trả 200 ở đó là nói "đã đo" cho một phép
	// đo không có gì để đo. Vỡ rõ, kèm chỗ sai.
	if len(snap.Turns) == 0 && snap.Anchor == "" {
		a.writeError(w, http.StatusBadRequest,
			"thân không có lượt hội thoại nào — `body` phải là một thân request thật, ví dụ chép từ run/*/debug_input.json")
		return
	}
	key, fallback := session.Key("", snap.Agent, snap.FirstUser, snap.FirstAssistant)
	snap.SessionKey = key

	// Sổ phiên tạm, sinh ra và chết theo đúng lượt này.
	reach := session.Reach{Marks: body.MessageMarks(), Text: body.MessageTextAt}
	sess := session.NewEphemeral().Touch(key, fallback, snap.Agent, reach, snap.HumanTurns, time.Now())

	ctx, cancel := contextWithBudget(r.Context(), a.d.TotalBudget)
	defer cancel()
	ctx = plugin.WithTracing(ctx) // chạy khô thì luôn kể prompt vào và lời đáp ra
	contribs, details := plugin.Gather(ctx, []plugin.Plugin{target}, snap, sess)

	view := triggerView{Plugin: name, Agent: agent, Note: noteDryRun, Gates: gateView{
		AgentTurnShaped:  body.AgentTurnShaped(a.d.FrameRules),
		Conversationable: body.ConversationShaped(),
		MachineReply:     body.MachineReply(),
	}}
	view.Gates.Pass = view.Gates.AgentTurnShaped && view.Gates.Conversationable && !view.Gates.MachineReply
	if len(details) > 0 {
		view.Detail = details[0]
	}
	if len(contribs) > 0 {
		view.Line = contribs[0].Text
	}
	a.writeJSON(w, http.StatusOK, view)
}

// maxTriggerBytes — thân request thật của một công cụ agent lên tới vài trăm KB, nên trần
// rộng; nhưng có trần, vì đây là cửa nhận thân tuỳ ý.
const maxTriggerBytes = 8 << 20

func (a *API) pluginByName(name string) plugin.Plugin {
	for _, p := range a.d.Plugins {
		if p.Name() == name {
			return p
		}
	}
	return nil
}

// triggerFormat — hình phải KHAI, không sniff thân (#6). Rỗng là Anthropic vì đó là hình
// của thân trong `debug_input.json` phổ biến nhất.
func triggerFormat(name string) (request.Format, error) {
	switch name {
	case "", "anthropic":
		return request.FormatAnthropic, nil
	case "openai":
		return request.FormatOpenAI, nil
	case "openai-responses":
		return request.FormatOpenAIResponses, nil
	case "gemini":
		return request.FormatGemini, nil
	default:
		return request.FormatUnknown, fmt.Errorf("unknown format %q — only anthropic, openai, openai-responses, gemini", name)
	}
}

// contextWithBudget — trần cả lượt như dòng chính. Không khai trần thì không đặt hạn:
// một lượt chạy khô không có ai chờ ở đầu dây bên kia.
func contextWithBudget(parent context.Context, budget time.Duration) (context.Context, context.CancelFunc) {
	if budget <= 0 {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, budget)
}
