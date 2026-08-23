// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package request

import (
	"encoding/json"
	"strings"
	"time"
)

// Giới hạn bản sao — plugin cần đủ để phán, không cần cả hội thoại.
const (
	targetLen       = 120
	anchorLen       = 200
	reachedMax      = 16 // lượt gọi tool gần nhất — đủ thấy nếp tay, không thành biên niên
	snapMaxTurns    = 12
	snapMaxTurnText = 2000
)

type Turn struct {
	Role string
	Text string
}

// TargetKind — đích này là LOẠI gì. Cần phân biệt vì chỉ một loại là một CHỖ trong cây:
// đưa một câu lệnh cho hàm đọc vùng thì nó cắt tiền tố của câu lệnh và trả về đó như một
// tên vùng (`cd C:/`), và bên đọc nhận một cái nhãn nói sai.
type TargetKind uint8

const (
	TargetNone TargetKind = iota
	TargetPath
	TargetCommand
	TargetURL
	TargetPattern
)

// kindLabels — tên gọi từng loại đích. Chỗ DUY NHẤT đặt tên chúng, nên bên đọc lấy ở
// đây chứ không chép tay: thêm một loại thì bên đọc tự đúng.
var kindLabels = map[TargetKind]string{
	TargetCommand: "câu lệnh",
	TargetURL:     "địa chỉ ngoài",
	TargetPattern: "khuôn tìm",
}

// IsPlace — đích loại này có phải một chỗ trong cây không, tức có đem đi đọc vùng được
// không. Một vị từ riêng thay vì để bên đọc suy từ `Label()` rỗng: một chuỗi rỗng làm
// tín hiệu thì bên nào quên xét nó sẽ in ra `[]` mà không ai báo.
func (k TargetKind) IsPlace() bool { return k == TargetPath }

// Label — tên loại đích. Rỗng cho loại là một CHỖ: chỗ gọi bằng tên VÙNG của nó, và tên
// vùng không nằm ở đây.
func (k TargetKind) Label() string { return kindLabels[k] }

// LabeledKinds — các loại CÓ tên gọi, thứ tự ổn định. Không phải mọi loại: loại là một
// chỗ thì không có tên loại. Để bên đọc khai đủ nhãn mà không chép tay.
func LabeledKinds() []TargetKind { return []TargetKind{TargetCommand, TargetURL, TargetPattern} }

// ToolCall — một lượt với tay: món gì, chạm vào đâu, và cái đích ấy là loại gì. Target
// rỗng khi tool không nhận đích nào có tên. Chỉ đọc: lõi không có đường ghi vào `tools`
// lẫn vào lượt gọi.
type ToolCall struct {
	Name   string
	Target string
	Kind   TargetKind
}

// Snapshot là bản sao trung lập, chỉ-đọc của request.
type Snapshot struct {
	Agent      string
	SessionKey string
	System     string
	// FirstUser — LƯỢT của người đầu tiên, mỏ neo dẫn xuất khoá phiên. Không phải message
	// user đầu tiên: message ấy thường là việc nhà của công cụ chủ và giống hệt nhau ở mọi
	// phiên. Chưa có lượt người nào → rỗng, và rỗng cũng là một mỏ neo hợp lệ: lượt chưa có
	// ai nói thì chưa có hội thoại nào để tách.
	FirstUser string
	// FirstAssistant — lượt trả lời đầu; mỏ neo thăng cấp khoá phiên.
	FirstAssistant string
	Turns          []Turn
	// Tools — tên kèm hướng dẫn từng tool. Agent bờ đọc để nói về đồ nghề. Chỉ đọc.
	Tools []ToolInfo
	// Anchor — lời người mở lượt đang chạy. Giữ RIÊNG, không để chung Turns: cửa sổ
	// cắt theo số lượt, mà giữa vòng tool dài lời người rơi ra ngoài. Phải luôn có mặt.
	Anchor string
	// HumanSpokeLast — người thật vừa nói, không phải máy trả kết quả tool. Vai của message
	// cuối KHÔNG suy ra được điều này: có định dạng chở kết quả tool dưới chính vai `user`.
	HumanSpokeLast bool
	// HumanTurns — số lần NGƯỜI nói trong CHÍNH request này. Đếm từ request, không
	// tích luỹ qua các lần chạy: request mang trọn lịch sử hội thoại, nên con số này
	// tự đúng và tự sửa. Rewind gửi lại ít lượt hơn → số giảm theo, đúng như hội thoại
	// vừa bị cắt ngắn; một bộ đếm chỉ biết cộng thì không có đường nào về.
	HumanTurns int
	// Reached — các lượt với tay, cũ → mới, kèm đích của từng lượt. Đệ chạy nhiều lượt
	// máy trước khi người trả lời; đây là chỗ thấy nó vừa làm gì, và làm vào đâu.
	Reached    []ToolCall
	ReceivedAt time.Time
}

func (b *Body) Snapshot(rules FrameRules) *Snapshot {
	snap := &Snapshot{
		System:         b.SystemText(),
		Tools:          b.ToolInfos(),
		HumanSpokeLast: b.HumanSpokeLast(),
		ReceivedAt:     time.Now(),
	}
	for _, raw := range b.messages() {
		// Vai quy về tên chung của lõi: Gemini gọi lượt trả lời là `model`, và nếu không quy
		// về thì Reached rỗng, FirstAssistant rỗng, và khoá phiên không bao giờ thăng cấp.
		role := b.normRole(b.roleOfMsg(raw))
		content := b.contentOf(raw)
		// role:tool / role:function = kết quả công cụ, không phải lượt hội thoại.
		if role == "tool" || role == "function" {
			continue
		}
		if role == RoleAssistant {
			snap.Reached = append(snap.Reached, b.toolsCalled(raw)...)
		}
		human := role == RoleUser && b.humanTurn(content, rules)
		// Đếm trước khi lọc theo chữ: một lượt chỉ có ảnh vẫn là một lượt của người.
		if human {
			snap.HumanTurns++
		}
		text := b.flatten(content)
		if text == "" {
			continue
		}
		if role == RoleUser {
			// Mỏ neo là LƯỢT của người, không phải message user đầu tiên. Công cụ chủ mở mọi
			// phiên bằng một message việc nhà giống hệt nhau (`<environment_info>`), nên neo
			// vào message đầu là neo vào một HẰNG SỐ: mọi hội thoại của cùng một đệ ra cùng
			// một khoá, và cái duy nhất tách chúng — khoá thăng cấp theo lời đáp đầu — không
			// bao giờ nổ với đệ ra tay trước, vì lượt gọi tool không có chữ (§ Session).
			if human && snap.FirstUser == "" {
				snap.FirstUser = Truncate(text, anchorLen)
			}
			snap.Anchor = Truncate(text, snapMaxTurnText) // lượt sau đè lượt trước
		}
		if role == RoleAssistant && snap.FirstAssistant == "" {
			snap.FirstAssistant = Truncate(text, anchorLen)
		}
		snap.Turns = append(snap.Turns, Turn{Role: role, Text: Truncate(text, snapMaxTurnText)})
	}
	if len(snap.Turns) > snapMaxTurns {
		snap.Turns = snap.Turns[len(snap.Turns)-snapMaxTurns:]
	}
	if len(snap.Reached) > reachedMax {
		snap.Reached = snap.Reached[len(snap.Reached)-reachedMax:]
	}
	return snap
}

// toolsCalled — tool một lượt assistant vừa với tới. Đọc ở lượt GỌI, không ở lượt
// trả kết quả: cái cần thấy là bàn tay đã với gì, và với vào đâu.
//
// Hai hình dạng parse RỜI, không gộp một struct: đường OpenAI để `content` là chuỗi,
// đường Anthropic để nó là mảng khối. Gộp lại thì một chuỗi làm vỡ cả lần đọc và
// `tool_calls` mất theo.
func toolsCalled(msg json.RawMessage) []ToolCall {
	var out []ToolCall

	// OpenAI: tool_calls[].function — arguments là JSON đã đóng thành chuỗi.
	var oai struct {
		ToolCalls []struct {
			Function struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"function"`
		} `json:"tool_calls"`
	}
	if json.Unmarshal(msg, &oai) == nil {
		for _, tc := range oai.ToolCalls {
			if tc.Function.Name != "" {
				target, kind := targetOf([]byte(tc.Function.Arguments))
				out = append(out, ToolCall{Name: tc.Function.Name, Target: target, Kind: kind})
			}
		}
	}

	// Anthropic: khối tool_use trong content, input là object.
	var anth struct {
		Content []struct {
			Type  string          `json:"type"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
	}
	if json.Unmarshal(msg, &anth) == nil {
		for _, blk := range anth.Content {
			if blk.Type == "tool_use" && blk.Name != "" {
				target, kind := targetOf(blk.Input)
				out = append(out, ToolCall{Name: blk.Name, Target: target, Kind: kind})
			}
		}
	}
	return out
}

// targetKeys — những khoá tham số nói tool này chạm vào ĐÂU. Danh sách đóng, không
// đoán: quét bừa mọi tham số thì `content` của một lần ghi file (cả nội dung mới)
// cũng lọt vào ngữ cảnh — vừa vô nghĩa vừa là chỗ bí mật rò ra (#5). Thứ tự là thứ
// tự ưu tiên: khoá đứng trước tả đích chính xác hơn khoá đứng sau.
var targetKeys = []struct {
	key  string
	kind TargetKind
}{
	{"path", TargetPath},
	{"url", TargetURL},
	{"glob", TargetPattern},
	{"command", TargetCommand},
	{"file_path", TargetPath},
	{"filePath", TargetPath},
	{"notebook_path", TargetPath},
	{"pattern", TargetPattern},
}

// targetOf rút đích của một lượt gọi tool từ tham số. Không khoá nào khớp → rỗng,
// và bên nhận chỉ thấy tên món. Cắt ngắn: đây là cái nhãn, không phải bản ghi.
func targetOf(args []byte) (string, TargetKind) {
	if len(args) == 0 {
		return "", TargetNone
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(args, &m) != nil {
		return "", TargetNone
	}
	for _, k := range targetKeys {
		raw, ok := m[k.key]
		if !ok {
			continue
		}
		var s string
		if json.Unmarshal(raw, &s) == nil && strings.TrimSpace(s) != "" {
			return Truncate(strings.TrimSpace(s), targetLen), k.kind
		}
	}
	return "", TargetNone
}

// flattenContent làm phẳng content thành text hội thoại: chỉ khối text, nối \n.
func flattenContent(c json.RawMessage) string {
	var s string
	if err := json.Unmarshal(c, &s); err == nil {
		return s
	}
	var blocks []json.RawMessage
	if err := json.Unmarshal(c, &blocks); err != nil {
		return ""
	}
	var sb strings.Builder
	for _, blk := range blocks {
		var m struct {
			Type string
			Text string
		}
		if err := json.Unmarshal(blk, &m); err != nil {
			continue
		}
		if m.Type == "text" && m.Text != "" {
			sb.WriteString(m.Text)
			sb.WriteString("\n")
		}
	}
	return strings.TrimSpace(sb.String())
}
