# Безопасность PageTree

[Русский](SECURITY.md)

## Модель угроз

Основные векторы для файлового контент-движка:
1. Path traversal (выход за пределы content/)
2. Stored XSS (загрузка исполняемых файлов)
3. Утечка ключей агентов
4. Несанкционированная запись контента

## Защита

### Path guard
Каждый путь проходит через `guard()`:
- `filepath.Clean` — нормализация
- Проверка префикса `content/`
- `filepath.EvalSymlinks` — обход symlink-escape

### Upload allowlist
Только `jpg/jpeg/png/webp/gif`. Имена санитизируются: `[a-z0-9._-]`, lowercase, без `..`.
HTML/SVG запрещены — `x.html`/`x.svg` исполнялись бы на origin приложения.

### HTML sanitizer (при рендере)
Allowlist тегов: `p, br, em, strong, i, b, ul, ol, li, blockquote, code, pre, h1-h6, table*, a, img, hr, span, div, figure, figcaption`

Allowlist атрибутов:
- `a`: href, title
- `img`: src, width, height, alt, align

Scheme allowlist для src/href: `https`, `http`, root-relative `/`, relative. Запрещены: `javascript:`, `data:`, `vbscript:`, `//` (protocol-relative).

Всё остальное вырезается, включая `on*` обработчики.

### AI-ключи

**Hash-then-store:** raw-ключ `ptk_live_<64hex>` (32 rand bytes → hex), в файле хранится только SHA-256 hex. Сверка — `crypto/subtle.ConstantTimeCompare`.

**Опциональное шифрование:** `keys.json` шифруется AES-256-GCM (`NewGCMWithRandomNonce`) при заданном `PAGETREE_MASTER_KEY` (32 байта). Ротация: `PAGETREE_MASTER_KEY_PREVIOUS` для decrypt старого файла.

**Жизненный цикл:** create (raw один раз) → update (имя/policy) → revoke → delete.

**Policy per key:** `pages`/`news`/`media` × `r`/`rw`/`w`/`none`. Deny побеждает. Проверка на каждый вызов.

### Rate limiting
- REST: per-key через nginx (отдельная зона)
- MCP: per-key `rate_per_min` в policy (in-module)
- Upload: 20MB decoded / 32MB body-cap

### Fail-closed
Без мастер-ключа при зашифрованном файле — отказ, не пустой store. Без Bearer-токена — 401. Невалидный policy scope — 403.

## Файлы

| Файл | Perms | Расположение |
|---|---|---|
| `keys.json` | 0600 | `DataDir` (вне web-root) |
| `*.md`, `*.json` | 0644 | `content/` |
| `media/*` | 0644 | `content/media/` |
