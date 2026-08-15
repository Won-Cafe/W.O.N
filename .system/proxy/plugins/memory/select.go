// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package memory

import (
	"fmt"
	"strconv"
	"strings"
)

// Hợp đồng của bộ chọn. `∅` cho im — cùng ký tự với ba agent bờ. Hai thứ cố ý
// không lấy từ hợp đồng giọng:
//   - câu "đây là dạng mặc định": bảo model im là chỗ nên về → số đúng tụt.
//   - khuôn `📄 <path>`: ô trống mời điền → model bịa 3/21 lượt. Đường dẫn phải CHÉP.
//
// Cũng cố ý không có câu "chịu mở hơn đi": thêm vào thì 89% → 68%.
//
// ĐIỀU KIỆN ĐO của hai số trên: `ornith:9b` với `think = false`. Ornith là reasoning
// model, nên đó là số của một model bị bịt khối nghĩ, không phải số của ornith.
//
// ĐO LẠI 14/08/2026 trên model nền hiện tại `qwen3.5:4b`, mười vòng thân mẫu, qua
// 11 cái kho không có. Câu "Never invent a path" ở trên KHÔNG giữ được ở khổ 4B, và số
// 3/21 của ornith là số nhẹ hơn thực tế nhiều lần, không phải số dè dặt.
//
// Cả 11 đường bịa đều mang tiền tố `working/`, trong khi kho lúc đo không có trang `working/`
// thật nào. Dòng tả `working/` dưới đây nói "việc họ đang mở" — thân mẫu thì tả đúng một
// việc đang mở, nên model dựng ra cái tên đáng lẽ phải có. Chỗ hở nằm ở đó, không ở câu
// cấm: một dòng tả vùng theo NGHĨA mời model suy ra trang, còn index thì chỉ liệt kê.
// `%s` là trần số trang, viết bằng CHỮ: hợp đồng đo được dùng chữ ("two"), và đổi
// hình một câu đã đo là bỏ phép đo. Trần đến từ `max_open_per_turn` — chép cứng số
// vào đây thì núm ấy chỉ hạ được, không nâng được, vì model vẫn nghe con số cũ.
const selectorHead = `You open pages from a personal memory store for the agent in the conversation.
%s
If nothing in the index answers to this turn, answer ` + "`∅`" + `.
Never invent a path.
` + "`personal/`" + ` and ` + "`procedural/`" + ` are lasting facts about the person — their habits, leanings, ways of working.
` + "`working/`" + ` is the task they have open right now.
` + "`moments/`" + ` is recent and unverified.
`

// Một hình lời đáp duy nhất, nên "never explain" không chọi với lệnh nào khác trong cùng
// một lời dặn.
const replyPathsOnly = "Answer with page paths from the index, copied exactly, at most %s, comma separated. Never explain."

func selectorSystem(lines string, limit int) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, selectorHead, fmt.Sprintf(replyPathsOnly, numWord(limit)))
	sb.WriteString("\n---\n\nMemory index:\n")
	sb.WriteString(lines)
	return sb.String()
}

// indexBlock ghép các dòng index. Dựng một lần mỗi phiên rồi chốt vào sổ phiên, và cả
// bộ chọn lẫn khối chèn đọc chung bản ấy — hai bản dựng riêng là hai bản lệch được.
func indexBlock(pages []page, stoneWeight int) string {
	var sb strings.Builder
	for _, p := range pages {
		sb.WriteString(indexLine(p, stoneWeight))
	}
	return sb.String()
}

// numWord viết số bằng chữ cho hợp đồng. Ngoài bảng thì trả chữ số — một hợp đồng nói
// "at most 7" vẫn đọc được, còn hợp đồng thiếu số thì không.
func numWord(n int) string {
	switch n {
	case 1:
		return "one"
	case 2:
		return "two"
	case 3:
		return "three"
	case 4:
		return "four"
	case 5:
		return "five"
	default:
		return strconv.Itoa(n)
	}
}

// indexLine — một dòng index, dùng cho cả bộ chọn và đệ đọc. KHÔNG mang ngày
// sửa: nó đổi mỗi lần trang bị sửa, mà index nằm trong tiền tố cache. Đo được là
// ngày không giúp độ đúng (89% cả hai cách), ngày viết vốn nằm trong tên trang.
// Sỏi chỉ hiện khi s+f > 0 — trang chưa có frontmatter thì không in.
func indexLine(p page, stoneWeight int) string {
	line := "- " + p.Path
	if p.Head != "" {
		line += " — " + p.Head
	}
	if p.Desc != "" {
		line += " · " + p.Desc
	}
	if p.S+p.F > 0 {
		line += fmt.Sprintf(" · sỏi %d (s%d/f%d)", stone(p.S, p.F, stoneWeight), p.S, p.F)
	}
	return line + "\n"
}

// parsePicks đọc lời model trả về thành danh sách đường dẫn. Chỉ nhận tên CÓ THẬT
// trong ứng viên — bịa thì bỏ, không đi tìm hộ. Hợp đồng nói "comma separated" nhưng
// model nền còn ngăn bằng dòng và khoảng trắng, nên tách hai nhịp: phẩy/dòng
// trước, mảnh nào không khớp thì cắt tiếp theo khoảng trắng. Vành đai là danh sách
// thật — nới cách cắt không nới cửa cho tên bịa.
func parsePicks(out string, cand []string, limit int) []string {
	t := strings.TrimSpace(out)
	if t == "" || strings.HasPrefix(strings.ToUpper(t), "NONE") || strings.HasPrefix(t, "∅") {
		return nil
	}
	valid := make(map[string]bool, len(cand))
	for _, p := range cand {
		valid[p] = true
	}
	var picked []string
	add := func(name string) bool {
		name = strings.Trim(strings.TrimSpace(name), "`*-\"'.,;:")
		if !valid[name] {
			return false
		}
		for _, p := range picked {
			if p == name {
				return true // đã có, coi như nhận rồi
			}
		}
		picked = append(picked, name)
		return true
	}
	for _, seg := range strings.FieldsFunc(t, func(r rune) bool { return r == ',' || r == '\n' }) {
		if len(picked) >= limit {
			break
		}
		if add(seg) {
			continue
		}
		for _, word := range strings.Fields(seg) {
			if len(picked) >= limit {
				break
			}
			add(word)
		}
	}
	return picked
}
