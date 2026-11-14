package pagetree

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// PageMeta — sidecar JSON fields for a page/news entry.
type PageMeta struct {
	Title    string    `json:"title"`
	Menu     bool      `json:"menu"`
	Order    int       `json:"order"`
	Date     string    `json:"date,omitempty"`     // "2006-01-02"
	Modified string    `json:"modified,omitempty"` // "2006-01-02"
	Draft    bool      `json:"draft"`
	SEO      SEOFields `json:"seo,omitempty"`
}

// SEOFields — SEO metadata in sidecar JSON.
type SEOFields struct {
	MetaTitle       string `json:"meta_title,omitempty"`
	MetaDescription string `json:"meta_description,omitempty"`
	Robots          string `json:"robots,omitempty"`
	OgTitle         string `json:"og_title,omitempty"`
	OgDescription   string `json:"og_description,omitempty"`
	OgImage         string `json:"og_image,omitempty"` // root-relative
	OgType          string `json:"og_type,omitempty"`
	TwitterCard     string `json:"twitter_card,omitempty"`
	Canonical       string `json:"canonical,omitempty"`
	BreadcrumbTitle string `json:"breadcrumb_title,omitempty"`
}

// SectionMeta — folder-level metadata (section.json).
type SectionMeta struct {
	Title string `json:"title"`
	Order int    `json:"order"`
	Menu  bool   `json:"menu"`
}

// Page — a loaded page: raw markdown + parsed meta.
type Page struct {
	Path     string // relative to content root: "pages/about.md"
	URL      string // "/pages/about"
	Markdown []byte
	Meta     PageMeta
}

// ContentEntry — tree node for admin listing.
type ContentEntry struct {
	Path     string         `json:"path"`
	URL      string         `json:"url,omitempty"`
	Type     string         `json:"type"` // "page","news","section","orphan","dir"
	Title    string         `json:"title"`
	IsDraft  bool           `json:"is_draft,omitempty"`
	HasMD    bool           `json:"has_md"`
	HasJSON  bool           `json:"has_json"`
	Children []ContentEntry `json:"children,omitempty"`
}

// MediaFile — a media asset entry.
type MediaFile struct {
	Name    string    `json:"name"`
	URL     string    `json:"url"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mtime"`
}

// Store — filesystem operations under content root.
type Store struct {
	root string // absolute path to content/
}

// NewStore validates root and returns a Store.
func NewStore(root string) (*Store, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("abs root: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("stat root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("root %s is not a directory", abs)
	}
	return &Store{root: abs}, nil
}

// guard cleans a relative path and ensures it stays under content root.
func (s *Store) guard(rel string) (string, error) {
	rel = filepath.ToSlash(rel)
	rel = strings.TrimPrefix(rel, "/")
	clean := filepath.Clean(rel)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == "." {
		return "", fmt.Errorf("path %q escapes content root", rel)
	}
	abs := filepath.Join(s.root, clean)
	// Ensure abs is under root after symlinks
	realRoot, err := filepath.EvalSymlinks(s.root)
	if err != nil {
		return "", fmt.Errorf("eval root symlinks: %w", err)
	}
	realAbs, err := filepath.EvalSymlinks(abs)
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("eval path symlinks: %w", err)
	}
	if err == nil && !strings.HasPrefix(realAbs, realRoot+string(filepath.Separator)) && realAbs != realRoot {
		return "", fmt.Errorf("path %q escapes content root via symlink", rel)
	}
	return clean, nil
}

// resolveURL maps a public URL path to a relative .md file path.
// "/pages/about" → "pages/about.md"; "/pages/docs/" → "pages/docs/index.md"
func (s *Store) resolveURL(urlPath string) (string, error) {
	urlPath = strings.TrimPrefix(urlPath, "/")
	urlPath = strings.TrimSuffix(urlPath, "/")
	if urlPath == "" {
		return "", errors.New("пустой путь")
	}
	// Try direct file: <path>.md
	direct := urlPath + ".md"
	if _, err := os.Stat(filepath.Join(s.root, filepath.FromSlash(direct))); err == nil {
		return s.guard(direct)
	}
	// Try index: <path>/index.md
	idx := urlPath + "/index.md"
	if _, err := os.Stat(filepath.Join(s.root, filepath.FromSlash(idx))); err == nil {
		return s.guard(idx)
	}
	return "", fmt.Errorf("no page at %q", urlPath)
}

// pathToURL maps a relative .md path to its public URL.
// "pages/about.md" → "/pages/about"; "pages/docs/index.md" → "/pages/docs/"
func (s *Store) pathToURL(rel string) string {
	rel = filepath.ToSlash(rel)
	rel = strings.TrimSuffix(rel, ".md")
	if strings.HasSuffix(rel, "/index") {
		rel = strings.TrimSuffix(rel, "/index")
		if rel == "" {
			return "/"
		}
		return "/" + rel + "/"
	}
	return "/" + rel
}

// ReadPage loads .md and its sidecar .json (defaults if missing).
func (s *Store) ReadPage(rel string) (*Page, error) {
	rel, err := s.guard(rel)
	if err != nil {
		return nil, err
	}
	md, err := os.ReadFile(filepath.Join(s.root, filepath.FromSlash(rel)))
	if err != nil {
		return nil, fmt.Errorf("read md: %w", err)
	}
	meta := PageMeta{}
	jsonPath := strings.TrimSuffix(rel, ".md") + ".json"
	jsonData, err := os.ReadFile(filepath.Join(s.root, filepath.FromSlash(jsonPath)))
	if err == nil {
		if err := json.Unmarshal(jsonData, &meta); err != nil {
			return nil, fmt.Errorf("parse sidecar %s: %w", jsonPath, err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read sidecar: %w", err)
	}
	// Defaults
	if meta.Title == "" {
		meta.Title = defaultTitle(md, rel)
	}
	return &Page{
		Path:     rel,
		URL:      s.pathToURL(rel),
		Markdown: md,
		Meta:     meta,
	}, nil
}

// defaultTitle extracts first H1 or falls back to filename.
func defaultTitle(md []byte, rel string) string {
	for _, line := range strings.Split(string(md), "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "# ") {
			return strings.TrimSpace(t[2:])
		}
	}
	base := filepath.Base(rel)
	return strings.TrimSuffix(base, ".md")
}

// WritePage atomically writes .md and sidecar .json.
func (s *Store) WritePage(rel string, md []byte, meta *PageMeta) error {
	rel, err := s.guard(rel)
	if err != nil {
		return err
	}
	if !strings.HasSuffix(rel, ".md") {
		return fmt.Errorf("path %q must end with .md", rel)
	}
	abs := filepath.Join(s.root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	// Write .md atomically
	if err := atomicWrite(abs, md, 0o644); err != nil {
		return fmt.Errorf("write md: %w", err)
	}
	// Write .json atomically
	if meta != nil {
		jsonPath := strings.TrimSuffix(abs, ".md") + ".json"
		jd, err := json.MarshalIndent(meta, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal meta: %w", err)
		}
		if err := atomicWrite(jsonPath, append(jd, '\n'), 0o644); err != nil {
			return fmt.Errorf("write meta: %w", err)
		}
	}
	return nil
}

// DeletePage removes .md, .json, and <name>_media/ (triple).
func (s *Store) DeletePage(rel string) error {
	rel, err := s.guard(rel)
	if err != nil {
		return err
	}
	base := strings.TrimSuffix(rel, ".md")
	abs := filepath.Join(s.root, filepath.FromSlash(rel))
	_ = os.Remove(abs) // .md
	_ = os.Remove(filepath.Join(s.root, filepath.FromSlash(base+".json")))
	_ = os.RemoveAll(filepath.Join(s.root, filepath.FromSlash(base+"_media")))
	return nil
}

// RenamePage moves .md, .json, and <name>_media/ to a new path (triple).
func (s *Store) RenamePage(oldRel, newRel string) error {
	oldRel, err := s.guard(oldRel)
	if err != nil {
		return err
	}
	newRel, err = s.guard(newRel)
	if err != nil {
		return err
	}
	if !strings.HasSuffix(newRel, ".md") {
		return fmt.Errorf("new path %q must end with .md", newRel)
	}
	oldBase := strings.TrimSuffix(oldRel, ".md")
	newBase := strings.TrimSuffix(newRel, ".md")
	oldAbs := filepath.Join(s.root, filepath.FromSlash(oldRel))
	newAbs := filepath.Join(s.root, filepath.FromSlash(newRel))

	if err := os.MkdirAll(filepath.Dir(newAbs), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	// Move .md
	if err := os.Rename(oldAbs, newAbs); err != nil {
		return fmt.Errorf("rename md: %w", err)
	}
	// Move .json
	oldJSON := filepath.Join(s.root, filepath.FromSlash(oldBase+".json"))
	newJSON := filepath.Join(s.root, filepath.FromSlash(newBase+".json"))
	_ = os.Rename(oldJSON, newJSON) // optional file
	// Move _media/
	oldMedia := filepath.Join(s.root, filepath.FromSlash(oldBase+"_media"))
	newMedia := filepath.Join(s.root, filepath.FromSlash(newBase+"_media"))
	_ = os.Rename(oldMedia, newMedia) // optional dir
	return nil
}

// ListTree returns a recursive content tree for admin.
func (s *Store) ListTree(subdir string) ([]ContentEntry, error) {
	subdir, err := s.guard(subdir)
	if err != nil {
		return nil, err
	}
	abs := filepath.Join(s.root, filepath.FromSlash(subdir))
	return s.listDir(abs, subdir)
}

func (s *Store) listDir(abs, rel string) ([]ContentEntry, error) {
	entries, err := os.ReadDir(abs)
	if err != nil {
		return nil, err
	}
	var result []ContentEntry
	mdSet := map[string]bool{}
	jsonSet := map[string]bool{}

	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			if strings.HasSuffix(name, "_media") {
				continue // media dirs shown as part of parent
			}
			childRel := filepath.ToSlash(filepath.Join(rel, name))
			children, err := s.listDir(filepath.Join(abs, name), childRel)
			if err != nil {
				continue
			}
			// Read section.json for title
			sec := SectionMeta{}
			sjPath := filepath.Join(abs, name, "section.json")
			if jd, err := os.ReadFile(sjPath); err == nil {
				_ = json.Unmarshal(jd, &sec)
			}
			title := sec.Title
			if title == "" {
				title = name
			}
			result = append(result, ContentEntry{
				Path:     childRel,
				URL:      s.pathToURL(childRel + "/index.md"),
				Type:     "dir",
				Title:    title,
				Children: children,
			})
			continue
		}
		if strings.HasSuffix(name, ".md") {
			mdSet[name] = true
		} else if strings.HasSuffix(name, ".json") && name != "section.json" {
			jsonSet[name] = true
		}
	}

	// Pair .md + .json
	for name := range mdSet {
		base := strings.TrimSuffix(name, ".md")
		childRel := filepath.ToSlash(filepath.Join(rel, name))
		meta := PageMeta{}
		if jsonSet[base+".json"] {
			jd, _ := os.ReadFile(filepath.Join(abs, base+".json"))
			_ = json.Unmarshal(jd, &meta)
		}
		if meta.Title == "" {
			meta.Title = base
		}
		typ := "page"
		if strings.HasPrefix(rel, "news") {
			typ = "news"
		}
		result = append(result, ContentEntry{
			Path:    childRel,
			URL:     s.pathToURL(childRel),
			Type:    typ,
			Title:   meta.Title,
			IsDraft: meta.Draft,
			HasMD:   true,
			HasJSON: jsonSet[base+".json"],
		})
	}
	// Orphan .json without .md
	for name := range jsonSet {
		base := strings.TrimSuffix(name, ".json")
		if !mdSet[base+".md"] {
			childRel := filepath.ToSlash(filepath.Join(rel, name))
			result = append(result, ContentEntry{
				Path:    childRel,
				Type:    "orphan",
				Title:   base,
				HasMD:   false,
				HasJSON: true,
			})
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Path < result[j].Path
	})
	return result, nil
}

// ReadSection reads section.json from a folder.
func (s *Store) ReadSection(rel string) (*SectionMeta, error) {
	rel, err := s.guard(rel)
	if err != nil {
		return nil, err
	}
	p := filepath.Join(s.root, filepath.FromSlash(rel), "section.json")
	jd, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return &SectionMeta{}, nil
		}
		return nil, err
	}
	var sec SectionMeta
	if err := json.Unmarshal(jd, &sec); err != nil {
		return nil, fmt.Errorf("parse section.json: %w", err)
	}
	return &sec, nil
}

// WriteSection writes section.json to a folder.
func (s *Store) WriteSection(rel string, sec *SectionMeta) error {
	rel, err := s.guard(rel)
	if err != nil {
		return err
	}
	p := filepath.Join(s.root, filepath.FromSlash(rel), "section.json")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	jd, err := json.MarshalIndent(sec, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(p, append(jd, '\n'), 0o644)
}

// ListMedia lists files in a media directory.
func (s *Store) ListMedia(rel string) ([]MediaFile, error) {
	rel, err := s.guard(rel)
	if err != nil {
		return nil, err
	}
	abs := filepath.Join(s.root, filepath.FromSlash(rel))
	entries, err := os.ReadDir(abs)
	if err != nil {
		return nil, err
	}
	var result []MediaFile
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		result = append(result, MediaFile{
			Name:    e.Name(),
			URL:     "/" + filepath.ToSlash(filepath.Join(rel, e.Name())),
			Size:    info.Size(),
			ModTime: info.ModTime(),
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

// SaveMedia writes a media file atomically. Returns root-relative URL.
func (s *Store) SaveMedia(dir, filename string, data []byte) (string, error) {
	dir, err := s.guard(dir)
	if err != nil {
		return "", err
	}
	// Sanitize filename
	filename = sanitizeFilename(filename)
	if filename == "" {
		return "", errors.New("пустое имя файла после санитизации")
	}
	abs := filepath.Join(s.root, filepath.FromSlash(dir), filename)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", err
	}
	if err := atomicWrite(abs, data, 0o644); err != nil {
		return "", err
	}
	return "/" + filepath.ToSlash(filepath.Join(dir, filename)), nil
}

// DeleteMedia removes a media file.
func (s *Store) DeleteMedia(rel string) error {
	rel, err := s.guard(rel)
	if err != nil {
		return err
	}
	return os.Remove(filepath.Join(s.root, filepath.FromSlash(rel)))
}

// RenameMedia renames a media file.
func (s *Store) RenameMedia(oldRel, newRel string) error {
	oldRel, err := s.guard(oldRel)
	if err != nil {
		return err
	}
	newRel, err = s.guard(newRel)
	if err != nil {
		return err
	}
	oldAbs := filepath.Join(s.root, filepath.FromSlash(oldRel))
	newAbs := filepath.Join(s.root, filepath.FromSlash(newRel))
	if err := os.MkdirAll(filepath.Dir(newAbs), 0o755); err != nil {
		return err
	}
	return os.Rename(oldAbs, newAbs)
}

// atomicWrite writes data to path via tmp+rename.
func atomicWrite(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), perm); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// sanitizeFilename allows only [a-z0-9._-], rejects "..".
func sanitizeFilename(name string) string {
	name = filepath.Base(name) // strip path
	name = strings.ToLower(name)
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			b.WriteRune(r)
		}
	}
	s := b.String()
	s = strings.TrimLeft(s, ".")
	if s == ".." {
		return ""
	}
	return s
}
