// Package pagetree — embeddable file-based markdown page/news engine.
// Pages and news are .md files with sidecar .meta.json; URL = directory nesting.
package pagetree

import (
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
)

// Renderer — pluggable md→HTML converter (consumer supplies, e.g. gomarkdown).
type Renderer interface {
	Render(md []byte) ([]byte, error)
}

// Config for Engine construction.
type Config struct {
	Root      string       // content root: pages/, news/, media/
	DataDir   string       // service data (keys.json, robots.txt) — outside web root
	SiteURL   string       // absolute base URL "https://site.ru" for OG/JSON-LD/sitemap
	Renderer  Renderer     // md→HTML
	BaseURL   string       // mount prefix, e.g. "" or "/site"
	Log       *slog.Logger // optional, nil → no-op
	FS        fs.FS        // optional asset override (embed.FS default)
	MasterKey []byte       // AES-256 key for keys.json encryption (nil = no encryption)
	PrevKey   []byte       // previous master key (for rotation)
}

// Engine holds all state; no globals.
type Engine struct {
	cfg      Config
	store    *Store
	keystore *KeyStore
	log      *slog.Logger
}

// New validates config and builds the engine.
func New(cfg Config) (*Engine, error) {
	if cfg.Root == "" {
		return nil, fmt.Errorf("pagetree: Config.Root is required")
	}
	if cfg.Renderer == nil {
		return nil, fmt.Errorf("pagetree: Config.Renderer is required")
	}
	log := cfg.Log
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil)) //nolint:staticcheck // no-op fallback
	}
	s, err := NewStore(cfg.Root)
	if err != nil {
		return nil, fmt.Errorf("pagetree: store: %w", err)
	}
	var ks *KeyStore
	if cfg.DataDir != "" {
		ks, err = NewKeyStore(cfg.DataDir, cfg.MasterKey, cfg.PrevKey)
		if err != nil {
			return nil, fmt.Errorf("pagetree: keystore: %w", err)
		}
	}
	return &Engine{cfg: cfg, store: s, keystore: ks, log: log}, nil
}

// PageHandler — GET /pages/{path...} (public).
func (e *Engine) PageHandler() http.Handler { return e.pageHandler() }

// NewsHandler — GET /news/{...}, blog, listings (public).
func (e *Engine) NewsHandler() http.Handler { return e.newsHandler() }

// MenuHandler — GET /api/content/menu (public JSON).
func (e *Engine) MenuHandler() http.Handler { return e.menuHandler() }

// MediaHandler — GET /media/{path...}, *_media/ (public).
func (e *Engine) MediaHandler() http.Handler { return e.mediaHandler() }

// ContentAPI — CRUD md/json + upload. Consumer wraps with auth middleware.
func (e *Engine) ContentAPI() http.Handler { return e.contentAPI() }

// AgentAPI — same content API under module-owned Bearer key auth.
func (e *Engine) AgentAPI() http.Handler { return e.agentAPI() }

// KeysAPI — AI-key management. Consumer wraps with auth middleware.
func (e *Engine) KeysAPI() http.Handler { return e.keysAPI() }

// AdminUIHandler — Vditor UI shell (static, no secrets).
func (e *Engine) AdminUIHandler() http.Handler { return e.adminUIHandler() }
