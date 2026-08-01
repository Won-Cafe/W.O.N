// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package localmodel

import (
	"strings"

	"won/proxy/core/request"
)

// Giới hạn khi vẽ bản sao cho model nhỏ đọc — đủ để phán, không tràn ngữ cảnh.
const (
	turnExcerpt    = 700
	renderMaxTurns = 6
)

// Đọc lời đáp về thì ở plugins/base (`Say`) — chỗ này chỉ dựng lời hỏi đi. Hợp đồng
// đầu ra và cách bóc nó sống cùng một nhà, để hai đầu không lệch nhau.

// RenderUser dựng phần lời hỏi. Ba agent bờ cùng một hình: đệ đang được chạm,
// vật liệu riêng, rồi bản sao hội thoại — không câu dẫn truyện nào.
// `who` là chữ nghề gọi người nó chạm — Outfitter nhìn "người mang" nên nhãn Wearer, và
// nhãn đúng nghề nâng hẳn tỉ lệ model nói được.
func RenderUser(snap *request.Snapshot, who, agent string, material ...string) string {
	var sb strings.Builder
	sb.WriteString("<" + who + ">" + request.AgentOrUnknown(agent) + "</" + who + ">\n\n")
	for _, m := range material {
		sb.WriteString(m)
	}
	sb.WriteString(RenderSnapshot(snap))
	return sb.String()
}

// Block bọc mảnh vật liệu nhiều dòng trong khối có tên. Rỗng → không có khối.
func Block(tag, text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	return request.Wrap(tag, text) + "\n\n"
}

// RenderSnapshot vẽ bản sao request cho agent nền. Mỗi lượt bọc trong khối có tên: không
// bọc thì lời đệ chảy liền vào lời dặn, và model nhỏ bắt chước bài mẫu dài nhất nó
// thấy — một dòng "đừng bắt chước" không cân được ngàn ký tự ví dụ.
//
// KHÔNG chở lời hệ thống của công cụ chủ: nó mở đầu bằng một lời khẳng định căn cước, và
// một căn cước thứ hai trong prompt là một căn cước model nền có thể nhận (§ Vì sao model
// nhỏ). Không tiếng bờ nào cần nó — vật liệu của nghề đi qua `material`.
func RenderSnapshot(snap *request.Snapshot) string {
	var sb strings.Builder
	sb.WriteString("<Conversation>\n")
	sb.WriteString("Words of other people. Read them; never copy their shape or their markers.\n")
	turns := snap.Turns
	if len(turns) > renderMaxTurns {
		turns = turns[len(turns)-renderMaxTurns:]
	}
	// Mỏ neo luôn đứng đầu cửa sổ. Cửa sổ cắt theo số lượt, nên giữa vòng tool dài,
	// lời người mở lượt rơi ra ngoài. Nó đứng đầu là đúng thứ tự thời gian.
	if a := snap.Anchor; a != "" && !hasText(turns, a) {
		turns = append([]request.Turn{{Role: request.RoleUser, Text: a}}, turns...)
	}
	for _, t := range turns {
		role := t.Role
		if role == "" {
			role = "unknown"
		}
		sb.WriteString("<" + role + ">\n")
		sb.WriteString(request.Truncate(flattenShape(t.Text), turnExcerpt))
		sb.WriteString("\n</" + role + ">\n")
	}
	sb.WriteString("</Conversation>\n")
	return sb.String()
}

// shapeReplacer — dấu nhấn của Markdown, bóc khỏi bản sao. Dựng MỘT LẦN: Replacer dựng
// sẵn bảng khớp lúc khởi tạo và an toàn cho gọi song song, nên dựng lại nó bên trong vòng
// lặp là trả giá dựng bảng ấy cho từng dòng của từng lượt, ở mọi lần hỏi model nền.
var shapeReplacer = strings.NewReplacer("**", "", "__", "", "`", "")

// flattenShape bỏ KHUÔN khỏi bản sao, giữ chữ. Định dạng và marker của đệ dòng chính là
// bài mẫu dài nhất trong prompt, và model nhỏ chép bài mẫu chứ không đọc hợp đồng
// (§ Vì sao model nhỏ).
func flattenShape(s string) string {
	out := make([]string, 0, 16)
	for _, ln := range strings.Split(s, "\n") {
		ln = strings.TrimSpace(ln)
		if isRule(ln) {
			continue // `---`, `***`: đường kẻ, không phải chữ
		}
		ln = shapeReplacer.Replace(ln)
		ln = request.TrimMarkers(ln) // tiêu đề, gạch đầu dòng, và marker của đệ khác
		if ln == "" {
			// Một dòng trống là nhịp đọc, giữ — nhưng không giữ hai dòng liền nhau.
			if len(out) > 0 && out[len(out)-1] != "" {
				out = append(out, "")
			}
			continue
		}
		out = append(out, ln)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// isRule — dòng chỉ gồm dấu kẻ ngang.
func isRule(ln string) bool {
	return len(ln) >= 3 && strings.Trim(ln, "-*_ ") == ""
}

// hasText — mỏ neo đã nằm sẵn trong cửa sổ chưa. Kể hai lần thì model đọc thành
// người hỏi hai lần.
func hasText(turns []request.Turn, text string) bool {
	for _, t := range turns {
		if t.Text == text {
			return true
		}
	}
	return false
}
