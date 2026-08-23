// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package base

import (
	"strings"
	"unicode"

	"won/proxy/core/request"
)

const (
	// Silent — lời đáp cho "tôi không nói gì lượt này", chung cho cả ba giọng.
	Silent = "∅"

	// lineCap — trần chữ một dòng. Ở đây, không ở từng plugin: một luật chép ba chỗ
	// là ba bản có thể lệch.
	lineCap = 400

	// stampCap — dấu tên chỉ được tìm trong quãng đầu dòng. Xa hơn thì dấu hai chấm
	// là dấu câu của người viết, và bóc nó là cắt mất nửa câu.
	stampCap = 48
)

// Contract dựng hợp đồng đầu ra: hai dạng, và không dạng nào đòi marker — marker do
// Say đeo. Model nền chỉ còn một quyết định, nói hay im.
//
// Hợp đồng khai HÌNH của hai dạng, KHÔNG khai luật nghề: "nói hay im" và mọi ranh giới
// của một vai thuộc soul (§ Ba dấu của Outfitter, bảng ranh giới).
//
// Hai thứ từng nằm ở đây và đã bỏ, cùng một lý do: mỗi tiếng có một vế FINAL GATE và một
// vế "shapes of a line", mà cả sáu vế ấy đọc lại Will và Origin của chính soul — bản thứ
// hai của một luật đã có nhà, và hai bản thì lệch được. Bỏ vế cửa-cuối thì tham số
// `finalGate` chết theo; `name` với `marker` thì chết từ trước, vì hàm không đọc chúng.
//
// Cái ở lại là thứ soul không nói được: hình của hai dạng, trần một dòng, và TÊN các khối
// thật trong lời hỏi — soul nói *cái nó thấy*, không nói *khối nào chở cái đó*.
func Contract(emptyMeans string, extra ...string) string {
	var sb strings.Builder
	sb.WriteString("---\n\nOUTPUT CONTRACT — mechanical, overrides style:\n")
	sb.WriteString("- Return exactly one of two forms. There is no third form.\n")
	sb.WriteString("- `" + Silent + "` — " + emptyMeans + "\n")
	sb.WriteString("- Otherwise: the line itself, and nothing else. No name, no label, no quotes.\n")
	sb.WriteString("- Exactly one line, at most two sentences (~40 words). No preamble.\n")
	for _, e := range extra {
		if e != "" {
			sb.WriteString("- " + e + "\n")
		}
	}
	return sb.String()
}

// Line — lời model chủ ý nói, đã bóc dấu, đã cắt trần. Rỗng = im lặng. Đây là dạng vào
// SỔ (`Session.Say`): sổ giữ CHỮ, vì cái quay lại prompt ở lượt sau phải là điều đã nói,
// không phải cách lõi đóng dấu nó (§ Marker do lõi đeo).
func Line(marker, name, out string) string {
	line := pickLine(marker, out)
	if line == "" || strings.HasPrefix(line, Silent) {
		return ""
	}
	if line = trimStamp(line, marker, name); line == "" {
		return ""
	}
	return request.Truncate(line, lineCap)
}

// Say dựng dòng sẽ chèn: `<marker> <Name>: <một dòng>`. Marker đến từ ĐÂY, không từ
// model: bất biến #4 không được đứng trên việc model có chịu tuân khuôn hay không.
func Say(marker, name, out string) string {
	line := Line(marker, name, out)
	if line == "" {
		return ""
	}
	return Stamp(marker, name, line)
}

// Stamp đeo marker và tên vào một dòng đã bóc sạch. Tách khỏi Say để chỗ nào cần cả
// hai dạng — dạng vào sổ và dạng vào request — không phải bóc lại lời của chính mình.
func Stamp(marker, name, line string) string {
	if line == "" {
		return ""
	}
	return marker + " " + name + ": " + line
}

// pickLine — dòng model chủ ý nói: ưu tiên dòng nó tự đeo marker, và chỉ nhận dòng CÓ
// CÂU. Model nhỏ chép khuôn của đệ dòng chính nên nó hay đặt một tiêu đề trước thân câu,
// mà một tiêu đề không phải một lời (§ Marker do lõi đeo).
func pickLine(marker, out string) string {
	lines := strings.Split(stripThinking(out), "\n")
	// Model tự đeo marker thì dòng ấy là dòng nó chủ ý — nhưng chỉ khi nó mang một câu.
	for _, ln := range lines {
		if ln = strings.TrimSpace(ln); strings.HasPrefix(ln, marker) && !decorative(ln, marker) {
			return ln
		}
	}
	for _, ln := range lines {
		if ln = strings.TrimSpace(ln); ln != "" && !decorative(ln, marker) {
			return ln
		}
	}
	return ""
}

// stripThinking bỏ khối ``` MỞ ĐẦU — vài model nhỏ đặt khối suy nghĩ ở đó. Chỉ khi nó mở
// đầu: một fence ở giữa hay ở cuối là chữ của lời đáp, và cắt theo nó thì phần trước fence
// mất sạch.
func stripThinking(out string) string {
	s := strings.TrimSpace(out)
	if !strings.HasPrefix(s, "```") {
		return out
	}
	if i := strings.Index(s[len("```"):], "```"); i >= 0 {
		return s[2*len("```")+i:]
	}
	return s[len("```"):] // fence không đóng: bỏ dấu mở, giữ chữ
}

// decorative — dòng này là NHÃN, không phải lời: bóc dấu trang trí hai đầu, còn lại một
// chữ đơn thì là nhãn. Giá phải trả: một lời thật dài đúng một chữ cũng rơi về im lặng —
// hợp đồng đòi một câu, nên đó là cái giá đúng chỗ.
func decorative(line, marker string) bool {
	s := strings.TrimSpace(strings.TrimPrefix(line, marker))
	trim := func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }
	s = strings.TrimFunc(s, trim)
	return s == "" || !strings.ContainsFunc(s, unicode.IsSpace)
}

// trimStamp bóc marker và dấu tên nếu model đã tự đeo — đeo hai lần thì dòng thành
// `🧰 Outfitter: 🧰 Outfitter: …`. Khớp cả `Loiterer:` lẫn `Loiterer (Anh xe ôm):`.
// Bóc bằng TrimMarkers nên marker của đệ KHÁC cũng rụng: model nhỏ chép `🤔` của Tzu
// vào đầu dòng, và dòng ta gửi đi chỉ được mang một dấu.
func trimStamp(line, marker, name string) string {
	line = request.TrimMarkers(strings.TrimPrefix(line, marker))
	if i := strings.Index(line, ":"); i > 0 && i <= stampCap {
		if strings.Contains(strings.ToLower(line[:i]), strings.ToLower(bareName(name))) {
			line = strings.TrimSpace(line[i+1:])
		}
	}
	return strings.TrimSpace(strings.Trim(line, `"`))
}

// bareName — "Loiterer (Anh xe ôm)" → "Loiterer".
func bareName(name string) string {
	if i := strings.Index(name, " ("); i > 0 {
		return name[:i]
	}
	return name
}
