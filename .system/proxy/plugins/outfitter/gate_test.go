// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package outfitter

import (
	"testing"

	"won/proxy/core/request"
)

// Mọi ca trong file này là dòng THẬT đã lọt ra, đọc từ `run/*/debug_detail.json`. Trên
// 904 lượt outfitter chạy, model nói 10 lần: 3 lần bịa một đích chưa ai với tới, 2 lần
// nhét văn xuôi vào ô đích, 1 lần chỉ vào người, và 1 dòng nói hai lần trong cùng phiên.
// Cửa gác bằng lời nhắc đã có sẵn lúc ấy, và nó không giữ được lượt nào.
//
// Fixture dựng lại HÌNH của dòng thật, không chép nội dung thật: một đường dẫn tuyệt đối
// kiểu Windows, một trang trong kho ký ức, một câu văn xuôi. Tên máy người dùng và tên
// việc thật của họ không có chỗ trong một file đi vào git (#5).
var kitReal = []request.ToolInfo{
	{Name: "read_file"}, {Name: "multi_replace_string_in_file"},
	{Name: "run_in_terminal"}, {Name: "mcp_exa_web_fetch_exa"}, {Name: "runSubagent"},
}

var namesReal = []string{"Tzu", "Sun", "Mo", "Fan", "Han", "Shu", "Outfitter"}

func TestKeepDropsInventedTarget(t *testing.T) {
	reached := []request.ToolCall{
		{Name: "read_file", Target: `C:\won\.system\memory\working\viec-dang-lam.md`, Kind: request.TargetPath},
		{Name: "runSubagent"},
	}
	// Cả ba dòng này đã lọt ra thật. Không đích nào có mặt trong dấu tay của lượt ấy, và
	// hai trong ba trỏ vào file không tồn tại trên đĩa.
	for _, line := range []string{
		"read_file → What/World/World.md → [kho ký ức]",
		"run_in_terminal → ./test-proxy-inject.sh [tầng cơ học]",
		`read_file → C:\won\.system\memory\README.md [tầng cơ học]`,
	} {
		if why := keep(line, kitReal, reached, namesReal, nil); why == "" {
			t.Errorf("đích bịa phải bị chặn: %q", line)
		}
	}
}

// Đích có thật thì qua, kể cả khi model nhắc nó ở dạng ngắn hơn bản kê: bản kê cắt đích
// ở 120 ký tự, nên đòi khớp từng ký tự là đòi cái bản kê không hứa.
func TestKeepAllowsRealTargetInShortForm(t *testing.T) {
	reached := []request.ToolCall{
		{Name: "read_file", Target: `C:\won\What\World\World.md`, Kind: request.TargetPath},
	}
	if why := keep("read_file → What/World/World.md [trục W.O.N]", kitReal, reached, namesReal, nil); why != "" {
		t.Errorf("đích có thật bị chặn oan: %s", why)
	}
}

// Văn xuôi trong ô đích: không phải một chỗ, nên không phải một dòng của kẻ giữ kho.
// README nói kẻ này trỏ vào "món và chỗ món đã đi" — một câu chỉ việc thì không có chỗ nào.
//
// Nguyên văn dòng thật gọi tên một tờ chỉ có ở cây của người dựng. File này đi theo bản
// giao, và cổng an toàn của bản giao chặn mọi chữ như thế — người nhận không có tờ ấy, nên
// một câu nêu tên nó là một câu trỏ vào chỗ trống. Fixture giữ HÌNH, đổi chữ.
func TestKeepDropsProseAsTarget(t *testing.T) {
	reached := []request.ToolCall{{Name: "mcp_exa_web_fetch_exa"}}
	for _, line := range []string{
		"mcp_exa_web_fetch_exa → đọc trang vừa tải để so sánh với nội dung repo, rồi xác định chỗ cần sửa.",
		"multi_replace_string_in_file → sửa lại chỗ vừa ghi sai rồi gọi lại một lần nữa.",
	} {
		if why := keep(line, kitReal, reached, namesReal, nil); why == "" {
			t.Errorf("văn xuôi trong ô đích phải bị chặn: %q", line)
		}
	}
}

// Chỉ vào người thì chặn, kể cả khi hợp đồng đã nhắc bằng lời. Dòng này lọt ra thật.
func TestKeepDropsLineNamingAPerson(t *testing.T) {
	reached := []request.ToolCall{{Name: "multi_replace_string_in_file", Target: "Own/Origin/Origin.md", Kind: request.TargetPath}}
	line := "multi_replace_string_in_file → Own/Origin/Origin.md, việc Shu vẫn đang cầm"
	if why := keep(line, kitReal, reached, namesReal, nil); why != "chỉ vào người" {
		t.Errorf("tên người phải bị chặn ở đúng cửa đó, got %q", why)
	}
}

// Một đường dẫn CÓ tên đệ trong nó vẫn là một chỗ. Chặn nó là chặn mất cái đích hợp lệ
// hay gặp nhất trong hệ này: file soul của chính các đệ.
func TestKeepAllowsAgentNameInsideAPath(t *testing.T) {
	reached := []request.ToolCall{
		{Name: "read_file", Target: `C:\won\.system\agents\Tzu.agent.md`, Kind: request.TargetPath},
	}
	line := `read_file ×6 → .system/agents/Tzu.agent.md [tầng cơ học]`
	if why := keep(line, kitReal, reached, namesReal, nil); why != "" {
		t.Errorf("đường dẫn mang tên đệ bị chặn oan: %s", why)
	}
}

// Món bịa: tên không có trong Kit. Dấu *thừa* nói về món CHƯA dùng nên nó không có đích,
// và đúng vì thế cửa đích không đỡ được ca này — cửa Kit mới đỡ.
func TestKeepDropsToolNotInKit(t *testing.T) {
	if why := keep("insert_edit_into_file nằm im suốt phiên", kitReal, nil, namesReal, nil); why == "" {
		t.Error("món không có trong Kit phải bị chặn")
	}
}

// Dấu *thừa* hợp lệ: gọi tên một món có thật, không trỏ đích nào. Không được chặn —
// chặn nó là bỏ mất một trong ba dấu của nghề.
func TestKeepAllowsIdleToolWithNoTarget(t *testing.T) {
	if why := keep("runSubagent nằm trong tay từ đầu phiên mà chưa lần nào được gọi", kitReal, nil, namesReal, nil); why != "" {
		t.Errorf("dấu thừa bị chặn oan: %s", why)
	}
}

// "Nói một lần rồi thôi" — dòng đã nằm trong sổ thì không nói lại. Ca thật: cùng một
// dòng về một trang trong kho ký ức nói ở lượt 7 rồi lượt 10 của cùng một phiên.
func TestKeepDropsWhatWasAlreadySaid(t *testing.T) {
	line := `read_file → C:\won\.system\memory\working\viec-dang-lam.md [kho ký ức]`
	reached := []request.ToolCall{
		{Name: "read_file", Target: `C:\won\.system\memory\working\viec-dang-lam.md`, Kind: request.TargetPath},
	}
	if why := keep(line, kitReal, reached, namesReal, nil); why != "" {
		t.Fatalf("lần đầu phải qua: %s", why)
	}
	if why := keep(line, kitReal, reached, namesReal, []string{line}); why == "" {
		t.Error("dòng đã nói phải bị chặn ở lần hai")
	}
}
