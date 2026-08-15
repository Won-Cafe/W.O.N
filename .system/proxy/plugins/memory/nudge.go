// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package memory

import (
	"fmt"
	"sort"
	"strings"

	"won/proxy/core/paths"
	"won/proxy/core/session"
)

const (
	// nudgeAge — trang phải có mặt qua chừng này phiên rồi mới đáng nhắc.
	nudgeAge = 3
	// nudgeRipe — nhiều nhất mấy dòng cho đầu "đã chín", để đầu "đang nhạt" còn chỗ.
	nudgeRipe = 1
	// nudgeLines — nhắc gọn. Kể trọn sổ mỗi phiên là dìm đúng thứ đáng nhìn.
	nudgeLines = 3
)

// nudge dựng sổ kho, tính một lần mỗi phiên. Vì sao cần: việc chuyển vùng của kho
// (củng cố · phai · lắng đọng) là việc phía ghi, mà ghi là bút của Shu, mà chưa có gì
// gọi Shu dậy. Không có lời nhắc thì một trang viết một lần, hai mươi năm sau vẫn nguyên
// trạng thái "chưa củng cố, chưa phai" — kho đứng yên vì vòng lặp không khép.
// Ranh với trục sống của một trang (tái củng cố): mạch chỉ đếm được lần mở và tuổi
// phiên, nên nó chỉ thấy trang nằm vùng nào; cái mới chọi cái cũ thì chỉ bút đọc ra.
// Nó nói SỐ, không phán. Đệ đọc rồi tự quyết. Ranh với Wayfarer: Wayfarer nói về
// *đường* và đo bằng NGÀY từ dấu thời gian của tệp; đây nói về *kho* và đo bằng PHIÊN
// từ sổ trang của chính nó. Hai trục, hai nguồn — trộn chúng là hai bản lệch được.
func nudge(pages []page, sess *session.Session, stoneWeight int, scorer string) string {
	stats, now := sess.PageStats()
	// Sổ chưa đủ phiên thì chưa có nếp nào để đọc: mọi trang còn mới, và hỏi một kho vừa
	// dựng "sao chưa lớn" là cằn nhằn. Cùng ngưỡng với tuổi trang bên dưới.
	if now < nudgeAge {
		return ""
	}
	type row struct {
		path string
		pg   page
		stat session.PageStat
		age  int // số phiên trang đã có mặt
	}
	// HAI ĐẦU sổ: trang moments/ được mở đi mở lại (đã chín, còn nằm vùng chưa kiểm
	// chứng) và trang chưa ai mở lại (đang nhạt). Xếp chung một bảng rồi cắt ba dòng
	// thì đầu "chưa ai mở" bị đẩy khỏi bảng — mất đúng nửa cần nhắc.
	var ripe, fading []row
	// Ba con số về HIỆN TẠI, gom trong cùng vòng quét: phiên trang mới nhất vào sổ, số
	// trang đang mở ở working/, và tuổi trang working/ cũ nhất. Trang chưa vào sổ nghĩa là kho
	// vừa nhận nó trong phiên này — đó chính là dấu kho còn lớn.
	newest, fresh, open, oldest := 0, 0, 0, 0
	for _, pg := range pages {
		inFocus := strings.HasPrefix(pg.Path, paths.ZoneWorking+"/")
		if inFocus {
			open++
		}
		st, ok := stats[pg.Path]
		if !ok {
			fresh++
			continue // trang mới thấy phiên này — chưa có gì để nói về nếp của nó
		}
		if st.FirstSession > newest {
			newest = st.FirstSession
		}
		age := now - st.FirstSession
		if inFocus && age > oldest {
			oldest = age
		}
		if age < nudgeAge {
			continue
		}
		r := row{path: pg.Path, pg: pg, stat: st, age: age}
		switch {
		case st.Opens == 0:
			fading = append(fading, r)
		case strings.HasPrefix(pg.Path, paths.ZoneMoments+"/"):
			ripe = append(ripe, r)
		}
		// Trang ở vùng bền và vẫn được mở: không có gì gọi ai làm gì — không nhắc.
	}
	sort.Slice(ripe, func(i, j int) bool { return ripe[i].stat.Opens > ripe[j].stat.Opens })
	sort.Slice(fading, func(i, j int) bool { return fading[i].age > fading[j].age })

	var head strings.Builder
	// Kho vừa nhận trang trong phiên này thì không có gì để đếm — im. Đếm chỉ nói được
	// điều gì khi quãng đứng yên đã dài hơn tuổi một trang còn mới.
	if fresh == 0 && newest > 0 && now-newest >= nudgeAge {
		fmt.Fprintf(&head, "Trang mới nhất vào sổ từ phiên %d — %d phiên kho chưa nhận trang nào mới.\n", newest, now-newest)
	}
	if open == 0 {
		fmt.Fprintf(&head, "`%s/` — không trang nào đang mở.\n", paths.ZoneWorking)
	} else {
		fmt.Fprintf(&head, "`%s/` — %d trang đang mở, trang cũ nhất có mặt qua %d phiên.\n", paths.ZoneWorking, open, oldest)
	}
	if head.Len() == 0 && len(ripe) == 0 && len(fading) == 0 {
		return ""
	}

	var sb strings.Builder
	// Tiêu đề mang tên plugin vì khối này đi ở nhịp lượt, và ở đó bất biến #4 kiểm trên
	// text trần trước khi lõi bọc tag.
	fmt.Fprintf(&sb, "## Memory — sổ kho, phiên %d\n", now)
	// Gọi TÊN người cầm bút, không gọi "bút": đây là chỗ model đọc sau chót, và một lời
	// gọi việc không có địa chỉ thì đệ phải tự bắc cầu sang bản đồ hệ mới biết giao cho ai.
	fmt.Fprintf(&sb, "Việc chuyển vùng của kho (củng cố · phai · lắng đọng) là việc của %s, không phải của mạch.\n", pen(scorer))
	sb.WriteString(head.String())
	for _, r := range take(ripe, nudgeRipe) {
		fmt.Fprintf(&sb, "- %s —", r.path)
		if r.pg.S+r.pg.F > 0 {
			fmt.Fprintf(&sb, " sỏi %d (s%d/f%d),", stone(r.pg.S, r.pg.F, stoneWeight), r.pg.S, r.pg.F)
		}
		fmt.Fprintf(&sb, " mở %d lần, lần gần nhất phiên %d, có mặt qua %d phiên, vẫn ở moments/.\n",
			r.stat.Opens, r.stat.LastOpen, r.age)
	}
	for _, r := range take(fading, nudgeLines-min(len(ripe), nudgeRipe)) {
		fmt.Fprintf(&sb, "- %s — có mặt qua %d phiên, chưa lần nào được mở lại.\n", r.path, r.age)
	}
	return sb.String()
}

func take[T any](in []T, n int) []T {
	if n < 0 {
		n = 0
	}
	if len(in) > n {
		return in[:n]
	}
	return in
}
