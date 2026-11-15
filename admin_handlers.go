package pagetree

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// --- Admin: Content CRUD (consumer wraps with auth) ---

func (e *Engine) contentAPI() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /list", e.adminList)
	mux.HandleFunc("GET /file", e.adminReadFile)
	mux.HandleFunc("PUT /page", e.adminWritePage)
	mux.HandleFunc("PUT /section", e.adminWriteSection)
	mux.HandleFunc("POST /upload", e.adminUpload)
	mux.HandleFunc("POST /rename", e.adminRenamePage)
	mux.HandleFunc("DELETE /page", e.adminDeletePage)
	mux.HandleFunc("GET /media", e.adminListMedia)
	mux.HandleFunc("DELETE /media", e.adminDeleteMedia)
	mux.HandleFunc("POST /media/rename", e.adminRenameMedia)
	return mux
}

// adminList returns content tree.
// GET /api/admin/content/list?root=pages|news
func (e *Engine) adminList(w http.ResponseWriter, r *http.Request) {
	root := r.URL.Query().Get("root")
	if root != "pages" && root != "news" {
		jsonError(w, "root должен быть pages или news", http.StatusBadRequest)
		return
	}
	tree, err := e.store.ListTree(root)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, tree)
}

// adminReadFile reads a .md or .json file.
// GET /api/admin/content/file?path=pages/about.md
func (e *Engine) adminReadFile(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		jsonError(w, "path обязателен", http.StatusBadRequest)
		return
	}
	clean, err := e.store.guard(path)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	data, err := os.ReadFile(filepath.Join(e.store.root, filepath.FromSlash(clean)))
	if err != nil {
		jsonError(w, "файл не найден", http.StatusNotFound)
		return
	}
	// Return as JSON with content
	resp := map[string]any{
		"path":    clean,
		"content": string(data),
	}
	if strings.HasSuffix(clean, ".json") {
		resp["type"] = "json"
	} else {
		resp["type"] = "md"
	}
	jsonOK(w, resp)
}

// adminWritePage atomically writes .md + .json.
// PUT /api/admin/content/page  body: {path, md, json}
func (e *Engine) adminWritePage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string          `json:"path"`
		MD   string          `json:"md"`
		JSON json.RawMessage `json:"json"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "невалидный JSON-запрос", http.StatusBadRequest)
		return
	}
	if req.Path == "" {
		jsonError(w, "path обязателен", http.StatusBadRequest)
		return
	}
	var meta *PageMeta
	if len(req.JSON) > 0 {
		meta = &PageMeta{}
		if err := json.Unmarshal(req.JSON, meta); err != nil {
			jsonError(w, "невалидный JSON метаданных: "+err.Error(), http.StatusBadRequest)
			return
		}
	}
	if err := e.store.WritePage(req.Path, []byte(req.MD), meta); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]any{"ok": true, "path": req.Path})
}

// adminWriteSection saves section.json for a directory.
// PUT /api/admin/content/section  {"path":"pages/docs","json":{"title":"...","order":1,"menu":true}}
func (e *Engine) adminWriteSection(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string          `json:"path"`
		JSON json.RawMessage `json:"json"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "невалидный JSON-запрос", http.StatusBadRequest)
		return
	}
	if req.Path == "" {
		jsonError(w, "path обязателен", http.StatusBadRequest)
		return
	}
	sec := &SectionMeta{}
	if len(req.JSON) > 0 {
		if err := json.Unmarshal(req.JSON, sec); err != nil {
			jsonError(w, "невалидный JSON секции: "+err.Error(), http.StatusBadRequest)
			return
		}
	}
	if err := e.store.WriteSection(req.Path, sec); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	jsonOK(w, map[string]any{"ok": true, "path": req.Path})
}

// adminDeletePage deletes md+json+_media (triple).
// DELETE /api/admin/content/page?path=pages/about.md
func (e *Engine) adminDeletePage(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		jsonError(w, "path обязателен", http.StatusBadRequest)
		return
	}
	if err := e.store.DeletePage(path); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]any{"ok": true})
}

// adminRenamePage renames md+json+_media (triple).
// POST /api/admin/content/rename  body: {old_path, new_path}
func (e *Engine) adminRenamePage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OldPath string `json:"old_path"`
		NewPath string `json:"new_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "невалидный JSON-запрос", http.StatusBadRequest)
		return
	}
	if err := e.store.RenamePage(req.OldPath, req.NewPath); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]any{"ok": true})
}

// adminUpload handles multipart file upload.
// POST /api/admin/content/upload  multipart: file, dir
func (e *Engine) adminUpload(w http.ResponseWriter, r *http.Request) {
	const maxSize = 20 << 20 // 20 MB
	r.Body = http.MaxBytesReader(w, r.Body, maxSize)
	if err := r.ParseMultipartForm(maxSize); err != nil {
		jsonError(w, "файл слишком большой или невалидная форма", http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		jsonError(w, "поле file обязательно", http.StatusBadRequest)
		return
	}
	defer func() { _ = file.Close() }()

	dir := r.FormValue("dir")
	if dir == "" {
		dir = "media"
	}
	// Check extension allowlist
	ext := strings.ToLower(filepath.Ext(header.Filename))
	allowed := map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".gif": true}
	if !allowed[ext] {
		jsonError(w, fmt.Sprintf("тип файла %q не разрешён (допустимы jpg/jpeg/png/webp/gif)", ext), http.StatusBadRequest)
		return
	}
	data, err := io.ReadAll(file)
	if err != nil {
		jsonError(w, "ошибка чтения файла: "+err.Error(), http.StatusInternalServerError)
		return
	}
	url, err := e.store.SaveMedia(dir, header.Filename, data)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]any{"url": url, "name": filepath.Base(url)})
}

// adminListMedia lists media files.
// GET /api/admin/content/media?path=media
func (e *Engine) adminListMedia(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		path = "media"
	}
	files, err := e.store.ListMedia(path)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if files == nil {
		files = []MediaFile{}
	}
	jsonOK(w, files)
}

// adminDeleteMedia deletes a media file.
// DELETE /api/admin/content/media?path=media/photo.jpg
func (e *Engine) adminDeleteMedia(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		jsonError(w, "path обязателен", http.StatusBadRequest)
		return
	}
	if err := e.store.DeleteMedia(path); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]any{"ok": true})
}

// adminRenameMedia renames a media file.
// POST /api/admin/content/media/rename  body: {old_path, new_path}
func (e *Engine) adminRenameMedia(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OldPath string `json:"old_path"`
		NewPath string `json:"new_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "невалидный JSON-запрос", http.StatusBadRequest)
		return
	}
	if err := e.store.RenameMedia(req.OldPath, req.NewPath); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]any{"ok": true})
}

// --- Admin: Keys API ---

func (e *Engine) keysAPI() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", e.adminListKeys)
	mux.HandleFunc("POST /", e.adminCreateKey)
	mux.HandleFunc("GET /{id}", e.adminGetKey)
	mux.HandleFunc("PUT /{id}", e.adminUpdateKey)
	mux.HandleFunc("DELETE /{id}", e.adminDeleteKey)
	return mux
}

// adminListKeys returns all keys.
func (e *Engine) adminListKeys(w http.ResponseWriter, r *http.Request) {
	if e.keystore == nil {
		jsonError(w, "keystore не настроен", http.StatusServiceUnavailable)
		return
	}
	keys := e.keystore.ListKeys()
	jsonOK(w, keys)
}

// adminCreateKey creates a new key.
func (e *Engine) adminCreateKey(w http.ResponseWriter, r *http.Request) {
	if e.keystore == nil {
		jsonError(w, "keystore не настроен", http.StatusServiceUnavailable)
		return
	}
	var req struct {
		Name   string    `json:"name"`
		Policy KeyPolicy `json:"policy"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "невалидный JSON-запрос", http.StatusBadRequest)
		return
	}
	key, raw, err := e.keystore.GenerateKey(req.Name, req.Policy)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	jsonOK(w, map[string]any{
		"key":     key,
		"raw_key": raw, // shown ONCE
	})
}

// adminGetKey returns a key by ID.
func (e *Engine) adminGetKey(w http.ResponseWriter, r *http.Request) {
	if e.keystore == nil {
		jsonError(w, "keystore не настроен", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	key, err := e.keystore.GetKey(id)
	if err != nil {
		jsonError(w, err.Error(), http.StatusNotFound)
		return
	}
	jsonOK(w, key)
}

// adminUpdateKey updates name/policy.
func (e *Engine) adminUpdateKey(w http.ResponseWriter, r *http.Request) {
	if e.keystore == nil {
		jsonError(w, "keystore не настроен", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	var req struct {
		Name   string     `json:"name"`
		Policy *KeyPolicy `json:"policy"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "невалидный JSON-запрос", http.StatusBadRequest)
		return
	}
	if err := e.keystore.UpdateKey(id, req.Name, req.Policy); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	jsonOK(w, map[string]any{"ok": true})
}

// adminDeleteKey revokes and deletes a key.
func (e *Engine) adminDeleteKey(w http.ResponseWriter, r *http.Request) {
	if e.keystore == nil {
		jsonError(w, "keystore не настроен", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	if err := e.keystore.DeleteKey(id); err != nil {
		jsonError(w, err.Error(), http.StatusNotFound)
		return
	}
	jsonOK(w, map[string]any{"ok": true})
}

// --- Agent API (under KeyAuth) ---

func (e *Engine) agentAPI() http.Handler {
	mux := http.NewServeMux()
	// Pages (read)
	mux.Handle("GET /pages", e.KeyAuthMW("pages", false)(http.HandlerFunc(e.adminList)))
	mux.Handle("GET /pages/{path...}", e.KeyAuthMW("pages", false)(http.HandlerFunc(e.adminReadFile)))
	// Pages (write)
	mux.Handle("PUT /pages/{path...}", e.KeyAuthMW("pages", true)(http.HandlerFunc(e.adminWritePage)))
	mux.Handle("DELETE /pages/{path...}", e.KeyAuthMW("pages", true)(http.HandlerFunc(e.adminDeletePage)))
	// News
	mux.Handle("GET /news", e.KeyAuthMW("news", false)(http.HandlerFunc(e.adminList)))
	mux.Handle("GET /news/{path...}", e.KeyAuthMW("news", false)(http.HandlerFunc(e.adminReadFile)))
	mux.Handle("PUT /news/{path...}", e.KeyAuthMW("news", true)(http.HandlerFunc(e.adminWritePage)))
	mux.Handle("DELETE /news/{path...}", e.KeyAuthMW("news", true)(http.HandlerFunc(e.adminDeletePage)))
	// Upload
	mux.Handle("POST /upload", e.KeyAuthMW("media", true)(http.HandlerFunc(e.adminUpload)))
	// Menu (read)
	mux.Handle("GET /menu", e.KeyAuthMW("pages", false)(http.HandlerFunc(e.menuHandler().ServeHTTP)))
	return mux
}

// --- Admin UI (embedded assets) ---

func (e *Engine) adminUIHandler() http.Handler {
	sub := http.StripPrefix("/content/admin", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" || path == "" || path == "/index.html" {
			data, _ := assetsFS.ReadFile("assets/admin.html")
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			// Shell меняется при деплое — иначе браузер показывает новый HTML со старым JS
			w.Header().Set("Cache-Control", "no-cache")
			_, _ = w.Write(data)
			return
		}
		rel := "assets" + path
		data, err := assetsFS.ReadFile(rel)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		ext := filepath.Ext(path)
		switch ext {
		case ".css":
			w.Header().Set("Content-Type", "text/css")
		case ".js":
			w.Header().Set("Content-Type", "application/javascript")
		case ".html":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
		default:
			w.Header().Set("Content-Type", "application/octet-stream")
		}
		// Приложение меняется при деплое; вендор (vditor) кэшируем
		if path == "/contentadmin.js" {
			w.Header().Set("Cache-Control", "no-cache")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=3600")
		}
		_, _ = w.Write(data)
	}))
	return sub
}

// --- JSON helpers ---

// jsonOK — ответ админ-API: данные не кэшируются (админ должен видеть свежее без перезагрузки).
func jsonOK(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(data)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
