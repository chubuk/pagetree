package pagetree

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"net/url"
	"strings"
)

// PageData — data passed to the HTML template.
type PageData struct {
	Title         string
	MetaDesc      string
	Robots        string
	Canonical     string
	OgTitle       string
	OgDescription string
	OgImage       string // absolute
	OgType        string
	OgURL         string // absolute
	TwitterCard   string
	JSONLD        template.JS
	AlternateMD   string // URL to raw .md
	Breadcrumbs   []Breadcrumb
	Content       template.HTML // sanitized HTML body
	SiteName      string
}

// Breadcrumb — one item in the breadcrumb trail.
type Breadcrumb struct {
	Name string
	URL  string // empty for current page
}

const pageTemplate = `<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
{{if .MetaDesc}}<meta name="description" content="{{.MetaDesc}}">{{end}}
{{if .Robots}}<meta name="robots" content="{{.Robots}}">{{end}}
{{if .Canonical}}<link rel="canonical" href="{{.Canonical}}">{{end}}
{{if .AlternateMD}}<link rel="alternate" type="text/markdown" href="{{.AlternateMD}}">{{end}}
<meta property="og:title" content="{{.OgTitle}}">
{{if .OgDescription}}<meta property="og:description" content="{{.OgDescription}}">{{end}}
{{if .OgImage}}<meta property="og:image" content="{{.OgImage}}">{{end}}
<meta property="og:url" content="{{.OgURL}}">
<meta property="og:type" content="{{.OgType}}">
<meta name="twitter:card" content="{{.TwitterCard}}">
<meta name="twitter:title" content="{{.OgTitle}}">
{{if .OgDescription}}<meta name="twitter:description" content="{{.OgDescription}}">{{end}}
{{if .OgImage}}<meta name="twitter:image" content="{{.OgImage}}">{{end}}
{{if .JSONLD}}<script type="application/ld+json">{{.JSONLD}}</script>{{end}}
<style>
body{font-family:system-ui,-apple-system,sans-serif;max-width:800px;margin:0 auto;padding:1rem;line-height:1.6}
nav.breadcrumbs{font-size:.85rem;margin-bottom:1rem;opacity:.7}
nav.breadcrumbs a{color:inherit}
img{max-width:100%;height:auto}
</style>
</head>
<body>
{{if .Breadcrumbs}}<nav class="breadcrumbs" aria-label="Хлебные крошки">{{range $i, $b := .Breadcrumbs}}{{if $i}} / {{end}}{{if $b.URL}}<a href="{{$b.URL}}">{{$b.Name}}</a>{{else}}<span aria-current="page">{{$b.Name}}</span>{{end}}{{end}}</nav>{{end}}
<main>
{{.Content}}
</main>
</body>
</html>`

var pageTmpl = template.Must(template.New("page").Parse(pageTemplate))

// renderPage produces full HTML for a page.
func (e *Engine) renderPage(p *Page) ([]byte, error) {
	// Sanitize body HTML
	rawHTML, err := e.cfg.Renderer.Render(p.Markdown)
	if err != nil {
		return nil, fmt.Errorf("render md: %w", err)
	}
	cleanHTML := sanitizeHTML(string(rawHTML))

	// Build SEO fields with fallbacks
	meta := p.Meta.SEO
	title := meta.MetaTitle
	if title == "" {
		title = p.Meta.Title
	}
	ogTitle := meta.OgTitle
	if ogTitle == "" {
		ogTitle = title
	}
	ogType := meta.OgType
	if ogType == "" {
		ogType = "website"
	}
	twitterCard := meta.TwitterCard
	if twitterCard == "" {
		twitterCard = "summary_large_image"
	}
	canonical := meta.Canonical
	if canonical == "" {
		canonical = e.cfg.SiteURL + p.URL
	}
	ogImage := meta.OgImage
	if ogImage != "" && strings.HasPrefix(ogImage, "/") {
		ogImage = e.cfg.SiteURL + ogImage
	}
	ogURL := e.cfg.SiteURL + p.URL

	// Breadcrumbs from path
	crumbs := e.buildBreadcrumbs(p)

	// JSON-LD
	jsonld := e.buildJSONLD(p, canonical, ogImage, crumbs)

	alternateMD := ""
	if e.cfg.SiteURL != "" {
		alternateMD = e.cfg.SiteURL + p.URL + ".md"
	}

	data := PageData{
		Title:         title,
		MetaDesc:      meta.MetaDescription,
		Robots:        meta.Robots,
		Canonical:     canonical,
		OgTitle:       ogTitle,
		OgDescription: meta.OgDescription,
		OgImage:       ogImage,
		OgType:        ogType,
		OgURL:         ogURL,
		TwitterCard:   twitterCard,
		JSONLD:        template.JS(jsonld),
		AlternateMD:   alternateMD,
		Breadcrumbs:   crumbs,
		Content:       template.HTML(cleanHTML),
	}
	var buf bytes.Buffer
	if err := pageTmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("execute template: %w", err)
	}
	return buf.Bytes(), nil
}

// buildBreadcrumbs generates the trail from URL path.
func (e *Engine) buildBreadcrumbs(p *Page) []Breadcrumb {
	crumbs := []Breadcrumb{{Name: "Главная", URL: "/"}}
	// Reconstruct from path: pages/docs/install → Главная / [Секция] / Заголовок
	urlPath := strings.Trim(p.URL, "/")
	segments := strings.Split(urlPath, "/")
	acc := ""
	for i, seg := range segments {
		acc += "/" + seg
		if i == len(segments)-1 {
			// Last = current page
			name := p.Meta.SEO.BreadcrumbTitle
			if name == "" {
				name = p.Meta.Title
			}
			crumbs = append(crumbs, Breadcrumb{Name: name, URL: ""})
		} else {
			// Section — section.json в каталоге, которому соответствует сегмент
			sec, _ := e.store.ReadSection(strings.TrimPrefix(acc, "/"))
			name := sec.Title
			if name == "" {
				name = seg
			}
			crumbs = append(crumbs, Breadcrumb{Name: name, URL: acc})
		}
	}
	return crumbs
}

// breadcrumbsHTML рендерит навигационную цепочку (для шаблонов листингов без html/template).
func breadcrumbsHTML(crumbs []Breadcrumb) string {
	var b strings.Builder
	b.WriteString(`<nav class="breadcrumbs" aria-label="Хлебные крошки">`)
	for i, c := range crumbs {
		if i > 0 {
			b.WriteString(" / ")
		}
		if c.URL != "" {
			fmt.Fprintf(&b, `<a href="%s">%s</a>`, html.EscapeString(c.URL), html.EscapeString(c.Name))
		} else {
			fmt.Fprintf(&b, `<span aria-current="page">%s</span>`, html.EscapeString(c.Name))
		}
	}
	b.WriteString(`</nav>`)
	return b.String()
}

// buildJSONLD generates structured data for the page.
func (e *Engine) buildJSONLD(p *Page, canonical, ogImage string, crumbs []Breadcrumb) string {
	schema := map[string]any{
		"@context": "https://schema.org",
		"@type":    "Article",
		"headline": p.Meta.Title,
	}
	if p.Meta.Date != "" {
		schema["datePublished"] = p.Meta.Date
	}
	if p.Meta.Modified != "" {
		schema["dateModified"] = p.Meta.Modified
	}
	if ogImage != "" {
		schema["image"] = ogImage
	}
	if canonical != "" {
		schema["mainEntityOfPage"] = canonical
	}
	// BreadcrumbList
	if len(crumbs) >= 2 {
		items := make([]map[string]any, 0, len(crumbs))
		for i, c := range crumbs {
			item := map[string]any{
				"@type":    "ListItem",
				"position": i + 1,
				"name":     c.Name,
			}
			if c.URL != "" {
				item["item"] = e.cfg.SiteURL + c.URL
			}
			items = append(items, item)
		}
		schema["breadcrumb"] = map[string]any{
			"@type":           "BreadcrumbList",
			"itemListElement": items,
		}
	}
	b, _ := json.Marshal(schema)
	return string(b)
}

// sanitizeHTML — allowlist-based HTML sanitizer.
func sanitizeHTML(input string) string {
	// Allowed tags
	allowedTags := map[string]bool{
		"p": true, "br": true, "em": true, "strong": true, "i": true, "b": true,
		"ul": true, "ol": true, "li": true,
		"blockquote": true, "code": true, "pre": true,
		"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
		"table": true, "thead": true, "tbody": true, "tr": true, "th": true, "td": true,
		"a": true, "img": true, "hr": true, "span": true, "div": true,
		"figure": true, "figcaption": true,
	}
	// Allowed attributes per tag
	allowedAttrs := map[string]map[string]bool{
		"a":   {"href": true, "title": true},
		"img": {"src": true, "width": true, "height": true, "alt": true, "align": true},
	}
	// Allowed URL schemes
	allowedSchemes := map[string]bool{
		"https": true, "http": true, "/": true, "mailto": true,
	}

	var out strings.Builder
	i := 0
	for i < len(input) {
		if input[i] == '<' {
			// Find closing '>' respecting quoted attributes
			end := findTagEnd(input, i)
			if end < 0 {
				break
			}
			tag := input[i : end+1]
			cleaned := cleanTag(tag, allowedTags, allowedAttrs, allowedSchemes)
			out.WriteString(cleaned)
			i = end + 1
			// If disallowed container tag, skip to its closing tag
			tagName := extractTagName(tag)
			if !allowedTags[tagName] && !strings.HasPrefix(tag, "</") && !strings.HasSuffix(tag, "/>") {
				closeTag := "</" + tagName + ">"
				idx := strings.Index(strings.ToLower(input[i:]), strings.ToLower(closeTag))
				if idx >= 0 {
					i += idx + len(closeTag)
				}
			}
		} else {
			out.WriteByte(input[i])
			i++
		}
	}
	return out.String()
}

// findTagEnd finds the index of '>' that closes the tag starting at 'start',
// respecting quoted attribute values that may contain '<' or '>'.
func findTagEnd(input string, start int) int {
	inQuote := byte(0)
	for i := start + 1; i < len(input); i++ {
		c := input[i]
		if inQuote != 0 {
			if c == inQuote {
				inQuote = 0
			}
			continue
		}
		if c == '"' || c == '\'' {
			inQuote = c
			continue
		}
		if c == '>' {
			return i
		}
	}
	return -1
}

// extractTagName returns the lowercase tag name from a tag string.
func extractTagName(tag string) string {
	tag = strings.TrimPrefix(tag, "<")
	tag = strings.TrimPrefix(tag, "/")
	space := strings.IndexAny(tag, " \t\n/>")
	if space < 0 {
		return strings.ToLower(strings.TrimSuffix(tag, ">"))
	}
	return strings.ToLower(tag[:space])
}

// cleanTag validates and cleans a single HTML tag.
func cleanTag(tag string, allowedTags map[string]bool, allowedAttrs map[string]map[string]bool, allowedSchemes map[string]bool) string {
	// Skip closing tags
	if strings.HasPrefix(tag, "</") {
		name := strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(tag, "</"), ">"))
		if allowedTags[name] {
			return "</" + name + ">"
		}
		return ""
	}
	// Self-closing or opening tag
	inner := strings.TrimSuffix(strings.TrimPrefix(tag, "<"), ">")
	inner = strings.TrimSuffix(inner, "/")

	// Extract tag name
	space := strings.IndexAny(inner, " \t\n")
	if space < 0 {
		name := strings.ToLower(inner)
		if allowedTags[name] {
			return "<" + name + ">"
		}
		return ""
	}
	name := strings.ToLower(inner[:space])
	if !allowedTags[name] {
		return ""
	}

	// Parse attributes
	attrs := parseAttrs(inner[space:])
	var out strings.Builder
	out.WriteString("<" + name)
	allowed := allowedAttrs[name]
	for k, v := range attrs {
		if allowed == nil || !allowed[k] {
			continue
		}
		// Scheme check for src/href
		if k == "src" || k == "href" {
			if !isAllowedURL(v, allowedSchemes) {
				continue
			}
		}
		// Strip on* handlers (already filtered by allowlist but belt-and-suspenders)
		if strings.HasPrefix(strings.ToLower(k), "on") {
			continue
		}
		out.WriteString(" " + k + `="` + escapeAttr(v) + `"`)
	}
	if strings.HasSuffix(tag, "/>") {
		out.WriteString("/>")
	} else {
		out.WriteString(">")
	}
	return out.String()
}

// parseAttrs extracts key="value" pairs from an attribute string.
func parseAttrs(s string) map[string]string {
	attrs := make(map[string]string)
	s = strings.TrimSpace(s)
	for s != "" {
		s = strings.TrimLeft(s, " \t\n")
		if s == "" {
			break
		}
		eq := strings.IndexByte(s, '=')
		if eq < 0 {
			// Boolean attribute
			name := strings.ToLower(strings.Fields(s)[0])
			attrs[name] = ""
			s = ""
			continue
		}
		name := strings.ToLower(strings.TrimSpace(s[:eq]))
		rest := strings.TrimLeft(s[eq+1:], " \t\n")
		var val string
		if len(rest) > 0 && (rest[0] == '"' || rest[0] == '\'') {
			q := rest[0]
			end := strings.IndexByte(rest[1:], q)
			if end < 0 {
				break
			}
			val = rest[1 : 1+end]
			s = rest[end+2:]
		} else {
			fields := strings.Fields(rest)
			if len(fields) == 0 {
				break
			}
			val = fields[0]
			s = strings.TrimPrefix(rest, val)
		}
		attrs[name] = val
	}
	return attrs
}

// isAllowedURL checks scheme allowlist.
func isAllowedURL(u string, allowed map[string]bool) bool {
	u = strings.TrimSpace(u)
	if u == "" {
		return false
	}
	if strings.HasPrefix(u, "//") {
		return false // protocol-relative
	}
	if strings.HasPrefix(u, "/") {
		return true // root-relative
	}
	if strings.HasPrefix(u, "data:") || strings.HasPrefix(u, "javascript:") || strings.HasPrefix(u, "vbscript:") {
		return false
	}
	parsed, err := url.Parse(u)
	if err != nil {
		return false
	}
	if parsed.Scheme == "" {
		return true // relative path
	}
	return allowed[parsed.Scheme]
}

// escapeAttr escapes attribute values.
func escapeAttr(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	return s
}
