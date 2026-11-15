package pagetree

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestPathAllowed(t *testing.T) {
	all := KeyPolicy{Pages: "rw", News: "rw", Media: "rw"}
	if !PathAllowed(all, "pages/about.md") {
		t.Error("пустой allowlist должен пропускать всё")
	}
	scoped := KeyPolicy{Pages: "rw", News: "none", Media: "none", Paths: []string{"pages/blog/", "news/2026"}}
	cases := []struct {
		path string
		want bool
	}{
		{"pages/blog/post.md", true},
		{"pages/blog", true},
		{"pages/blog/sub/x.md", true},
		{"news/2026/a.md", true},
		{"pages/about.md", false},
		{"pages/blogs/x.md", false},
		{"news/2025/a.md", false},
	}
	for _, c := range cases {
		if got := PathAllowed(scoped, c.path); got != c.want {
			t.Errorf("PathAllowed(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestValidatePolicyPaths(t *testing.T) {
	p := KeyPolicy{Pages: "r", News: "none", Media: "none", Paths: []string{"pages/../etc"}}
	if err := validatePolicy(p); err == nil {
		t.Error("префикс с .. должен отклоняться")
	}
	p2 := KeyPolicy{Pages: "r", News: "none", Media: "none", Paths: []string{"  "}}
	if err := validatePolicy(p2); err == nil {
		t.Error("пустой префикс должен отклоняться")
	}
	p3 := KeyPolicy{Pages: "rw", News: "r", Media: "w", Paths: []string{"pages/blog/"}}
	if err := validatePolicy(p3); err != nil {
		t.Errorf("валидная политика отклонена: %v", err)
	}
}

// mcpGuardCall прогоняет policyGuard с ключом в контексте.
func mcpGuardCall(t *testing.T, key *APIKey, tool string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	eng := &Engine{}
	h := eng.policyGuard(func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText("ok"), nil
	})
	ctx := context.WithValue(context.Background(), apiKeyContextKey, key)
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Name: tool, Arguments: args}}
	res, err := h(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestMCPPolicyGuardEnforcement(t *testing.T) {
	ro := &APIKey{ID: "k1", Policy: KeyPolicy{Pages: "r", News: "none", Media: "none"}}
	rw := &APIKey{ID: "k2", Policy: KeyPolicy{Pages: "rw", News: "rw", Media: "rw"}}
	scoped := &APIKey{ID: "k3", Policy: KeyPolicy{Pages: "rw", News: "rw", Media: "rw", Paths: []string{"pages/blog/"}}}

	// read-only ключ зовёт delete_page → отказ
	res := mcpGuardCall(t, ro, "delete_page", map[string]any{"path": "pages/about.md"})
	if !res.IsError {
		t.Error("read-only ключ не должен получить delete_page")
	} else if !strings.Contains(res.Content[0].(mcp.TextContent).Text, "недостаточно прав") {
		t.Errorf("неожиданная ошибка: %v", res.Content)
	}

	// read-only ключ читает страницу → ок
	res = mcpGuardCall(t, ro, "get_page", map[string]any{"path": "pages/about.md"})
	if res.IsError {
		t.Errorf("get_page для read-only ключа: %v", res.Content)
	}

	// rw ключ пишет → ок
	res = mcpGuardCall(t, rw, "save_page", map[string]any{"path": "pages/about.md"})
	if res.IsError {
		t.Errorf("save_page для rw ключа: %v", res.Content)
	}

	// path-скоуп: вне префикса → отказ
	res = mcpGuardCall(t, scoped, "save_page", map[string]any{"path": "pages/about.md"})
	if !res.IsError {
		t.Error("запись вне префикса должна быть запрещена")
	}

	// path-скоуп: внутри префикса → ок
	res = mcpGuardCall(t, scoped, "save_page", map[string]any{"path": "pages/blog/post.md"})
	if res.IsError {
		t.Errorf("запись внутри префикса: %v", res.Content)
	}

	// path-скоуп для media: dir+filename
	res = mcpGuardCall(t, scoped, "upload_media", map[string]any{"dir": "pages/blog/_media", "filename": "a.jpg"})
	if res.IsError {
		t.Errorf("upload в разрешённый dir: %v", res.Content)
	}
	res = mcpGuardCall(t, scoped, "upload_media", map[string]any{"dir": "media", "filename": "a.jpg"})
	if !res.IsError {
		t.Error("upload вне префикса должен быть запрещён")
	}

	// rename: оба пути проверяются
	res = mcpGuardCall(t, scoped, "rename_page", map[string]any{"old_path": "pages/blog/a.md", "new_path": "pages/secret.md"})
	if !res.IsError {
		t.Error("rename в запрещённый путь должен быть запрещён")
	}
}

// TestAgentRESTPathScope — path-скоуп через REST-контур (agentAPI + Bearer ключ).
func TestAgentRESTPathScope(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"pages", "news", "media"} {
		mkdir(t, root, d)
	}
	eng, err := New(Config{Root: root, DataDir: t.TempDir(), Renderer: &stubRenderer{}})
	if err != nil {
		t.Fatal(err)
	}
	_, raw, err := eng.keystore.GenerateKey("scoped", KeyPolicy{Pages: "rw", News: "none", Media: "none", Paths: []string{"pages/blog/"}})
	if err != nil {
		t.Fatal(err)
	}
	_, rawRO, err := eng.keystore.GenerateKey("ro", KeyPolicy{Pages: "r", News: "none", Media: "none"})
	if err != nil {
		t.Fatal(err)
	}
	api := eng.AgentAPI()

	do := func(method, url, body string, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, url, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		rec := httptest.NewRecorder()
		api.ServeHTTP(rec, req)
		return rec
	}

	putBody := func(p string) string {
		b, _ := json.Marshal(map[string]any{"path": p, "md": "# x", "json": map[string]any{}})
		return string(b)
	}
	if rec := do("PUT", "/pages/about.md", putBody("pages/about.md"), raw); rec.Code != http.StatusForbidden {
		t.Errorf("PUT вне префикса = %d, want 403: %s", rec.Code, rec.Body.String())
	}
	if rec := do("PUT", "/pages/blog/post.md", putBody("pages/blog/post.md"), raw); rec.Code != http.StatusOK {
		t.Errorf("PUT внутри префикса = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if rec := do("PUT", "/pages/about.md", putBody("pages/about.md"), rawRO); rec.Code != http.StatusForbidden {
		t.Errorf("PUT read-only ключом = %d, want 403", rec.Code)
	}
	if rec := do("GET", "/pages/pages/blog/post.md", "", raw); rec.Code != http.StatusOK {
		t.Errorf("GET внутри префикса = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if rec := do("GET", "/pages/pages/about.md", "", raw); rec.Code != http.StatusForbidden {
		t.Errorf("GET вне префикса = %d, want 403", rec.Code)
	}
}

// TestMCPToolPolicyMapComplete — каждый зарегистрированный тул покрыт mcpPolicyMap.
func TestMCPToolPolicyMapComplete(t *testing.T) {
	for name := range mcpPolicyMap {
		pc := mcpPolicyMap[name]
		if pc.resource == "" {
			t.Errorf("тул %q: пустой ресурс", name)
		}
	}
	// JSON round-trip policy с paths (контракт keysAPI)
	b, err := json.Marshal(KeyPolicy{Pages: "r", News: "none", Media: "none", Paths: []string{"pages/blog/"}})
	if err != nil {
		t.Fatal(err)
	}
	var p KeyPolicy
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatal(err)
	}
	if len(p.Paths) != 1 || p.Paths[0] != "pages/blog/" {
		t.Errorf("paths не пережили round-trip: %v", p.Paths)
	}
}
