// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package identity

import (
	"strings"
	"testing"
)

// Luật kênh mang phần RIÊNG của một đệ: hai kênh, nhãn của chính vai đó, nếp
// ghi-qua-Shu. Ai giữ vai gì thì không nằm ở đây — nó giống nhau cho mọi đệ nên
// sống ở bản đồ hệ (House.md); chép hai chỗ là để hai bản lệch nhau.
func TestWiringPerAgent(t *testing.T) {
	w := wiring("Sun")
	if w == "" {
		t.Fatal("đệ trong Circle phải có luật kênh")
	}
	// Mỏ neo phải nguyên vẹn trong câu dạy đệ: lõi khớp đúng chuỗi này (`request.DispatchMark`),
	// nên đệ được dạy thiếu emoji là đệ được dạy một dấu lõi không đọc.
	for _, want := range []string{"👋 Tzu gọi —", "🗺️ Báo cáo trinh sát", "qua đệ Shu"} {
		if !strings.Contains(w, want) {
			t.Errorf("luật kênh thiếu %q", want)
		}
	}
	if strings.Contains(w, "⚖️ Phán quyết") {
		t.Error("Sun không được mang nhãn của Han — nhãn là của riêng vai đang hỏi")
	}
}

// Tzu không có nhãn đóng gói: nó là chỗ lời đi ra với người, không phải chỗ
// trả dữ liệu nghề. Thay vào đó nó nhận luật điều phối.
func TestWiringTzuHasNoLabel(t *testing.T) {
	w := wiring("Tzu")
	if strings.Contains(w, "Nhãn đầu ra") {
		t.Error("Tzu không đóng gói dưới nhãn nghề")
	}
	if !strings.Contains(w, "Điều phối một tay") {
		t.Error("Tzu phải nhận luật điều phối")
	}
}

// Ngoài Circle thì không có luật kênh: người đứng bờ chạy dưới dòng, không gọi
// tay; căn cước rỗng thì không đoán vai (#6).
func TestWiringOutsideCircle(t *testing.T) {
	for _, name := range []string{"", "Loiterer", "Outfitter", "Wayfarer", "tên lạ"} {
		if w := wiring(name); w != "" {
			t.Errorf("wiring(%q) phải rỗng, got %d ký tự", name, len(w))
		}
	}
}

// Khớp tên không phân biệt hoa thường, nhưng lời viết ra dùng chính tả của bảng:
// căn cước tới từ tên file, mà tên file thì người gõ.
func TestWiringCaseInsensitive(t *testing.T) {
	if !strings.Contains(wiring("shu"), "đệ Shu") {
		t.Error("khớp tên phải bỏ qua hoa thường, và viết ra đúng chính tả")
	}
}
