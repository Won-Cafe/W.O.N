// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package outfitter

import (
	"strings"
	"unicode"

	"won/proxy/core/request"
)

// Cửa cơ học sau khi model đã nói. Vai của kẻ giữ kho khai ở README: "nói một dòng về
// một món đang nằm sai chỗ… chỉ vào món và vào chỗ món ĐÃ ĐI, không chỉ vào người".
// Ba mệnh đề ấy quyết định được bằng chuỗi, nên chúng sống ở đây chứ không nằm nhờ vào
// việc model 4B có chịu tuân lời nhắc hay không: một luật chỉ khai trong prompt là một
// luật model được quyền bỏ. Trượt cửa nào cũng về im lặng — fail-open toàn tuyến.

// arrows — dấu nối giữa món và đích. Model nhỏ dùng cả hai hình.
var arrows = []string{"→", "->"}

// minTargetRunes — đích ngắn hơn thì không đem đi so: một hai chữ khớp bừa vào bất cứ
// đường dẫn nào, và một cửa gác bằng cách khớp bừa thì không phải cửa.
const minTargetRunes = 3

// keep — dòng này có đúng vai kẻ giữ kho không. Trả về TÊN CỬA đã chặn, rỗng = qua cửa.
// Tên cửa, không kèm chữ của dòng: lời model là nội dung hội thoại, và nội dung chỉ sống
// ở nhật ký chẩn bệnh — nơi có công tắc riêng và người đọc biết mình đang mở gì (#5). Mọi
// slog khác trong cây cũng chỉ chở số đo và tên, không chở chữ.
func keep(line string, kit []request.ToolInfo, reached []request.ToolCall, names, said []string) string {
	if !namesTool(line, kit) {
		return "không gọi tên món nào trong Kit"
	}
	if d := destOf(line); d != "" && !wentThere(d, reached) {
		return "đích không phải chỗ món đã đi"
	}
	if namesPerson(line, names) {
		return "chỉ vào người"
	}
	for _, s := range said {
		if norm(s) == norm(line) {
			return "đã nói câu này trong phiên"
		}
	}
	return ""
}

// namesTool — dòng có gọi tên một món CÓ THẬT trong tay người mang không. Đây là cửa
// chặn món bịa: model nhỏ đọc tên tool trong hội thoại rồi dựng thêm tên na ná.
func namesTool(line string, kit []request.ToolInfo) bool {
	low := strings.ToLower(line)
	for _, t := range kit {
		if t.Name != "" && strings.Contains(low, strings.ToLower(t.Name)) {
			return true
		}
	}
	return false
}

// destOf — phần đích của dòng: chữ sau dấu nối ĐẦU, đã bóc nhãn `[...]` mà model chép
// theo bản kê. Không có dấu nối = dòng không trỏ đích nào, và đó là dòng hợp lệ: dấu
// *thừa* nói về một món nằm im, mà món nằm im thì chưa đi đâu cả.
//
// Dấu nối ĐẦU, không phải cuối: hình một dòng là `<món> → <đích>`, nên chữ sau mũi tên
// thứ nhất là đích. Đọc từ mũi tên cuối thì một dòng hai mũi tên — `món → chỗ → [nhãn]`,
// đã lọt ra thật — cho đích rỗng, và đích rỗng thì cửa này mở ra cho mọi thứ đi qua.
func destOf(line string) string {
	i, n := -1, 0
	for _, a := range arrows {
		if j := strings.Index(line, a); j >= 0 && (i < 0 || j < i) {
			i, n = j, len(a)
		}
	}
	if i < 0 {
		return ""
	}
	d := strings.TrimSpace(line[i+n:])
	if j := strings.LastIndex(d, "["); j >= 0 {
		d = strings.TrimSpace(d[:j])
	}
	for _, a := range arrows {
		d = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(d), a))
	}
	return strings.TrimSpace(strings.Trim(d, `.,"'“”`))
}

// wentThere — cái đích ấy có nằm trong dấu tay vừa để lại không. So hai chiều vì bản kê
// cắt đích ở 120 ký tự và model hay nhắc lại đích ở dạng ngắn: `What/World/World.md`
// và `C:\...\What\World\World.md` là một chỗ.
func wentThere(dest string, reached []request.ToolCall) bool {
	d := norm(dest)
	if len([]rune(d)) < minTargetRunes {
		return false
	}
	for _, c := range reached {
		t := norm(c.Target)
		if len([]rune(t)) < minTargetRunes {
			continue
		}
		if strings.Contains(t, d) || strings.Contains(d, t) {
			return true
		}
	}
	return false
}

// namesPerson — dòng có chỉ vào một người trong hệ không. Quét sau khi đã bỏ mọi đoạn
// trông như đường dẫn: `.system/agents/Tzu.agent.md` là một CHỖ hợp lệ để trỏ tới, còn
// một câu gọi tên đệ rồi kể việc đệ ấy làm thì là chỉ vào người.
//
// Trả bool, không trả cái tên: cái tên bắt được cũng là một mảnh của dòng, và dòng không
// đi vào log.
//
// Roster chỉ dùng để BỎ một cái tên, không bao giờ rót vào prompt: rót vào thì model
// gọi ra tên đệ không có trong đối thoại (§ outfitter.go, lý do không rót House).
func namesPerson(line string, names []string) bool {
	var sb strings.Builder
	for _, f := range strings.Fields(line) {
		if strings.ContainsAny(f, `/\`) {
			continue
		}
		sb.WriteString(f)
		sb.WriteByte(' ')
	}
	bare := strings.ToLower(sb.String())
	for _, n := range names {
		if n == "" {
			continue
		}
		if word(bare, strings.ToLower(n)) {
			return true
		}
	}
	return false
}

// word — `sub` có đứng như một chữ trọn trong `s` không. Cần biên chữ vì tên đệ ngắn:
// không có biên thì "Mo" khớp vào giữa "memory".
func word(s, sub string) bool {
	for i := 0; ; {
		j := strings.Index(s[i:], sub)
		if j < 0 {
			return false
		}
		j += i
		if edge(s, j-1) && edge(s, j+len(sub)) {
			return true
		}
		i = j + len(sub)
	}
}

// edge — vị trí này là biên chữ: ngoài chuỗi, hoặc một ký tự không phải chữ/số.
//
// Đọc theo BYTE, không theo rune, và lệch ấy là chủ đích an toàn: một byte giữa ký tự
// tiếng Việt nhiều byte đều ≥ 0x80, tức vẫn là "chữ", nên chỗ ấy KHÔNG tính là biên và
// tên đệ không khớp. Lệch về phía bỏ sót một cái tên, không về phía chặn oan một dòng thật.
func edge(s string, i int) bool {
	if i < 0 || i >= len(s) {
		return true
	}
	r := rune(s[i])
	return !unicode.IsLetter(r) && !unicode.IsDigit(r)
}

// norm — hình để so đường dẫn: một kiểu dấu tách, một kiểu chữ. Không dùng filepath vì
// đây so hai chuỗi model viết ra, không dựng đường thật trên hệ đang chạy.
func norm(s string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), `\`, "/"))
}
