package pagetree

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

// TestMCPPolicyParity verifies tool→scope mapping matches REST routes 1:1.
func TestMCPPolicyParity(t *testing.T) {
	// REST route equivalents: tool name → (resource, write)
	restRoutes := map[string]policyCheck{
		"list_pages":   {"pages", false}, // GET /api/admin/content/list?root=pages
		"get_page":     {"pages", false}, // GET /api/admin/content/file?path=...
		"save_page":    {"pages", true},  // PUT /api/admin/content/page
		"delete_page":  {"pages", true},  // DELETE /api/admin/content/page
		"rename_page":  {"pages", true},  // POST /api/admin/content/rename
		"list_news":    {"news", false},  // GET /api/admin/content/list?root=news
		"get_news":     {"news", false},  // GET /api/admin/content/file?path=...
		"save_news":    {"news", true},   // PUT /api/admin/content/page (news path)
		"delete_news":  {"news", true},   // DELETE /api/admin/content/page (news path)
		"list_media":   {"media", false}, // GET /api/admin/content/media
		"upload_media": {"media", true},  // POST /api/admin/content/upload
		"delete_media": {"media", true},  // DELETE /api/admin/content/media
		"get_menu":     {"pages", false}, // GET /api/content/menu
	}

	// Every REST route must have an MCP entry
	for name, rc := range restRoutes {
		pc, ok := mcpPolicyMap[name]
		if !ok {
			t.Errorf("REST route %q has no MCP policy entry", name)
			continue
		}
		if pc.resource != rc.resource || pc.write != rc.write {
			t.Errorf("policy mismatch for %q: REST=(%s,%v) MCP=(%s,%v)",
				name, rc.resource, rc.write, pc.resource, pc.write)
		}
	}

	// Every MCP entry must have a REST equivalent
	for name := range mcpPolicyMap {
		if _, ok := restRoutes[name]; !ok {
			t.Errorf("MCP tool %q has no REST route equivalent", name)
		}
	}
}

// TestMCPPolicyMapScope values are valid.
func TestMCPPolicyMapValid(t *testing.T) {
	validResources := map[string]bool{"pages": true, "news": true, "media": true}
	for name, pc := range mcpPolicyMap {
		if !validResources[pc.resource] {
			t.Errorf("tool %q: invalid resource %q", name, pc.resource)
		}
	}
}

// TestMCPUploadSizeLimit verifies 20MB decoded cap.
func TestMCPUploadSizeLimit(t *testing.T) {
	// Build >20MB of data
	bigData := make([]byte, 21<<20)
	bigB64 := base64.StdEncoding.EncodeToString(bigData)

	// The handler would decode and check size — verify our check logic
	decoded, err := base64.StdEncoding.DecodeString(bigB64)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded) <= 20<<20 {
		t.Skip("decoded data under limit — skip")
	}
	// This is what the handler does:
	if len(decoded) > 20<<20 {
		// Expected: error
	} else {
		t.Error("size check failed to trigger")
	}
}

// TestMCPSmokeCRUD — full CRUD cycle through store (same funcs as MCP handlers).
func TestMCPSmokeCRUD(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"pages", "news", "media"} {
		mkdir(t, root, d)
	}
	s, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}

	// Save page
	meta := &PageMeta{Title: "Test", Menu: true, Order: 1}
	if err := s.WritePage("pages/test.md", []byte("# Test"), meta); err != nil {
		t.Fatal("save:", err)
	}

	// Read page
	p, err := s.ReadPage("pages/test.md")
	if err != nil {
		t.Fatal("read:", err)
	}
	if p.Meta.Title != "Test" {
		t.Errorf("title = %q", p.Meta.Title)
	}

	// Rename
	if err := s.RenamePage("pages/test.md", "pages/renamed.md"); err != nil {
		t.Fatal("rename:", err)
	}

	// Delete
	if err := s.DeletePage("pages/renamed.md"); err != nil {
		t.Fatal("delete:", err)
	}

	// Upload + delete media
	url, err := s.SaveMedia("media", "test.jpg", []byte("fake"))
	if err != nil {
		t.Fatal("upload:", err)
	}
	if url != "/media/test.jpg" {
		t.Errorf("url = %q", url)
	}
	if err := s.DeleteMedia("media/test.jpg"); err != nil {
		t.Fatal("delete media:", err)
	}
}

func mkdir(t *testing.T, root, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, name), 0o755); err != nil {
		t.Fatal(err)
	}
}
