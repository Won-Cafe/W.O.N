// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Locked — khoá KHÔNG ghi được qua API, và lý do từng khoá. `listen`/`control` là
// bootstrap — đổi khi tiến trình đang chạy chỉ làm mất chính cái cửa vừa gọi vào.
var Locked = map[string]string{
	"listen":  "địa chỉ data-plane cố định lúc bind — chỉ cờ CLI hoặc won.conf",
	"control": "địa chỉ control cố định lúc bind — chỉ cờ CLI hoặc won.conf",
}

// WriteResult — cái đã đổi và cái bị chặn, để lời đáp nói đúng việc đã làm.
type WriteResult struct {
	Path    string            `json:"path"`
	Changed map[string]string `json:"changed"`
	Added   []string          `json:"added,omitempty"`
	Refused map[string]string `json:"refused,omitempty"`
}

// UpdateFile ghi các khoá vào won.conf TẠI CHỖ: dòng đã có thì thay phần giá trị và giữ
// nguyên comment cuối dòng, dòng chưa có thì thêm vào cuối file. Không dựng lại file từ
// Config — làm thế là xoá sạch comment người viết, và won.conf phần lớn là comment.
//
// Khoá trong Locked bị từ chối, không ghi âm thầm. Khoá lạ cũng bị từ chối: một khoá sai
// chính tả nằm trong file là một lỗi khởi động ở lần chạy sau.
func UpdateFile(path string, kv map[string]string) (*WriteResult, error) {
	if path == "" {
		return nil, fmt.Errorf("no won.conf path")
	}
	res := &WriteResult{Path: path, Changed: map[string]string{}}
	want := map[string]string{}
	for k, v := range kv {
		k = strings.TrimSpace(k)
		if reason, locked := Locked[k]; locked {
			if res.Refused == nil {
				res.Refused = map[string]string{}
			}
			res.Refused[k] = reason
			continue
		}
		if !knownCoreKey(k) && !strings.Contains(k, ".") {
			if res.Refused == nil {
				res.Refused = map[string]string{}
			}
			res.Refused[k] = "khoá lạ — won.conf chỉ nhận khoá đã biết (config.knownCoreKey)"
			continue
		}
		want[k] = strings.TrimSpace(v)
	}
	if len(want) == 0 {
		return res, nil
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	nl := "\n"
	if strings.Contains(string(raw), "\r\n") {
		nl = "\r\n"
	}
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")

	for i, line := range lines {
		key, ok := settingKey(line)
		if !ok {
			continue
		}
		v, wanted := want[key]
		if !wanted {
			continue
		}
		lines[i] = replaceValue(line, v)
		res.Changed[key] = v
		delete(want, key)
	}

	// Khoá chưa có dòng nào: thêm ở cuối, mỗi khoá một dòng. Không cố đoán nên nhét vào
	// mục nào — đoán sai thì dòng nằm dưới một comment nói về chuyện khác.
	if len(want) > 0 {
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
			lines = append(lines, "")
		}
		lines = append(lines, "# đặt qua Control API")
		for k, v := range want {
			lines = append(lines, k+" = "+v)
			res.Changed[k] = v
			res.Added = append(res.Added, k)
		}
	}

	out := strings.Join(lines, nl)
	tmp := filepath.Join(filepath.Dir(path), ".won.conf.tmp")
	if err := os.WriteFile(tmp, []byte(out), 0o644); err != nil {
		return nil, err
	}
	// Ghi tạm rồi rename: cắt điện giữa lúc ghi cũng không để lại một won.conf nửa vời,
	// và một won.conf nửa vời là một lỗi khởi động.
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return nil, err
	}
	return res, nil
}

// settingKey — dòng này là một `key = value` không bị comment che không, và khoá là gì.
func settingKey(line string) (string, bool) {
	t := strings.TrimSpace(line)
	if t == "" || t[0] == '#' {
		return "", false
	}
	eq := strings.IndexByte(t, '=')
	if eq < 0 {
		return "", false
	}
	// `#` trước dấu `=` nghĩa là cả dòng đã bị comment che.
	if h := strings.IndexByte(t, '#'); h >= 0 && h < eq {
		return "", false
	}
	return strings.TrimSpace(t[:eq]), true
}

// replaceValue thay phần giá trị, giữ thụt lề đầu dòng và comment cuối dòng — comment
// cuối dòng là lời người viết giải nghĩa cái núm, đổi giá trị không phải xoá nó.
func replaceValue(line, val string) string {
	eq := strings.IndexByte(line, '=')
	if eq < 0 {
		return line
	}
	head := line[:eq+1]
	rest := line[eq+1:]
	trail := ""
	if h := strings.IndexByte(rest, '#'); h >= 0 {
		trail = rest[h:]
		gap := len(rest[:h]) - len(strings.TrimRight(rest[:h], " \t"))
		if gap < 1 {
			gap = 1
		}
		trail = strings.Repeat(" ", gap) + trail
	}
	return head + " " + val + trail
}
