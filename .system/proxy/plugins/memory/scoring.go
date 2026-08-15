// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package memory

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"won/proxy/core/paths"
	"won/proxy/core/plugin"
)

// ControlHandler mở route scoring cho agent được phép — đệ Shu gọi để ghi sỏi.
func (m *Memory) ControlHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /update", m.handleUpdate)
	return mux
}

// updateReq — body JSON cho scoring endpoint.
type updateReq struct {
	Path   string `json:"path"`
	Action string `json:"action"` // "confirm" | "contest"
}

// updateResp — kết quả ghi sỏi.
type updateResp struct {
	S     int `json:"s"`
	F     int `json:"f"`
	Stone int `json:"stone"`
}

func (m *Memory) handleUpdate(w http.ResponseWriter, r *http.Request) {
	agent, _ := r.Context().Value(plugin.AgentCtxKey{}).(string)
	if agent != m.scorer {
		http.Error(w, "forbidden: only "+m.scorer+" may score", http.StatusForbidden)
		return
	}
	var req updateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if !validMemoryPath(req.Path) {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	switch req.Action {
	case "confirm", "contest":
	default:
		http.Error(w, "action must be confirm or contest", http.StatusBadRequest)
		return
	}
	full := filepath.Join(m.Paths.Memory(), filepath.FromSlash(req.Path))
	// filepath.Clean đã kiểm trong validMemoryPath; double-check không thoát gốc.
	if !strings.HasPrefix(filepath.Clean(full), filepath.Clean(m.Paths.Memory())+string(filepath.Separator)) {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	// Khoá quanh read-modify-write: hai confirm đồng thời cùng đọc s=0 thì cùng ghi
	// s=1, mất một lượt — TOCTOU.
	m.scoreMu.Lock()
	defer m.scoreMu.Unlock()
	b, err := os.ReadFile(full)
	if err != nil {
		http.Error(w, "read failed: "+err.Error(), http.StatusBadRequest)
		return
	}
	body, fm := splitFrontmatter(string(b))
	s, f := parseFrontmatter(fm)
	if req.Action == "confirm" {
		s++
	} else {
		f++
	}
	newContent := writeFrontmatter(s, f, body)
	if err := atomicWrite(full, []byte(newContent)); err != nil {
		http.Error(w, "write failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, updateResp{S: s, F: f, Stone: stone(s, f, m.stoneWeight)})
}

// validMemoryPath — path phải nằm trong vùng ký ức, kết thúc .md, không thoát gốc.
func validMemoryPath(p string) bool {
	if !strings.HasSuffix(p, ".md") {
		return false
	}
	clean := filepath.Clean(filepath.FromSlash(p))
	if strings.HasPrefix(clean, "..") {
		return false
	}
	parts := strings.Split(filepath.ToSlash(clean), "/")
	if len(parts) < 2 {
		return false
	}
	for _, zone := range paths.Zones {
		if parts[0] == zone {
			return true
		}
	}
	return false
}

// writeFrontmatter dựng `---\ns: S\nf: F\n---\n` + body. Pure function.
func writeFrontmatter(s, f int, body string) string {
	return fmt.Sprintf("---\ns: %d\nf: %d\n---\n%s", s, f, body)
}

// atomicWrite ghi temp rồi rename — không để file nửa vời.
func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
