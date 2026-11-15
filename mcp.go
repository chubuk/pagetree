package pagetree

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"path"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// policyCheck maps an MCP tool to its resource scope and write flag.
type policyCheck struct {
	resource string
	write    bool
}

// mcpPolicyMap — single source of truth for tool→scope mapping.
// Must stay 1:1 with REST routes (verified by TestMCPPolicyParity).
var mcpPolicyMap = map[string]policyCheck{
	"list_pages":   {"pages", false},
	"get_page":     {"pages", false},
	"save_page":    {"pages", true},
	"delete_page":  {"pages", true},
	"rename_page":  {"pages", true},
	"list_news":    {"news", false},
	"get_news":     {"news", false},
	"save_news":    {"news", true},
	"delete_news":  {"news", true},
	"list_media":   {"media", false},
	"upload_media": {"media", true},
	"delete_media": {"media", true},
	"get_menu":     {"pages", false},
}

// policyGuard enforces the key's policy on every MCP tool call (fail-closed).
// Also enforces path-scope (KeyPolicy.Paths) on path-like arguments.
func (e *Engine) policyGuard(h server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name := req.Params.Name
		pc, ok := mcpPolicyMap[name]
		if !ok {
			return mcp.NewToolResultError("инструмент " + name + " не имеет политики доступа"), nil
		}
		key := KeyFromContext(ctx)
		if key == nil {
			return mcp.NewToolResultError("нет аутентифицированного ключа"), nil
		}
		if !CheckPolicy(key.Policy, pc.resource, pc.write) {
			access := "чтение"
			if pc.write {
				access = "запись"
			}
			return mcp.NewToolResultError(fmt.Sprintf("недостаточно прав: инструмент %q требует %s ресурса %q", name, access, pc.resource)), nil
		}
		for _, p := range argPaths(req.GetArguments()) {
			if !PathAllowed(key.Policy, p) {
				return mcp.NewToolResultError(fmt.Sprintf("путь %q вне разрешённых префиксов ключа", p)), nil
			}
		}
		return h(ctx, req)
	}
}

// argPaths extracts path-like arguments for path-scope checks.
func argPaths(args map[string]any) []string {
	var out []string
	for _, k := range []string{"path", "old_path", "new_path"} {
		if s, ok := args[k].(string); ok && s != "" {
			out = append(out, s)
		}
	}
	dir, _ := args["dir"].(string)
	fn, _ := args["filename"].(string)
	switch {
	case dir != "" && fn != "":
		out = append(out, path.Join(dir, fn))
	case dir != "":
		out = append(out, dir)
	case fn != "":
		out = append(out, fn)
	}
	return out
}

// AgentMCPHandler returns an http.Handler serving the MCP endpoint.
// Auth (Bearer ptk_*) is applied via KeyAuthMW at the call site.
func (e *Engine) AgentMCPHandler() http.Handler {
	s := server.NewMCPServer("pagetree", "0.1.0",
		server.WithToolCapabilities(true),
		server.WithRecovery(),
	)
	e.registerMCPTools(s)
	httpSrv := server.NewStreamableHTTPServer(s)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Body cap: 32MB (20MB raw base64 ≈ 27MB encoded + JSON overhead)
		r.Body = http.MaxBytesReader(w, r.Body, 32<<20)

		// Auth check: Bearer ptk_* on every request (POST + GET/SSE)
		auth := r.Header.Get("Authorization")
		if auth == "" {
			auth = r.URL.Query().Get("access_token")
		}
		var key *APIKey
		if len(auth) > 7 && auth[:7] == "Bearer " {
			var err error
			key, err = e.keystore.Authenticate(auth[7:])
			if err != nil {
				http.Error(w, `{"jsonrpc":"2.0","error":{"code":-32001,"message":"невалидный или отозванный ключ"},"id":null}`, http.StatusUnauthorized)
				return
			}
		} else if auth != "" {
			// Try raw token (some clients pass without Bearer prefix)
			var err error
			key, err = e.keystore.Authenticate(auth)
			if err != nil {
				http.Error(w, `{"jsonrpc":"2.0","error":{"code":-32001,"message":"невалидный или отозванный ключ"},"id":null}`, http.StatusUnauthorized)
				return
			}
		} else {
			http.Error(w, `{"jsonrpc":"2.0","error":{"code":-32001,"message":"отсутствует bearer token"},"id":null}`, http.StatusUnauthorized)
			return
		}
		e.keystore.TouchKey(key.ID)
		ctx := context.WithValue(r.Context(), apiKeyContextKey, key)
		httpSrv.ServeHTTP(w, r.WithContext(ctx))
	})
}

// registerMCPTools registers all 13 MCP tools (each wrapped in policyGuard).
func (e *Engine) registerMCPTools(s *server.MCPServer) {
	add := func(tool mcp.Tool, h server.ToolHandlerFunc) {
		s.AddTool(tool, e.policyGuard(h))
	}
	// === PAGES ===
	add(mcp.NewTool("list_pages",
		mcp.WithDescription("List pages tree from content directory. Returns items with path, url, title, draft status."),
		mcp.WithString("root", mcp.Description("Root directory: 'pages' or 'news' (default: pages)")),
		mcp.WithNumber("page", mcp.Description("Page number (default 1)"), mcp.DefaultNumber(1)),
		mcp.WithNumber("per_page", mcp.Description("Items per page (default 20)"), mcp.DefaultNumber(20)),
		mcp.WithReadOnlyHintAnnotation(true),
	), e.mcpListPages())

	add(mcp.NewTool("get_page",
		mcp.WithDescription("Read a page: returns markdown content and metadata (title, menu, order, date, seo)."),
		mcp.WithString("path", mcp.Required(), mcp.Description("File path relative to content root, e.g. 'pages/about.md'")),
		mcp.WithReadOnlyHintAnnotation(true),
	), e.mcpGetPage("pages"))

	add(mcp.NewTool("save_page",
		mcp.WithDescription("Save a page atomically (md + json sidecar). Creates or overwrites."),
		mcp.WithString("path", mcp.Required(), mcp.Description("File path, e.g. 'pages/about.md'")),
		mcp.WithString("md", mcp.Required(), mcp.Description("Markdown content")),
		mcp.WithObject("json", mcp.Description("Sidecar metadata: {title, menu, order, date, modified, draft, seo:{...}}")),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
	), e.mcpSavePage())

	add(mcp.NewTool("delete_page",
		mcp.WithDescription("Delete a page triple: .md, .json, and _media/ directory."),
		mcp.WithString("path", mcp.Required(), mcp.Description("File path, e.g. 'pages/about.md'")),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
	), e.mcpDeletePage())

	add(mcp.NewTool("rename_page",
		mcp.WithDescription("Rename a page triple: .md, .json, and _media/ directory move together."),
		mcp.WithString("old_path", mcp.Required(), mcp.Description("Current path, e.g. 'pages/about.md'")),
		mcp.WithString("new_path", mcp.Required(), mcp.Description("New path, e.g. 'pages/company.md'")),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
	), e.mcpRenamePage())

	// === NEWS ===
	add(mcp.NewTool("list_news",
		mcp.WithDescription("List news entries. Returns items with path, url, title, date."),
		mcp.WithNumber("page", mcp.Description("Page number (default 1)"), mcp.DefaultNumber(1)),
		mcp.WithNumber("per_page", mcp.Description("Items per page (default 20)"), mcp.DefaultNumber(20)),
		mcp.WithReadOnlyHintAnnotation(true),
	), e.mcpListNews())

	add(mcp.NewTool("get_news",
		mcp.WithDescription("Read a news entry: returns markdown content and metadata."),
		mcp.WithString("path", mcp.Required(), mcp.Description("File path, e.g. 'news/releases/v1.2.md'")),
		mcp.WithReadOnlyHintAnnotation(true),
	), e.mcpGetPage("news"))

	add(mcp.NewTool("save_news",
		mcp.WithDescription("Save a news entry atomically (md + json sidecar). Date field is important for blog ordering."),
		mcp.WithString("path", mcp.Required(), mcp.Description("File path, e.g. 'news/releases/v1.2.md'")),
		mcp.WithString("md", mcp.Required(), mcp.Description("Markdown content")),
		mcp.WithObject("json", mcp.Description("Sidecar metadata: {title, date, modified, draft, seo:{...}}")),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
	), e.mcpSavePage())

	add(mcp.NewTool("delete_news",
		mcp.WithDescription("Delete a news entry triple: .md, .json, and _media/ directory."),
		mcp.WithString("path", mcp.Required(), mcp.Description("File path, e.g. 'news/releases/v1.2.md'")),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
	), e.mcpDeletePage())

	// === MEDIA ===
	add(mcp.NewTool("list_media",
		mcp.WithDescription("List media files in a directory. Returns name, url, size, mtime."),
		mcp.WithString("dir", mcp.Description("Directory path (default: 'media')")),
		mcp.WithReadOnlyHintAnnotation(true),
	), e.mcpListMedia())

	add(mcp.NewTool("upload_media",
		mcp.WithDescription("Upload a media file (jpg/jpeg/png/webp/gif). Returns the root-relative URL."),
		mcp.WithString("filename", mcp.Required(), mcp.Description("Filename with extension, e.g. 'photo.jpg'")),
		mcp.WithString("data", mcp.Required(), mcp.Description("File content as base64 string (max 20MB decoded)")),
		mcp.WithString("dir", mcp.Description("Target directory (default: 'media')")),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
	), e.mcpUploadMedia())

	add(mcp.NewTool("delete_media",
		mcp.WithDescription("Delete a media file."),
		mcp.WithString("path", mcp.Required(), mcp.Description("File path, e.g. 'media/photo.jpg'")),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
	), e.mcpDeleteMedia())

	// === MENU ===
	add(mcp.NewTool("get_menu",
		mcp.WithDescription("Get site menu as JSON: pages with menu=true (sorted by order) and news sections with counts."),
		mcp.WithReadOnlyHintAnnotation(true),
	), e.mcpGetMenu())
}

// --- Tool handlers ---
// Pattern: errors → mcp.NewToolResultError (return nil error), results → mcp.NewToolResultStructured.

func (e *Engine) mcpListPages() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		root := req.GetString("root", "pages")
		if root != "pages" && root != "news" {
			return mcp.NewToolResultError("root должен быть 'pages' или 'news'"), nil
		}
		page := req.GetInt("page", 1)
		perPage := req.GetInt("per_page", 20)

		tree, err := e.store.ListTree(root)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		var items []ContentEntry
		collectPages(tree, &items)
		total := len(items)
		start := (page - 1) * perPage
		if start > total {
			start = total
		}
		end := start + perPage
		if end > total {
			end = total
		}
		paged := items[start:end]
		if paged == nil {
			paged = []ContentEntry{}
		}
		result := map[string]any{"items": paged, "total": total, "page": page, "per_page": perPage}
		return mcp.NewToolResultStructured(result, prettyJSON(result)), nil
	}
}

func (e *Engine) mcpGetPage(prefix string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path, err := req.RequireString("path")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		p, err := e.store.ReadPage(path)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		result := map[string]any{
			"path":     p.Path,
			"url":      p.URL,
			"markdown": string(p.Markdown),
			"meta":     p.Meta,
		}
		return mcp.NewToolResultStructured(result, prettyJSON(result)), nil
	}
}

func (e *Engine) mcpSavePage() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path, err := req.RequireString("path")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		md, err := req.RequireString("md")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		var meta *PageMeta
		if rawJSON, ok := req.GetArguments()["json"]; ok && rawJSON != nil {
			jd, _ := json.Marshal(rawJSON)
			meta = &PageMeta{}
			if err := json.Unmarshal(jd, meta); err != nil {
				return mcp.NewToolResultError("невалидный JSON метаданных: " + err.Error()), nil
			}
		}
		if err := e.store.WritePage(path, []byte(md), meta); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		result := map[string]any{"ok": true, "path": path}
		return mcp.NewToolResultStructured(result, prettyJSON(result)), nil
	}
}

func (e *Engine) mcpDeletePage() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path, err := req.RequireString("path")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if err := e.store.DeletePage(path); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		result := map[string]any{"ok": true, "deleted": path}
		return mcp.NewToolResultStructured(result, prettyJSON(result)), nil
	}
}

func (e *Engine) mcpRenamePage() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		oldPath, err := req.RequireString("old_path")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		newPath, err := req.RequireString("new_path")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if err := e.store.RenamePage(oldPath, newPath); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		result := map[string]any{"ok": true, "old_path": oldPath, "new_path": newPath}
		return mcp.NewToolResultStructured(result, prettyJSON(result)), nil
	}
}

func (e *Engine) mcpListNews() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		page := req.GetInt("page", 1)
		perPage := req.GetInt("per_page", 20)

		tree, err := e.store.ListTree("news")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		var items []ContentEntry
		collectPages(tree, &items)
		total := len(items)
		start := (page - 1) * perPage
		if start > total {
			start = total
		}
		end := start + perPage
		if end > total {
			end = total
		}
		paged := items[start:end]
		if paged == nil {
			paged = []ContentEntry{}
		}
		result := map[string]any{"items": paged, "total": total, "page": page, "per_page": perPage}
		return mcp.NewToolResultStructured(result, prettyJSON(result)), nil
	}
}

func (e *Engine) mcpListMedia() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		dir := req.GetString("dir", "media")
		files, err := e.store.ListMedia(dir)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if files == nil {
			files = []MediaFile{}
		}
		result := map[string]any{"items": files, "total": len(files)}
		return mcp.NewToolResultStructured(result, prettyJSON(result)), nil
	}
}

func (e *Engine) mcpUploadMedia() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		filename, err := req.RequireString("filename")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		dataB64, err := req.RequireString("data")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		dir := req.GetString("dir", "media")

		// Decode + cap at 20MB
		data, err := base64.StdEncoding.DecodeString(dataB64)
		if err != nil {
			return mcp.NewToolResultError("невалидные base64-данные"), nil
		}
		if len(data) > 20<<20 {
			return mcp.NewToolResultError(fmt.Sprintf("файл слишком большой: %d байт (макс. 20МБ)", len(data))), nil
		}

		// Extension allowlist (same as REST)
		ext := ""
		for i := len(filename) - 1; i >= 0; i-- {
			if filename[i] == '.' {
				ext = filename[i:]
				break
			}
		}
		allowed := map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".gif": true}
		if !allowed[ext] {
			return mcp.NewToolResultError(fmt.Sprintf("тип файла %q не разрешён (допустимы jpg/jpeg/png/webp/gif)", ext)), nil
		}

		url, err := e.store.SaveMedia(dir, filename, data)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		result := map[string]any{"ok": true, "url": url, "name": filename, "size": len(data)}
		return mcp.NewToolResultStructured(result, prettyJSON(result)), nil
	}
}

func (e *Engine) mcpDeleteMedia() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		path, err := req.RequireString("path")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if err := e.store.DeleteMedia(path); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		result := map[string]any{"ok": true, "deleted": path}
		return mcp.NewToolResultStructured(result, prettyJSON(result)), nil
	}
}

func (e *Engine) mcpGetMenu() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		menu := e.buildMenu()
		return mcp.NewToolResultStructured(menu, prettyJSON(menu)), nil
	}
}

// prettyJSON returns indented JSON for the text fallback.
func prettyJSON(v any) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}
