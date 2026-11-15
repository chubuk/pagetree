package pagetree

import (
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// --- Public handlers ---

// pageHandler serves GET /pages/{path...}
func (e *Engine) pageHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "метод не поддерживается", http.StatusMethodNotAllowed)
			return
		}
		rel := e.stripBase(r.URL.Path, "/pages")
		if rel == "" {
			e.servePageListing(w, r, "pages")
			return
		}
		// Check if it's a media file request (about_media/photo.jpg)
		if isMediaPath(rel) {
			e.serveFile(w, r, rel)
			return
		}
		pagePath, err := e.store.resolveURL("/pages/" + rel)
		if err != nil {
			// Нет страницы — но каталог с страницами существует → листинг секции
			if tree, _ := e.store.ListTree("pages/" + rel); len(tree) > 0 {
				e.servePageListing(w, r, "pages/"+rel)
				return
			}
			e.serve404(w, r)
			return
		}
		p, err := e.store.ReadPage(pagePath)
		if err != nil {
			e.serve404(w, r)
			return
		}
		if p.Meta.Draft {
			e.serve404(w, r)
			return
		}
		html, err := e.renderPage(p)
		if err != nil {
			e.log.Error("render page", "path", p.Path, "error", err)
			http.Error(w, "внутренняя ошибка", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		cachePublic(w)
		_, _ = w.Write(html)
	})
}

// newsHandler serves GET /news/{...} — blog, sections, news pages.
func (e *Engine) newsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "метод не поддерживается", http.StatusMethodNotAllowed)
			return
		}
		rel := e.stripBase(r.URL.Path, "/news")
		// rel = "", "releases/", "releases/v1.2"
		rel = strings.Trim(rel, "/")

		if rel == "" {
			e.serveBlogListing(w, r)
			return
		}

		parts := strings.SplitN(rel, "/", 2)
		section := parts[0]

		// Check media path
		if isMediaPath(rel) {
			e.serveFile(w, r, rel)
			return
		}

		if len(parts) == 1 {
			// Section listing or page at section level
			// Try as page first
			pagePath, err := e.store.resolveURL("/news/" + rel)
			if err == nil {
				p, _ := e.store.ReadPage(pagePath)
				if p != nil && !p.Meta.Draft {
					html, err := e.renderPage(p)
					if err == nil {
						w.Header().Set("Content-Type", "text/html; charset=utf-8")
						cachePublic(w)
						_, _ = w.Write(html)
						return
					}
				}
			}
			e.serveSectionListing(w, r, section)
			return
		}

		// section/slug — news page
		pagePath, err := e.store.resolveURL("/news/" + rel)
		if err != nil {
			e.serve404(w, r)
			return
		}
		p, err := e.store.ReadPage(pagePath)
		if err != nil {
			e.serve404(w, r)
			return
		}
		if p.Meta.Draft {
			e.serve404(w, r)
			return
		}
		html, err := e.renderPage(p)
		if err != nil {
			http.Error(w, "внутренняя ошибка", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		cachePublic(w)
		_, _ = w.Write(html)
	})
}

// cachePublic — публичный контент (фрон) кэшируется коротко; админ-API всегда no-store.
func cachePublic(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "public, max-age=60")
}

// menuHandler serves GET /api/content/menu — JSON menu.
func (e *Engine) menuHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "метод не поддерживается", http.StatusMethodNotAllowed)
			return
		}
		menu := e.buildMenu()
		w.Header().Set("Content-Type", "application/json")
		cachePublic(w)
		_ = json.NewEncoder(w).Encode(menu)
	})
}

// mediaHandler serves GET /media/{path...}
func (e *Engine) mediaHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "метод не поддерживается", http.StatusMethodNotAllowed)
			return
		}
		rel := e.stripBase(r.URL.Path, "/media")
		if rel == "" {
			http.NotFound(w, r)
			return
		}
		e.serveFile(w, r, "media/"+rel)
	})
}

// --- Helpers ---

// stripBase removes mount prefix from URL path.
func (e *Engine) stripBase(urlPath, prefix string) string {
	p := strings.TrimPrefix(urlPath, prefix)
	return strings.TrimPrefix(p, "/")
}

// isMediaPath checks if the path points to a media file (e.g., about_media/photo.jpg).
func isMediaPath(rel string) bool {
	return strings.Contains(rel, "_media/")
}

// serveFile serves a static file from content root.
func (e *Engine) serveFile(w http.ResponseWriter, r *http.Request, rel string) {
	clean, err := e.store.guard(rel)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	abs := filepath.Join(e.store.root, filepath.FromSlash(clean))
	info, err := os.Stat(abs)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	// Content type
	ct := mime.TypeByExtension(filepath.Ext(abs))
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	http.ServeFile(w, r, abs)
}

// servePageListing renders a paginated listing of pages.
func (e *Engine) servePageListing(w http.ResponseWriter, r *http.Request, root string) {
	page := queryPage(r)
	perPage := 10
	tree, _ := e.store.ListTree(root)
	var items []ContentEntry
	collectPages(tree, &items)
	sort.Slice(items, func(i, j int) bool {
		return items[i].Title < items[j].Title
	})
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

	sec, _ := e.store.ReadSection(root)
	title := sec.Title
	if title == "" {
		title = "Страницы"
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<!DOCTYPE html><html><head><meta charset="utf-8"><title>%s</title>
<style>body{font-family:system-ui,sans-serif;max-width:800px;margin:0 auto;padding:1rem}
.pagination a{margin:0 .5rem;text-decoration:none;color:#0066cc}</style></head><body>`, title)
	fmt.Fprintf(&b, `<h1>%s</h1><ul>`, title)
	for _, item := range paged {
		fmt.Fprintf(&b, `<li><a href="%s">%s</a></li>`, item.URL, item.Title)
	}
	b.WriteString(`</ul>`)
	writePagination(&b, page, total, perPage)
	b.WriteString(`</body></html>`)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	cachePublic(w)
	_, _ = w.Write([]byte(b.String()))
}

// serveBlogListing renders the blog feed (all news, date DESC).
func (e *Engine) serveBlogListing(w http.ResponseWriter, r *http.Request) {
	page := queryPage(r)
	perPage := 10
	tree, _ := e.store.ListTree("news")
	var items []ContentEntry
	collectPages(tree, &items)
	// Sort by date DESC (from meta — need to load)
	type newsItem struct {
		ContentEntry
		Date string
	}
	var news []newsItem
	for _, item := range items {
		p, _ := e.store.ReadPage(item.Path)
		if p == nil || p.Meta.Draft {
			continue
		}
		news = append(news, newsItem{ContentEntry: item, Date: p.Meta.Date})
	}
	sort.Slice(news, func(i, j int) bool {
		return news[i].Date > news[j].Date
	})
	total := len(news)
	start := (page - 1) * perPage
	if start > total {
		start = total
	}
	end := start + perPage
	if end > total {
		end = total
	}

	var b strings.Builder
	b.WriteString(`<!DOCTYPE html><html><head><meta charset="utf-8"><title>Новости</title>
<style>body{font-family:system-ui,sans-serif;max-width:800px;margin:0 auto;padding:1rem}
.pagination a{margin:0 .5rem;text-decoration:none;color:#0066cc}
article{margin-bottom:1.5rem;border-bottom:1px solid #eee;padding-bottom:1rem}</style></head><body>`)
	b.WriteString(`<h1>Новости</h1>`)
	for _, n := range news[start:end] {
		fmt.Fprintf(&b, `<article><h2><a href="%s">%s</a></h2>`, n.URL, n.Title)
		if n.Date != "" {
			fmt.Fprintf(&b, `<time>%s</time>`, n.Date)
		}
		b.WriteString(`</article>`)
	}
	writePagination(&b, page, total, perPage)
	b.WriteString(`</body></html>`)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	cachePublic(w)
	_, _ = w.Write([]byte(b.String()))
}

// serveSectionListing renders a section listing with pagination.
func (e *Engine) serveSectionListing(w http.ResponseWriter, r *http.Request, section string) {
	page := queryPage(r)
	perPage := 10
	tree, _ := e.store.ListTree("news/" + section)
	var items []ContentEntry
	collectPages(tree, &items)
	// Load dates and sort DESC
	type newsItem struct {
		ContentEntry
		Date string
	}
	var news []newsItem
	for _, item := range items {
		p, _ := e.store.ReadPage(item.Path)
		if p == nil || p.Meta.Draft {
			continue
		}
		news = append(news, newsItem{ContentEntry: item, Date: p.Meta.Date})
	}
	sort.Slice(news, func(i, j int) bool {
		return news[i].Date > news[j].Date
	})
	total := len(news)
	start := (page - 1) * perPage
	if start > total {
		start = total
	}
	end := start + perPage
	if end > total {
		end = total
	}

	sec, _ := e.store.ReadSection("news/" + section)
	title := sec.Title
	if title == "" {
		title = section
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<!DOCTYPE html><html><head><meta charset="utf-8"><title>%s</title>
<style>body{font-family:system-ui,sans-serif;max-width:800px;margin:0 auto;padding:1rem}
.pagination a{margin:0 .5rem;text-decoration:none;color:#0066cc}</style></head><body>`, title)
	fmt.Fprintf(&b, `<h1>%s</h1><ul>`, title)
	for _, n := range news[start:end] {
		fmt.Fprintf(&b, `<li><a href="%s">%s</a> <time>%s</time></li>`, n.URL, n.Title, n.Date)
	}
	b.WriteString(`</ul>`)
	writePagination(&b, page, total, perPage)
	b.WriteString(`</body></html>`)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	cachePublic(w)
	_, _ = w.Write([]byte(b.String()))
}

// collectPages flattens ContentEntry tree to leaf pages.
func collectPages(entries []ContentEntry, out *[]ContentEntry) {
	for _, e := range entries {
		if e.Type == "page" || e.Type == "news" {
			*out = append(*out, e)
		}
		collectPages(e.Children, out)
	}
}

// queryPage extracts ?page=N.
func queryPage(r *http.Request) int {
	p, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || p < 1 {
		return 1
	}
	return p
}

// writePagination renders pagination links.
func writePagination(b *strings.Builder, page, total, perPage int) {
	pages := (total + perPage - 1) / perPage
	if pages <= 1 {
		return
	}
	b.WriteString(`<nav class="pagination">`)
	if page > 1 {
		fmt.Fprintf(b, `<a href="?page=%d">&laquo; Назад</a>`, page-1)
	}
	for i := 1; i <= pages; i++ {
		if i == page {
			fmt.Fprintf(b, `<strong>%d</strong>`, i)
		} else {
			fmt.Fprintf(b, `<a href="?page=%d">%d</a>`, i, i)
		}
	}
	if page < pages {
		fmt.Fprintf(b, `<a href="?page=%d">Вперёд &raquo;</a>`, page+1)
	}
	b.WriteString(`</nav>`)
}

// menuData — JSON structure for the menu endpoint.
type menuData struct {
	Pages []menuItem `json:"pages"`
	News  []menuItem `json:"news_sections"`
}

type menuItem struct {
	Title string `json:"title"`
	URL   string `json:"url"`
	Order int    `json:"order"`
	Count int    `json:"count,omitempty"`
}

// buildMenu collects menu items from content.
func (e *Engine) buildMenu() menuData {
	var menu menuData

	// Pages with menu:true
	tree, _ := e.store.ListTree("pages")
	var pages []menuItem
	collectMenuItems(tree, e.store, &pages)
	sort.Slice(pages, func(i, j int) bool {
		if pages[i].Order != pages[j].Order {
			return pages[i].Order < pages[j].Order
		}
		return pages[i].Title < pages[j].Title
	})
	menu.Pages = pages

	// News sections
	newsTree, _ := e.store.ListTree("news")
	for _, entry := range newsTree {
		if entry.Type != "dir" {
			continue
		}
		sec, _ := e.store.ReadSection(entry.Path)
		if !sec.Menu {
			continue
		}
		var items []ContentEntry
		collectPages(entry.Children, &items)
		title := sec.Title
		if title == "" {
			title = entry.Title
		}
		menu.News = append(menu.News, menuItem{
			Title: title,
			URL:   entry.URL,
			Order: sec.Order,
			Count: len(items),
		})
	}
	sort.Slice(menu.News, func(i, j int) bool {
		if menu.News[i].Order != menu.News[j].Order {
			return menu.News[i].Order < menu.News[j].Order
		}
		return menu.News[i].Title < menu.News[j].Title
	})
	return menu
}

// collectMenuItems picks pages with menu:true from tree.
func collectMenuItems(entries []ContentEntry, s *Store, out *[]menuItem) {
	for _, e := range entries {
		if e.Type == "page" || e.Type == "news" {
			p, err := s.ReadPage(e.Path)
			if err == nil && p.Meta.Menu {
				*out = append(*out, menuItem{
					Title: p.Meta.Title,
					URL:   p.URL,
					Order: p.Meta.Order,
				})
			}
		}
		collectMenuItems(e.Children, s, out)
	}
}

// serve404 renders a simple 404 page.
func (e *Engine) serve404(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte(`<!DOCTYPE html><html><head><meta charset="utf-8"><title>404</title></head>
<body><h1>404 — Страница не найдена</h1><p><a href="/">На главную</a></p></body></html>`))
}
