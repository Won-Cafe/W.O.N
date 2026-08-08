// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package request

import (
	"encoding/json"
	"hash/fnv"
	"strings"
)

// CacheMark — một mắt của chuỗi đi lên upstream: vân tay và cỡ của MỘT khối. Cache khớp
// theo tiền tố nên đơn vị đo là chuỗi khối, không phải n byte đầu thân (§ Cache).
type CacheMark struct {
	Slot  string // vai của khối: `system` hoặc vai của message
	Label string // tên khối nếu chữ tự bọc `<Tên>`, không thì rỗng
	Hash  uint64
	Bytes int
}

// CacheMarks — trọn chuỗi upstream đọc, theo đúng thứ tự nó đọc. Gọi TRƯỚC Marshal, trên
// chính body sắp đi ra. Vân tay lấy trên byte thật của phần tử, không chỉ trên chữ hội
// thoại: cache khớp trên chuỗi đã tuần tự hoá, nên đổi ngoài `content` cũng phá tiền tố.
func (b *Body) CacheMarks() []CacheMark {
	var out []CacheMark
	// OpenAI chở lời hệ thống trong chính mảng messages, nên chuỗi wire của nó chỉ có một
	// phần. Hai nhà kia có field system riêng, đứng TRƯỚC hội thoại.
	if b.format != FormatOpenAI {
		for _, s := range b.SystemBlocks() {
			out = append(out, newMark(RoleSystem, []byte(s), s))
		}
	}
	for _, m := range b.messages() {
		out = append(out, newMark(b.normRole(b.roleOfMsg(m)), m, b.flatten(b.contentOf(m))))
	}
	return out
}

// newMark — vân tay trên raw, nhãn đọc từ chữ. Hai nguồn khác nhau là chủ ý: hash phải
// theo byte thật, còn nhãn để người soi nhật ký gọi được tên khối vừa gãy.
func newMark(slot string, raw json.RawMessage, text string) CacheMark {
	h := fnv.New64a()
	h.Write(raw)
	return CacheMark{Slot: slot, Label: blockTag(text), Hash: h.Sum64(), Bytes: len(raw)}
}

// blockTag — tên khối nếu chữ mở bằng `<Tên>`. Chỉ nhận tên trần: có khoảng trắng, gạch
// chéo hay xuống dòng trong đó thì đấy là chữ của người khác, không phải đường may của lõi.
func blockTag(text string) string {
	if !strings.HasPrefix(text, "<") {
		return ""
	}
	end := strings.IndexByte(text, '>')
	if end <= 1 {
		return ""
	}
	name := text[1:end]
	if strings.ContainsAny(name, " \t\n/") {
		return ""
	}
	return name
}
