// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package request

import (
	"encoding/json"
	"strings"
)

// HumanSpokeLast — message cuối có phải lời người thật không. Không chỉ xem
// vai: Anthropic gửi tool_result dưới vai user. Điều kiện đúng: vai user, và
// nội dung có ít nhất một khối không phải tool_result.
func (b *Body) HumanSpokeLast() bool {
	msgs := b.messages()
	if len(msgs) == 0 {
		return false
	}
	last := msgs[len(msgs)-1]
	if b.roleOfMsg(last) != RoleUser {
		return false
	}
	// Gemini chở kết quả tool bằng part `functionResponse`, cũng dưới vai user — cùng cái bẫy
	// của `tool_result`, khác cái tên.
	if b.format == FormatGemini {
		return humanPartsGemini(geminiParts(last))
	}
	var m struct{ Content json.RawMessage }
	if err := json.Unmarshal(last, &m); err != nil {
		return false
	}
	return humanContent(m.Content, nil)
}

// HumanTexts — CHỮ của từng lượt người, nguyên văn, theo thứ tự xuất hiện. Nhật ký chia
// lượt theo lời người chứ không theo vai: Anthropic gửi tool_result dưới vai user, nên
// chia theo vai mở một lượt mới cho mỗi vòng máy. Đếm lượt thì có `Snapshot.HumanTurns`;
// hàm này trả chữ, không trả số.
//
// injected = chữ lõi vừa đặt, phải trừ ra: tiếng của lượt có thể đi vai user, và không
// trừ thì nhật ký mở một lượt ma cho chính chữ của mình.
//
// rules bắt buộc vì hàm này phải dùng ĐÚNG vị mà `Snapshot.HumanTurns` dùng (`b.humanTurn`):
// hai định nghĩa "lượt người" là hai sự thật khác nhau về cùng một hội thoại.
func (b *Body) HumanTexts(injected []string, rules FrameRules) []string {
	msgs := b.messages()
	if msgs == nil {
		return nil
	}
	ours := set(injected)
	var out []string
	for _, msg := range msgs {
		if b.normRole(b.roleOfMsg(msg)) != RoleUser {
			continue
		}
		// Ruột đọc qua cửa rẽ theo format: Gemini chở chữ ở `parts`, đọc thẳng `content` ra rỗng.
		content := b.contentOf(msg)
		if !b.humanContentOf(content, ours) {
			continue
		}
		if !b.humanTurn(content, rules) {
			continue // việc nhà của công cụ chủ: không phải một lượt, nên không mở lượt
		}
		out = append(out, b.flatten(content))
	}
	return out
}

// humanContentOf — lời người hay kết quả tool mặc vai user, rẽ theo format.
func (b *Body) humanContentOf(content json.RawMessage, ours map[string]bool) bool {
	if b.format == FormatGemini {
		return humanPartsGemini(content)
	}
	return humanContent(content, ours)
}

// contentBlock — phần lõi ĐỌC tới của một khối content dạng mảng (mặt viết là textBlock).
// Một nhà cho cả hai chỗ đọc: cùng một hình, và chép tay lần thứ hai là hai bản lệch được —
// thêm một field ở bên này mà quên bên kia thì cửa "đây có phải lời người không" trả lời
// khác nhau cho cùng một message.
type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// humanContent — nội dung là lời người hay kết quả tool mặc vai user. Chuỗi
// trần có chữ = lời người; dạng khối cần ít nhất một khối không phải tool_result.
//
// ours trừ ra khối lõi vừa đặt: đường Anthropic nối tiếng lượt vào message chở
// tool_result, không trừ thì message ấy hoá "lời người" và nhật ký mở lượt ma.
func humanContent(raw json.RawMessage, ours map[string]bool) bool {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s) != "" && !ours[s]
	}
	var blocks []contentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return false
	}
	for _, blk := range blocks {
		if blk.Type != "tool_result" && !ours[blk.Text] {
			return true
		}
	}
	return false
}

// humanTurn — message vai user này có phải MỘT LƯỢT của người không. Chặt hơn
// humanContent: nó còn trừ ra message chỉ chở việc nhà của công cụ chủ. VS Code gửi
// một message user thuần `<environment_info>`/`<workspace_info>` ở mọi lượt, nên đếm
// theo vai là lượt người đầu tiên đã mang số 2.
//
// Hai đường thành lượt người, và không đường nào phải đoán tên tag:
//   - có chữ NGOÀI mọi cặp tag — người viết trần, như Claude Code
//   - có chữ trong tag đã khai ở `unwrap_tags` — chỗ công cụ chủ đặt câu của người
//
// Còn lại là việc nhà: chữ nằm trọn trong những vỏ không ai khai là của người.
func humanTurn(raw json.RawMessage, rules FrameRules) bool {
	if !humanContent(raw, nil) {
		return false
	}
	// Chưa khai vỏ nào chở lời người thì lõi KHÔNG siết: công cụ chủ nào cũng có thể
	// gói câu của người vào một vỏ, và siết mà không biết vỏ ấy tên gì thì mọi lượt
	// hoá việc nhà — đếm 0, tệ hơn đếm thừa. Siết chỉ khi có lời khai (#6).
	if len(rules.Unwrap) == 0 {
		return true
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		var blocks []contentBlock
		if err := json.Unmarshal(raw, &blocks); err != nil {
			return true // hình không đọc được thì không siết thêm (#6)
		}
		var sb strings.Builder
		for _, blk := range blocks {
			if blk.Type == "tool_result" {
				continue
			}
			// Khối không phải chữ (ảnh, tài liệu) là nội dung thật, không phải vỏ —
			// một lượt chỉ có ảnh vẫn là một lượt của người.
			if blk.Type != "text" {
				return true
			}
			sb.WriteString(blk.Text)
			sb.WriteByte('\n')
		}
		s = sb.String()
	}
	for _, tag := range rules.Unwrap {
		if inner, ok := extractBlock(s, tag); ok && strings.TrimSpace(inner) != "" {
			return true
		}
	}
	return strings.TrimSpace(outsideTags(s)) != ""
}

// outsideTags bỏ mọi cặp `<tag>…</tag>` cân khớp, trả phần chữ còn lại. Tự quét thay
// vì regexp: RE2 không có back-reference nên không khớp được tag mở với tag đóng.
// Tag lẻ (chỉ mở, không đóng) thì giữ nguyên — không đủ cặp thì không phải một vỏ.
func outsideTags(s string) string {
	var sb strings.Builder
	for i := 0; i < len(s); {
		name, after, ok := openTag(s, i)
		if !ok {
			sb.WriteByte(s[i])
			i++
			continue
		}
		end := strings.Index(s[after:], "</"+name+">")
		if end < 0 {
			sb.WriteByte(s[i])
			i++
			continue
		}
		i = after + end + len("</"+name+">")
	}
	return sb.String()
}

// openTag đọc `<name>` tại vị trí i. Chỉ nhận tên tag thuần — chữ, số, `-`, `_` — nên
// một dấu `<` trong câu người viết không bị hiểu thành vỏ.
func openTag(s string, i int) (name string, after int, ok bool) {
	if s[i] != '<' {
		return "", 0, false
	}
	j := i + 1
	for j < len(s) && (isTagByte(s[j])) {
		j++
	}
	if j == i+1 || j >= len(s) || s[j] != '>' {
		return "", 0, false
	}
	return s[i+1 : j], j + 1, true
}

func isTagByte(c byte) bool {
	return c == '-' || c == '_' ||
		(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

func set(in []string) map[string]bool {
	out := make(map[string]bool, len(in))
	for _, s := range in {
		out[s] = true
	}
	return out
}
