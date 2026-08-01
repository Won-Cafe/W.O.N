// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package main

import (
	"net"
	"testing"

	"won/proxy/core/config"
	"won/proxy/services/logging"
)

// `debug_log` là núm RIÊNG: ghi thân request xuống đĩa (#5) không còn dính vào việc in ra
// màn hình. Ba trạng thái, và trạng thái "chưa khai" phải giữ nguyên hành vi cũ — nâng cấp
// không được đổi hành vi của người chưa sửa gì.
func TestDebugLogKnobIsIndependentOfLogLevel(t *testing.T) {
	yes, no := true, false
	cases := []struct {
		name  string
		knob  *bool
		level string
		want  bool
	}{
		{"chưa khai + debug = có, đúng như trước", nil, logging.LevelDebug, true},
		{"chưa khai + info = không, đúng như trước", nil, logging.LevelInfo, false},
		{"chưa khai + silent = không", nil, logging.LevelSilent, false},
		{"khai true + info = console yên mà vẫn ghi", &yes, logging.LevelInfo, true},
		{"khai true + silent = im hẳn mà vẫn ghi", &yes, logging.LevelSilent, true},
		{"khai false + debug = console ồn mà không ghi", &no, logging.LevelDebug, false},
	}
	for _, c := range cases {
		cfg := &config.Config{DebugLog: c.knob}
		if got := debugLogOn(cfg, c.level); got != c.want {
			t.Errorf("%s: được %v, muốn %v", c.name, got, c.want)
		}
	}
}

// Buồng lái là mặt TUỲ CHỌN, nên hai ca phải khác nhau rõ: không khai = không cửa và
// KHÔNG lỗi (trạng thái bình thường); khai mà cổng bị giữ = lỗi trả về cho main quyết,
// và main chọn chạy tiếp không buồng lái (#2). Gộp hai ca ấy làm một là cách một cổng
// bị tiến trình cũ giữ làm chết cả dòng chính.
func TestListenControl(t *testing.T) {
	ln, err := listenControl("")
	if err != nil || ln != nil {
		t.Fatalf("không khai thì không cửa, không lỗi: %v %v", ln, err)
	}

	held, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()

	if ln, err := listenControl(held.Addr().String()); err == nil {
		if ln != nil {
			ln.Close()
		}
		t.Fatal("cổng đang có người giữ phải trả lỗi, để main còn kêu lên rồi chạy tiếp")
	}

	free, err := listenControl("127.0.0.1:0")
	if err != nil || free == nil {
		t.Fatalf("cổng rảnh phải mở được: %v %v", free, err)
	}
	free.Close()
}
