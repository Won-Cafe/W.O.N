// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package memory

import (
	"fmt"
	"strings"

	"won/proxy/core/paths"
	"won/proxy/core/session"
)

// zoneSign — mỗi vùng một DẤU: cái để nhận ra một điều vừa nghe thuộc vùng nào. Không
// phải mô tả cái đang nằm trong thư mục, mà là hình của thứ rơi vào đó — vì việc đệ làm
// là nghe một lượt trò chuyện rồi thấy có gì đáng lắng lại không.
//
// Vùng thêm vào paths.Zones mà chưa có dấu ở đây thì hiện tên trần: lõi không bịa nghĩa
// cho thư mục nó chưa biết (#6).
var zoneSign = map[string]string{
	paths.ZoneWorking:    "việc đang theo — mở ra rồi kéo qua nhiều phiên; nó đóng khi người nói xong, không phải khi tắt máy",
	paths.ZoneMoments:    "vừa nảy, chưa kiểm chứng — một quyết định, một chỗ vỡ lẽ, một điều nói ra lần đầu",
	paths.ZoneProcedural: "cách làm đã lặp đủ để tin",
	paths.ZonePersonal:   "điều đã lặp qua nhiều phiên về chính người ấy — thiên hướng, thói quen, chỗ hay vấp",
}

// renderIndex dựng khối rót vào lời hệ thống: kho là gì, dấu của từng vùng, cách dùng,
// rồi index cả kho. Index đứng đây mọi lượt vì nó rẻ (tiền tố cache) và là đường chữa cho
// cái sai duy nhất của bộ chọn: khi bộ chọn im mà trang đáng mở, đệ vẫn THẤY trang tồn
// tại và gọi tên được. Bốn phần đều đứng yên cả phiên nên chúng ở đây; sổ kho thì không —
// nó gọi tới một việc, và chỗ của một lời gọi việc là nhịp lượt (§ Ký ức).
func renderIndex(lines string, dropped, total int, scorer, route string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Memory — index kho ký ức (%d trang)\n\n", total)
	sb.WriteString(renderWhat() + "\n\n")
	if s := renderSigns(); s != "" {
		sb.WriteString(s + "\n\n")
	}
	if w := renderUse(scorer, route); w != "" {
		sb.WriteString(w + "\n")
	}
	sb.WriteString(lines)
	if dropped > 0 {
		fmt.Fprintf(&sb, "(+%d trang nữa không nêu — kho dài hơn trần index)\n", dropped)
	}
	return sb.String()
}

// renderWhat nói kho này là gì và ranh của nó. Hai vế phủ định không thừa: một bên là
// trụ — chữ người tự viết về mình — và trộn vào là lấy cái đệ THẤY đè lên cái người LÀ;
// bên kia là sổ vận hành của máy, thứ không thuộc về ai.
//
// "người dùng hệ này", không phải "người đang nói": khối này rót cho MỌI đệ, mà một đệ
// mở ra như subagent chỉ thấy lời giao của Tzu ở vai người — người thật không có mặt
// trong hội thoại ấy.
func renderWhat() string {
	return "Kho là ký ức của các đệ về NGƯỜI dùng hệ này — không phải trụ của họ, không phải sổ của máy. " +
		"Trang nào trong index cũng mở thẳng được theo đường dẫn, không đợi ai mở hộ."
}

// renderSigns bày dấu của từng vùng, đọc từ paths.Zones — cùng một nguồn mà list() quét,
// nên lời giới thiệu không thể lệch với cái thật sự được đọc.
func renderSigns() string {
	parts := make([]string, 0, len(paths.Zones))
	for _, z := range paths.Zones {
		line := "- `" + z + "/`"
		if s := zoneSign[z]; s != "" {
			line += " — " + s
		}
		parts = append(parts, line)
	}
	if len(parts) == 0 {
		return ""
	}
	return "Các vùng, và dấu để nhận ra một điều vừa nghe thuộc vùng nào:\n" + strings.Join(parts, "\n")
}

// renderUse khai CÁCH DÙNG kho: thấy dấu thì làm gì, chỗ của kho trong lượt, bút nằm ở
// đâu, rồi hai vành đai cơ học. Không có phần này thì khối đọc ra là một cuốn mục lục
// chỉ-đọc — đo được trên kho thật, 80 phiên không sinh một trang nào và `procedural/` rỗng trọn.
//
// Vì sao nó ở đây mà không ở soul: soul là bản thể mười một trục, không mang cơ học. Chép
// "gọi Shu ghi memory" vào soul thì tắt plugin xong soul vẫn dặn đi qua một cái cửa không
// còn ở đó. Cơ học đến cùng cơ chế và biến mất cùng nó — khối này chỉ tồn tại khi plugin bật.
//
// KHÔNG khai một hành động chỉ MỘT vai làm được. Khối này rót cho mọi đệ, mà "giao đệ Shu
// ghi" thì chỉ Tzu thi hành được — đệ không gọi đệ, và chính người cầm bút đọc câu ấy là
// đọc một lệnh tự trỏ vào mình. Lời chung cho cả ba cảnh là "nói ra": Tzu nói rồi điều
// phối, đệ khác nói trong phần trả về, người cầm bút nói vì sao đáng trước khi chép.
//
// Hai dòng cuối là ràng buộc thật, không phải mô tả: trang thiếu tiêu đề + mô tả nằm trên
// đĩa mà không vào index, nên bộ chọn không chọn tới và đệ không gọi tên được — mất trong
// im lặng. Và sỏi chỉ vào được qua đúng một cửa.
func renderUse(scorer, route string) string {
	who := pen(scorer)
	var sb strings.Builder
	sb.WriteString("Nghe thấy một dấu như thế mà kho chưa có trang nào giữ nó thì nói ra — nói vì sao đáng, trước khi có ai đặt bút.\n")
	sb.WriteString(renderTouch() + "\n")
	fmt.Fprintf(&sb, "Mạch chỉ đọc, không ghi: bút của kho nằm ở %s — trang mới, trang dời vùng, trang hạ độ tin đều qua tay đó.\n", who)
	sb.WriteString("Một trang là `vùng/tên.md`, mở bằng một dòng `# tiêu đề` rồi ngay dưới một dòng `*mô tả*` in nghiêng — thiếu hai dòng ấy thì trang nằm trên đĩa mà không vào được index này.\n")
	if scorer != "" && route != "" {
		fmt.Fprintf(&sb, "Sỏi `s`/`f` là số của máy: ghi qua `%s` trên Control API, chỉ %s mở được cửa ấy — không đặt tay vào frontmatter.\n", route, who)
	}
	return sb.String()
}

// renderSeed — lời mời khi kho chưa có trang nào. Nói đúng những gì `renderUse` nói, trừ
// index và trừ sỏi: kho là gì, dấu của từng vùng, chỗ của kho trong lượt, hình một trang,
// bút nằm ở đâu. Cả hai đường về sau đều cần một trang đã có — `nudge` đọc sổ trang,
// `renderUse` đi kèm index — nên kho rỗng thì đây là lời duy nhất còn nói được.
//
// KHÔNG nhắc `s`/`f` và cửa Control API: sỏi là việc của trang đã sống, và một lời gọi việc
// chưa dùng tới thì chỉ là chữ chở thêm (#3).
//
// Tiêu đề mang tên plugin: khối này đi ở nhịp lượt, và ở đó #4 kiểm trên text TRẦN, trước
// khi lõi bọc tag.
func renderSeed(scorer string) string {
	var sb strings.Builder
	sb.WriteString("## Memory — kho ký ức chưa có trang nào\n")
	sb.WriteString(renderWhat() + "\n")
	if s := renderSigns(); s != "" {
		sb.WriteString(s + "\n")
	}
	fmt.Fprintf(&sb, "Nghe thấy một dấu như thế mà kho chưa giữ nó thì nói ra — nói vì sao đáng, trước khi có ai đặt bút. Mạch chỉ đọc, không ghi: bút của kho nằm ở %s.\n", pen(scorer))
	sb.WriteString(renderTouch() + "\n")
	sb.WriteString("Một trang là `vùng/tên.md`, mở bằng một dòng `# tiêu đề` rồi ngay dưới một dòng `*mô tả*` in nghiêng — thiếu hai dòng ấy thì trang nằm trên đĩa mà không vào được index.\n")
	return sb.String()
}

// renderTouch khai CHỖ của kho trong một lượt — cái từng thiếu: lời nhắc thua vì không có
// vị trí trong nhịp, không phải vì chữ yếu (§ Ký ức). Vế "đệ mang nhịp" là có chủ đích:
// khối rót cho mọi đệ mà bốn mốc là nhịp của một vai — vai có mốc nhận tên mốc, vai khác
// vẫn nhận cái lõi: đọc sớm, bút muộn.
func renderTouch() string {
	return "Chỗ của kho trong một lượt: đọc và nhận ra ngay sau khi ngẫm và dò thêm — đệ mang nhịp bốn mốc gọi chỗ ấy là 🧠 Nhớ — còn mọi cú ghi dồn về cuối lượt, khi sự thật của lượt đã tròn."
}

// pen gọi tên đệ cầm bút, dựng từ chính núm `scorer` — MỘT tên, một nguồn, hai chỗ đọc
// (khối hệ thống và sổ kho). Chép cứng "Shu" là một bản nói sai khi núm được vặn mà không
// ai báo; đọc từ núm thì lời khai không bao giờ chọi với cái plugin thật sự cho qua cửa.
// Vắng lời khai thì gọi bằng vai, không bịa một cái tên (#6).
func pen(scorer string) string {
	if scorer == "" {
		return "đệ biên ghi"
	}
	return "đệ " + scorer
}

// renderOpened dựng khối theo lượt: các trang đã mở, mỗi trang kèm đường dẫn làm nguồn.
//
// Mở đầu bằng một dòng TỰ KHAI, không phải để đẹp: cưỡng chế #4 kiểm trên text trần,
// trước khi lõi bọc tag — mà ruột khối này là chữ của chính người dùng, không quy về ai
// được. Dòng ấy cũng là chỗ khai đường đi tiếp: index nêu mọi trang và đệ có tay, không
// khai thì đệ tưởng những trang kia không với tới được.
func renderOpened(opened []session.Page) string {
	if len(opened) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("## Memory — trang mở cho lượt này\n")
	sb.WriteString("(trang khác trong index thì đọc thẳng theo đường dẫn)\n")
	for _, o := range opened {
		fmt.Fprintf(&sb, "\n---\n\n## %s\n\n%s\n", o.Path, o.Text)
	}
	return sb.String()
}
