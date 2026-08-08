// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package debug

import (
	"encoding/json"
	"testing"
)

// Round-trip không đụng gì phải trả lại ĐÚNG thứ tự khoá gốc — kể cả khi thứ
// tự đó không phải bảng chữ cái, thứ mà map[string]any của Go sẽ tự xáo lại.
func TestOrderedMapRoundTripPreservesKeyOrder(t *testing.T) {
	raw := `{"zebra":1,"apple":2,"mango":3}`
	var m orderedMap
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatal(err)
	}
	got, err := json.Marshal(&m)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != raw {
		t.Errorf("thứ tự khoá bị xáo: muốn %s, got %s", raw, got)
	}
}

// set trên khoá đã có phải THAY giá trị tại đúng vị trí cũ, không dời ra cuối.
func TestOrderedMapSetExistingKeyKeepsPosition(t *testing.T) {
	var m orderedMap
	json.Unmarshal([]byte(`{"a":1,"b":2,"c":3}`), &m)
	m.set("b", json.RawMessage(`99`))
	got, _ := json.Marshal(&m)
	if string(got) != `{"a":1,"b":99,"c":3}` {
		t.Errorf("vị trí bị đổi: %s", got)
	}
}

// set trên khoá mới phải thêm vào CUỐI — không có "vị trí gốc" nào để giữ.
func TestOrderedMapSetNewKeyAppends(t *testing.T) {
	var m orderedMap
	json.Unmarshal([]byte(`{"a":1}`), &m)
	m.set("b", json.RawMessage(`2`))
	got, _ := json.Marshal(&m)
	if string(got) != `{"a":1,"b":2}` {
		t.Errorf("khoá mới không nằm ở cuối: %s", got)
	}
}

func TestOrderedMapGetMissingKey(t *testing.T) {
	var m orderedMap
	json.Unmarshal([]byte(`{"a":1}`), &m)
	if _, ok := m.get("missing"); ok {
		t.Error("khoá không tồn tại phải trả ok=false")
	}
}
