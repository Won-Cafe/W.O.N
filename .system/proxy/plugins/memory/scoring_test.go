// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package memory

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"won/proxy/core/paths"
	"won/proxy/core/plugin"
)

func newScorerMem(t *testing.T, root string) *Memory {
	t.Helper()
	p, err := New(plugin.Env{Paths: paths.Tree{Root: root}, Services: plugin.NewHub()})
	if err != nil {
		t.Fatal(err)
	}
	return p.(*Memory)
}

func scoreReq(t *testing.T, m *Memory, agent, body string) *httptest.ResponseRecorder {
	t.Helper()
	h := m.ControlHandler()
	req := httptest.NewRequest(http.MethodPut, "/update", strings.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), plugin.AgentCtxKey{}, agent))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestConfirmIncrementsS(t *testing.T) {
	root := writeStore(t, map[string]string{"moments/a.md": "# Moment — A\n\n*Chưa kiểm chứng.*\n\nBody.\n"})
	m := newScorerMem(t, root)
	rec := scoreReq(t, m, "Shu", `{"path":"moments/a.md","action":"confirm"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("code %d: %s", rec.Code, rec.Body.String())
	}
	var resp updateResp
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.S != 1 || resp.F != 0 {
		t.Errorf("confirm: s=%d f=%d, want s=1 f=0", resp.S, resp.F)
	}
	if resp.Stone != 10 {
		t.Errorf("stone=%d, want 10", resp.Stone)
	}
	// File thực sự đổi.
	b, _ := os.ReadFile(filepath.Join(root, ".system", "memory", "moments", "a.md"))
	if !strings.HasPrefix(string(b), "---\ns: 1\nf: 0\n---\n") {
		t.Errorf("frontmatter không ghi: %s", string(b))
	}
	if !strings.Contains(string(b), "Body.") {
		t.Errorf("body bị chạm: %s", string(b))
	}
}

func TestContestIncrementsF(t *testing.T) {
	root := writeStore(t, map[string]string{"moments/a.md": "---\ns: 2\nf: 0\n---\n# Moment — A\n\nBody.\n"})
	m := newScorerMem(t, root)
	rec := scoreReq(t, m, "Shu", `{"path":"moments/a.md","action":"contest"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("code %d: %s", rec.Code, rec.Body.String())
	}
	var resp updateResp
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp.S != 2 || resp.F != 1 {
		t.Errorf("contest: s=%d f=%d, want s=2 f=1", resp.S, resp.F)
	}
	if resp.Stone != 10 {
		t.Errorf("stone=%d, want 10 (2-1)*10", resp.Stone)
	}
}

func TestNoFrontmatterAdded(t *testing.T) {
	root := writeStore(t, map[string]string{"procedural/a.md": "# How — A\n\n*Way.*\n\nBody.\n"})
	m := newScorerMem(t, root)
	rec := scoreReq(t, m, "Shu", `{"path":"procedural/a.md","action":"confirm"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("code %d: %s", rec.Code, rec.Body.String())
	}
	b, _ := os.ReadFile(filepath.Join(root, ".system", "memory", "procedural", "a.md"))
	if !strings.HasPrefix(string(b), "---\ns: 1\nf: 0\n---\n") {
		t.Errorf("frontmatter phải được thêm: %s", string(b))
	}
}

func TestInvalidPathRejected(t *testing.T) {
	root := writeStore(t, map[string]string{"moments/a.md": "# A\n\nBody.\n"})
	m := newScorerMem(t, root)
	for _, p := range []string{
		`{"path":"../../../etc/passwd","action":"confirm"}`,
		`{"path":"README.md","action":"confirm"}`,
		`{"path":"moments/a.txt","action":"confirm"}`,
	} {
		rec := scoreReq(t, m, "Shu", p)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("path %s phải 400, got %d", p, rec.Code)
		}
	}
}

func TestInvalidActionRejected(t *testing.T) {
	root := writeStore(t, map[string]string{"moments/a.md": "# A\n\nBody.\n"})
	m := newScorerMem(t, root)
	rec := scoreReq(t, m, "Shu", `{"path":"moments/a.md","action":"delete"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("action lạ phải 400, got %d", rec.Code)
	}
}

func TestWrongAgentForbidden(t *testing.T) {
	root := writeStore(t, map[string]string{"moments/a.md": "# A\n\nBody.\n"})
	m := newScorerMem(t, root)
	rec := scoreReq(t, m, "Tzu", `{"path":"moments/a.md","action":"confirm"}`)
	if rec.Code != http.StatusForbidden {
		t.Errorf("agent sai phải 403, got %d", rec.Code)
	}
}

func TestBodyUntouched(t *testing.T) {
	body := "# Moment — A\n\n*Chưa kiểm chứng.*\n\nNội dung quan trọng.\n"
	root := writeStore(t, map[string]string{"moments/a.md": body})
	m := newScorerMem(t, root)
	rec := scoreReq(t, m, "Shu", `{"path":"moments/a.md","action":"confirm"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("code %d: %s", rec.Code, rec.Body.String())
	}
	b, _ := os.ReadFile(filepath.Join(root, ".system", "memory", "moments", "a.md"))
	if !strings.Contains(string(b), "Nội dung quan trọng.") {
		t.Errorf("body bị chạm: %s", string(b))
	}
}

func TestPathTraversalRejected(t *testing.T) {
	root := writeStore(t, map[string]string{"moments/a.md": "# A\n\nBody.\n"})
	m := newScorerMem(t, root)
	rec := scoreReq(t, m, "Shu", `{"path":"../../../etc/passwd","action":"confirm"}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("path traversal phải 400, got %d", rec.Code)
	}
}
