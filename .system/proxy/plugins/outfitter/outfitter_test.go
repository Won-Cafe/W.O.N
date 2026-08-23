// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package outfitter

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"won/proxy/core/paths"
	"won/proxy/core/request"
	"won/proxy/core/session"
)

// Danh mục đưa vào tầm mắt model 9B: tên kèm mục đích, một món một dòng. Hướng
// dẫn tool của IDE dài hàng đoạn — rót trọn vào là lấp chỗ nó cần để nghĩ.
func TestRenderKitNameAndPurpose(t *testing.T) {
	kit, dropped := renderKit([]request.ToolInfo{
		{Name: "grep", Description: "Tìm chuỗi trong file.\nHỗ trợ regex đầy đủ, nhiều cờ, nhiều chế độ output."},
		{Name: "write", Description: "Ghi một file ra đĩa"},
		{Name: "mystery"},
	})
	if dropped != 0 {
		t.Errorf("không bỏ món nào mà báo bỏ %d", dropped)
	}
	if !strings.Contains(kit, "- grep — Tìm chuỗi trong file") {
		t.Errorf("thiếu tên + mục đích: %q", kit)
	}
	if strings.Contains(kit, "regex đầy đủ") {
		t.Errorf("phần điều kiện dùng không phải mục đích, phải cắt: %q", kit)
	}
	// Món không có hướng dẫn vẫn phải liệt kê: nó CÓ trong tay người mang, im
	// về nó là nói sai về cái tay đó.
	if !strings.Contains(kit, "- mystery") {
		t.Errorf("món không hướng dẫn vẫn phải có tên: %q", kit)
	}
}

// Cắt thì phải nói ra — danh mục dài hơn tầm mắt thì số món bị bỏ được khai,
// không cắt âm thầm.
func TestRenderKitReportsWhatItDropped(t *testing.T) {
	var many []request.ToolInfo
	for i := 0; i < catalogCap+7; i++ {
		many = append(many, request.ToolInfo{Name: "t"})
	}
	kit, dropped := renderKit(many)
	if dropped != 7 {
		t.Errorf("muốn khai 7 món bị bỏ, got %d", dropped)
	}
	if n := strings.Count(kit, "- t"); n != catalogCap {
		t.Errorf("muốn %d dòng, got %d", catalogCap, n)
	}
}

// Nếp tay: lần liền nhau vào cùng một đích gộp thành ×n, vì `Read ×6` nói chuyện
// khác hẳn `Read, Grep, Write`.
//
// Vòng tool VỪA XONG vẫn đọc được ở lượt người kế tiếp: `Reached` rút từ lịch sử
// trong request, và request nào cũng mang trọn lịch sử. Đó là lý do Outfitter không
// cần chạy giữa vòng tool để thấy nó (plugin.TurnVoice).
// Đường dẫn dựng bằng filepath.Join, không viết tay theo cú pháp của MỘT hệ điều hành:
// `path/filepath` cố ý đổi nghĩa theo hệ đang chạy, nên một gốc `C:\won` viết cứng thì trên
// Unix không phải đường dẫn tuyệt đối, `Rel` trả về đúng một đoạn, và Region rơi vào nhánh
// "thư mục lạ" thay vì kho ký ức. Trước đây test này khai đúng thế và fail suốt trên máy
// không phải Windows — nó tả cái máy của người viết, không tả hợp đồng.
func TestReachedShowsRunsAndTargets(t *testing.T) {
	p := &Outfitter{}
	root := t.TempDir() // gốc tuyệt đối, đúng hình của hệ đang chạy
	p.Paths = paths.Tree{Root: root}
	snap := &request.Snapshot{
		Reached: []request.ToolCall{
			{Name: "Grep", Target: "Stories", Kind: request.TargetPath},
			{Name: "Read", Target: "Stories/Adventure.md", Kind: request.TargetPath},
			{Name: "Read", Target: "Stories/Adventure.md", Kind: request.TargetPath},
			{Name: "Read", Target: "Stories/Adventure.md", Kind: request.TargetPath},
			{Name: "Write", Target: filepath.Join(root, ".system", "memory", "working", "a.md"), Kind: request.TargetPath},
		},
		HumanSpokeLast: true,
	}
	got := p.reached(snap, sessionWith(t, 2, 6))
	for _, want := range []string{
		"Lượt người thứ 2", "chạy 6 lần máy",
		"- Read ×3 → Stories/Adventure.md", "[Stories/]",
		"- Write →", "[" + paths.RegionMemory + "]",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("thiếu %q trong:\n%s", want, got)
		}
	}
}

// Cùng một món vào HAI đích khác nhau không phải một nếp tay — không gộp.
func TestReachedDoesNotMergeDifferentTargets(t *testing.T) {
	p := &Outfitter{}
	got := p.reached(&request.Snapshot{Reached: []request.ToolCall{
		{Name: "Edit", Target: "a.md", Kind: request.TargetPath},
		{Name: "Edit", Target: "b.md", Kind: request.TargetPath},
	}}, sessionWith(t, 1, 4))
	if strings.Contains(got, "×2") {
		t.Errorf("hai đích khác nhau bị gộp:\n%s", got)
	}
}

// Chưa với tới gì thì khai đúng thế — không để trống một chỗ model sẽ tự điền.
func TestReachedIdle(t *testing.T) {
	p := &Outfitter{}
	got := p.reached(&request.Snapshot{HumanSpokeLast: true}, sessionWith(t, 2, 2))
	if !strings.Contains(got, "Chưa với tới món nào") {
		t.Errorf("lượt người vừa nói, chưa dùng tool: %s", got)
	}
}

// Không có đồ nghề nào → không có gì để nói.
func TestRenderKitEmpty(t *testing.T) {
	if kit, _ := renderKit(nil); kit != "" {
		t.Errorf("danh mục rỗng phải trả rỗng, got %q", kit)
	}
}

// Bản kê phải dựng TỪ hằng số của paths, không chép tay: thêm một vùng thì bản kê
// tự đúng. Test này bắt cái chép tay — nó so lời với chính hằng số.
func TestLegendNamesEveryRegionFromPaths(t *testing.T) {
	got := legend()
	for _, want := range []string{
		paths.RegionAxis, paths.RegionMemory, paths.RegionSystem,
		paths.RegionOutside, paths.RegionUnknown,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("bản kê thiếu vùng %q:\n%s", want, got)
		}
	}
	for _, dir := range paths.Axis {
		if !strings.Contains(got, dir+"/") {
			t.Errorf("bản kê thiếu thư mục trục %q:\n%s", dir, got)
		}
	}
	// Bản kê nói vùng là CHỖ NÀO, không nói chỗ nào được phép chạm: phán là việc
	// của soul, đúng thế đứng của Wayfarer.
	for _, verdict := range []string{"được phép", "không được", "cấm", "sai", "vi phạm"} {
		if strings.Contains(got, verdict) {
			t.Errorf("bản kê phán thay soul (%q):\n%s", verdict, got)
		}
	}
}

// Hình dạng mỗi dòng <Reached> mà bản kê hứa phải khớp cái reached() thật in ra —
// hai bản lệch nhau thì model đọc theo bản sai.
func TestLegendMatchesReachedShape(t *testing.T) {
	p := &Outfitter{}
	p.Paths = paths.Tree{Root: t.TempDir()}
	line := p.reached(&request.Snapshot{Reached: []request.ToolCall{
		{Name: "Edit", Target: "Own/a.md", Kind: request.TargetPath},
		{Name: "Edit", Target: "Own/a.md", Kind: request.TargetPath},
	}}, sessionWith(t, 1, 5))
	if !strings.Contains(line, "- Edit ×2 → Own/a.md  ["+paths.RegionAxis+"]") {
		t.Errorf("dòng thật không khớp hình dạng bản kê hứa:\n%s", line)
	}
}

// sessionWith dựng một phiên đã chạy `runs` lần, trong đó người nói `turns` lời khác
// nhau — đúng hình thật: mỗi lời người kéo theo nhiều lần máy chạy tiếp, và lời đứng
// yên suốt những lần đó.
func sessionWith(t *testing.T, turns, runs int) *session.Session {
	t.Helper()
	st := session.NewStore(filepath.Join(t.TempDir(), "state.json"))
	var s *session.Session
	for i := 0; i < runs; i++ {
		// Hội thoại dài dần tới `turns` lượt người, rồi đứng yên — lần chạy sau đó là máy.
		s = st.Touch("k", "", "Tzu", session.Reach{}, min(i+1, turns), time.Now())
	}
	return s
}

// Một câu lệnh KHÔNG phải một chỗ, nên nó không được mang tên vùng. Đo trên dấu thật:
// trong 232 dòng <Reached> đã gửi đi, 35 dòng mang nhãn rác kiểu `cd C:/` hay
// `curl -sL "https:/` — Region nhận cả câu lệnh PowerShell rồi cắt tiền tố của nó ra làm
// tên vùng. Model nhỏ học đúng cái hình ấy rồi tự đúc nhãn cho đích nó chưa hề thấy.
func TestReachedLabelsNonPlaceTargetsByKind(t *testing.T) {
	p := &Outfitter{}
	p.Paths = paths.Tree{Root: t.TempDir()}
	got := p.reached(&request.Snapshot{Reached: []request.ToolCall{
		{Name: "run_in_terminal", Target: `cd C:\won\.system\proxy; go vet ./...`, Kind: request.TargetCommand},
		{Name: "fetch", Target: "https://example.com/a.md", Kind: request.TargetURL},
		{Name: "grep_search", Target: "**/*.go", Kind: request.TargetPattern},
		{Name: "read_file", Target: "Own/a.md", Kind: request.TargetPath},
	}}, sessionWith(t, 3, 4))

	for _, want := range []string{
		"[" + request.TargetCommand.Label() + "]",
		"[" + request.TargetURL.Label() + "]",
		"[" + request.TargetPattern.Label() + "]",
		"[" + paths.RegionAxis + "]",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("thiếu nhãn %q trong:\n%s", want, got)
		}
	}
	// Cái KHÔNG được có: một mảnh câu lệnh đứng ở chỗ tên vùng.
	if strings.Contains(got, "[cd C:") {
		t.Errorf("câu lệnh vẫn bị đọc thành vùng:\n%s", got)
	}
}

// Bản kê phải khai đủ MỌI nhãn loại, dựng từ hằng số của request — thêm một loại thì bản
// kê tự đúng, không phải nhớ sửa hai chỗ.
func TestLegendNamesEveryTargetKind(t *testing.T) {
	got := legend()
	for _, k := range request.LabeledKinds() {
		if !strings.Contains(got, k.Label()) {
			t.Errorf("bản kê thiếu nhãn loại %q:\n%s", k.Label(), got)
		}
	}
}

// Một thước cho mọi nhịp: ngưỡng đọc LẦN CHẠY. Vật liệu của kẻ giữ kho là `<Kit>` và
// `<Reached>`, và cả hai lớn theo lần chạy — số câu người nói không nói gì về việc đã có
// nếp tay nào để đọc hay chưa.
//
// Vì sao bỏ min_turns: đo trên nhật ký thật, ở lượt người thứ 3 runs tích luỹ có phiên
// chỉ 0, tức ngưỡng-đọc-lượt-người cho phép hỏi model về đồ nghề khi chưa món nào được
// dùng. Ngược lại nó KHÔNG BAO GIỜ chạm với một lượt được giao — đệ ấy sống trọn đời
// trong một lượt người, và 5 đệ ngoài Tzu chưa phiên nào vượt 1 lượt (Shu 31 phiên, Sun
// 13, Mo 10, Han 3, Fan 2). Một thước lệch cả hai đầu thì không phải thước.
func TestThresholdCountsRunsNotHumanTurns(t *testing.T) {
	p := &Outfitter{minRuns: 6}

	// Lượt được giao: một lượt người, nhưng đã 6 lần chạy — có nếp tay, được nói.
	if s := sessionWith(t, 1, 6); s.Runs() < p.minRuns {
		t.Errorf("6 lần chạy phải chạm ngưỡng dù chỉ 1 lượt người: runs=%d", s.Runs())
	}
	// Ba lượt người mà mới 2 lần chạy: chưa có gì để đọc, vẫn phải im.
	if s := sessionWith(t, 3, 2); s.Runs() >= p.minRuns {
		t.Errorf("2 lần chạy chưa chạm ngưỡng dù đã 3 lượt người: runs=%d", s.Runs())
	}
	// Một cái liếc vẫn là một cái liếc.
	if s := sessionWith(t, 1, 1); s.Runs() >= p.minRuns {
		t.Errorf("1 lần chạy không được chạm ngưỡng: runs=%d", s.Runs())
	}
}
