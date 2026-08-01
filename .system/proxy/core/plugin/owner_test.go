// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package plugin

import "testing"

// Plugin trần: không nhận quyền nào.
type bare struct{ fake }

// Plugin nhận quyền.
type owner struct{ fake }

func (owner) OwnsSystem() bool { return true }

// Plugin khai quyền rồi tự tắt — khai là một câu trả lời, không phải một cái nhãn.
type declined struct{ fake }

func (declined) OwnsSystem() bool { return false }

// Lõi không biết tên plugin nào; nó chỉ hỏi "có ai nhận việc này không".
// Vắng người nhận → lõi không chạm gì, đó là nghĩa của "còn lại giữ nguyên".
func TestOwnership(t *testing.T) {
	cases := []struct {
		name       string
		plugins    []Plugin
		wantSystem bool
	}{
		{"không plugin nào", nil, false},
		{"toàn plugin trần", []Plugin{bare{fake{name: "a"}}, bare{fake{name: "b"}}}, false},
		{"có người nhận", []Plugin{bare{fake{name: "a"}}, owner{fake{name: "b"}}}, true},
		{"khai rồi tự tắt", []Plugin{declined{fake{name: "a"}}}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := SystemOwned(c.plugins); got != c.wantSystem {
				t.Errorf("SystemOwned = %v, want %v", got, c.wantSystem)
			}
		})
	}
}
