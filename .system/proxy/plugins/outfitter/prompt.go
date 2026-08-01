// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package outfitter

import (
	"fmt"
	"path/filepath"
	"strings"

	"won/proxy/core/paths"
	"won/proxy/core/request"
	"won/proxy/core/session"
)

// Dựng lời hỏi cho model nền. Phần cơ học của prompt sống ở đây, không trong soul:
// nó tả cái plugin thật sự gửi đi, nên nó phải đổi cùng chỗ với code gửi.

// legend — bản kê: khối nào có mặt, mỗi dòng đọc thế nào, tên vùng nghĩa là gì. Dựng
// từ hằng số của paths, không chép tay. Chỉ nói vùng là CHỖ NÀO — chỗ nào được phép
// chạm là việc của soul.
func legend() string {
	var sb strings.Builder
	sb.WriteString("---\n\nBẢN KÊ — cơ học, tả đúng những gì đặt trước mắt bạn lượt này:\n")
	sb.WriteString("- `<Kit>`: đồ nghề người mang đang có, mỗi dòng `<tên món> — <mục đích>`.\n")
	sb.WriteString("- `<Reached>`: dấu tay vừa để lại, cũ → mới, mỗi dòng " +
		"`<tên món> ×<số lần liền nhau> → <đích>  [<vùng>]`.\n")
	sb.WriteString("- `<Conversation>`: vài lượt gần nhất, mở đầu bằng lời người mở lượt.\n")
	sb.WriteString("- Đích rỗng = món đó không nhận tham số nào nói chỗ; lúc ấy chỉ có tên món.\n")
	sb.WriteString("- Vùng là chỗ cái đích nằm trong cây W.O.N:\n")
	// Tree gốc-rỗng: cùng chỗ dựng đường thật, nên bản kê không lệch với Region().
	bare := paths.Tree{}
	for _, r := range []struct{ name, holds string }{
		{paths.RegionAxis, strings.Join(paths.Axis, "/ · ") + "/"},
		{paths.RegionMemory, slash(bare.Memory()) + "/"},
		{paths.RegionSystem, slash(bare.Proxy()) + "/ và phần còn lại của " + paths.Marker + "/"},
		{paths.RegionOutside, "không nằm dưới gốc cây"},
		{paths.RegionUnknown, "không đọc ra được chỗ"},
	} {
		sb.WriteString("  - `" + r.name + "` — " + r.holds + "\n")
	}
	sb.WriteString("  - `<tên>/` — một thư mục khác dưới gốc, gọi đúng tên nó\n")
	return sb.String()
}

func slash(p string) string { return strings.TrimPrefix(filepath.ToSlash(p), "/") }

// reached bày nếp tay: món nào, vào đâu, lần liền nhau gộp thành `×n`. Đích kèm VÙNG
// vì model nền không tự biết `Own/a.md` khác `Stories/a.md` ở chỗ nào. Không một chữ
// nói nếp tay tốt hay xấu.
func (p *Outfitter) reached(snap *request.Snapshot, sess *session.Session) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Lượt người thứ %d trong phiên; đệ đã chạy %d lần máy.",
		sess.Turns(), sess.Runs())
	if len(snap.Reached) == 0 {
		sb.WriteString(" Chưa với tới món nào.")
		return sb.String()
	}

	sb.WriteString("\n")
	for _, r := range runs(snap.Reached) {
		sb.WriteString("- " + r.Name)
		if r.n > 1 {
			fmt.Fprintf(&sb, " ×%d", r.n)
		}
		if r.Target != "" {
			sb.WriteString(" → " + r.Target + "  [" + p.Paths.Region(r.Target) + "]")
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// run — một món với tới n lần liền nhau vào cùng một đích.
type run struct {
	request.ToolCall
	n int
}

// runs gộp theo cả tên VÀ đích: `Edit ×3` vào một file là một nếp tay, ba file khác
// nhau là nếp khác.
func runs(calls []request.ToolCall) []run {
	var out []run
	for _, c := range calls {
		if n := len(out); n > 0 && out[n-1].ToolCall == c {
			out[n-1].n++
			continue
		}
		out = append(out, run{ToolCall: c, n: 1})
	}
	return out
}

// renderKit dựng danh mục "tên — mục đích", một món một dòng. Món không có hướng
// dẫn thì vẫn liệt kê tên. Trả kèm số món bị bỏ ngoài tầm mắt.
func renderKit(tools []request.ToolInfo) (string, int) {
	if len(tools) == 0 {
		return "", 0
	}
	shown, dropped := tools, 0
	if len(shown) > catalogCap {
		dropped = len(shown) - catalogCap
		shown = shown[:catalogCap]
	}

	var sb strings.Builder
	for _, t := range shown {
		sb.WriteString("- " + t.Name)
		if purpose := firstLine(t.Description); purpose != "" {
			sb.WriteString(" — " + request.Truncate(purpose, purposeCap))
		}
		sb.WriteString("\n")
	}
	return sb.String(), dropped
}

// firstLine lấy câu mở đầu của hướng dẫn — chỗ nói món này để làm gì.
func firstLine(desc string) string {
	desc = strings.TrimSpace(desc)
	if i := strings.IndexAny(desc, "\n."); i > 0 {
		desc = desc[:i]
	}
	return strings.TrimSpace(desc)
}
