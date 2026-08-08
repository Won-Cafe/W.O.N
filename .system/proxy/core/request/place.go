// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package request

import (
	"encoding/json"
	"hash/fnv"
)

// Placement — một khối của lõi và chỗ nó đứng: `After` là số message đứng trước nó trong
// mảng ĐANG tới. Người gọi dò con số này bằng vân tay mỗi lần chạy, không giữ sẵn, nên đầu
// mảng trượt cũng không lệch (§ Cache).
type Placement struct {
	After int
	Text  string
}

func (b *Body) MessageCount() int { return len(b.messages()) }

// MessageMark — vân tay message thứ i. Ngoài mảng → 0, và 0 không bao giờ là câu trả lời
// "khớp": một khối ghim mỏ neo 0 sẽ không dò ra chỗ nào, tức im lặng, đúng lối hỏng (#2).
func (b *Body) MessageMark(i int) uint64 {
	msgs := b.messages()
	if i < 0 || i >= len(msgs) {
		return 0
	}
	return markOf(msgs[i])
}

// MessageMarks — vân tay CẢ mảng, một lượt. Dò lại chỗ đứng của sổ ghim cần so với mọi
// message, và gọi MessageMark từng cái là tách lại mảng mỗi lần.
func (b *Body) MessageMarks() []uint64 {
	msgs := b.messages()
	out := make([]uint64, len(msgs))
	for i, m := range msgs {
		out[i] = markOf(m)
	}
	return out
}

// MessageTextAt — chữ hội thoại của message thứ i; rỗng khi ngoài mảng hoặc không có chữ.
// Sổ phiên hỏi qua cửa này: "lời đáp đứng ngay sau chuỗi dài n là của nhánh nào" (§ Session).
func (b *Body) MessageTextAt(i int) string {
	msgs := b.messages()
	if i < 0 || i >= len(msgs) {
		return ""
	}
	return b.flatten(b.contentOf(msgs[i]))
}

func markOf(msg json.RawMessage) uint64 {
	h := fnv.New64a()
	h.Write(msg)
	return h.Sum64()
}

// PlaceMessages dựng lại mảng hội thoại: message của công cụ chủ giữ nguyên thứ tự, khối
// của lõi xen vào đúng chỗ đã ghi. Trả về chữ ĐÃ đặt được, theo thứ tự trong mảng.
//
// Dựng lại chứ không nối vào cuối: khối của lượt trước phải về đúng chỗ cũ thì thân đi ra
// mới là phép nối của thân lần trước (§ Cache). places phải xếp theo After không giảm.
func (b *Body) PlaceMessages(places []Placement) []string {
	if len(places) == 0 {
		return nil
	}
	msgs, ok := b.messageList()
	if !ok {
		return nil
	}
	out := make([]json.RawMessage, 0, len(msgs)+len(places))
	placed := make([]string, 0, len(places))
	p := 0
	put := func(upto int) {
		for p < len(places) && places[p].After <= upto {
			if nm := b.coreMessage(RoleUser, places[p].Text); nm != nil {
				out = append(out, nm)
				placed = append(placed, places[p].Text)
			}
			p++
		}
	}
	for i := range msgs {
		put(i)
		out = append(out, msgs[i])
	}
	// Khối khai vị trí bằng hoặc quá cuối mảng rơi về cuối — chỗ đúng của khối lượt này,
	// và chỗ duy nhất còn lại cho khối trót ghi vị trí quá tay.
	put(len(msgs))
	if nb := mustJSON(out); nb != nil {
		b.fields[b.messagesKey()] = nb
	}
	return placed
}
