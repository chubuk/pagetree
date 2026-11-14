# SEO-пакет PageTree

[Русский](SEO.md)

## Что генерируется в `<head>`

Автоматически из sidecar JSON:

1. `<title>` — из `seo.meta_title` или `title`
2. `<meta name="description">` — из `seo.meta_description`
3. `<meta name="robots">` — из `seo.robots` (index/noindex/follow/nofollow/nosnippet)
4. `<link rel="canonical">` — из `seo.canonical` или авто из SiteURL+path
5. **Open Graph**: `og:title`, `og:description`, `og:image`, `og:url`, `og:type`
6. **Twitter Card**: `twitter:card`, `twitter:title`, `twitter:description`, `twitter:image`
7. **JSON-LD**: `Article`/`NewsArticle`/`BlogPosting` + `BreadcrumbList`
8. `<link rel="alternate" type="text/markdown">` — ссылка на .md версию

## Поля sidecar JSON (seo.*)

| Поле | Назначение | Рекомендация |
|---|---|---|
| `meta_title` | Заголовок для поисковиков | ~60 символов |
| `meta_description` | Описание | ~120-160 символов |
| `robots` | Управление индексацией | `index, follow` (default) |
| `og_title` | Заголовок для соцсетей | fallback: meta_title |
| `og_description` | Описание для соцсетей | fallback: meta_description |
| `og_image` | Картинка для соцсетей | root-relative `/media/...` |
| `og_type` | Тип | `website` (default) |
| `twitter_card` | Тип Twitter-карточки | `summary_large_image` |
| `canonical` | Канонический URL | авто из SiteURL+path |
| `breadcrumb_title` | Название в крошках | override |

**`keywords` удалён** — Google официально игнорирует keywords meta.

## Хлебные крошки

**Тропинка — автоматически из вложенности:**
`/pages/docs/install` → `Главная / Документация / Установка`

**Названия — редактируемые:**
- Страница: `title` из JSON, override через `seo.breadcrumb_title`
- Раздел: `section.json.title` → fallback: humanized slug

**JSON-LD BreadcrumbList** генерируется из той же тропинки (≥2 ListItem, position с 1, name обязателен, item — абсолютный URL кроме последнего).

## Draft-семантика

`draft:true` → 404 публично + исключён из sitemap/llms.txt. Побеждает любое `seo.robots`.

## Автогенерация

| Файл | Что |
|---|---|
| `sitemap.xml` | Все опубликованные страницы + новости |
| `llms.txt` | Markdown-оглавление сайта (llms.txt v2 spec) |
| `robots.txt` | Редактируемый в админке + AI-ботовые секции |

## Абсолютные URL

OG/JSON-LD/sitemap требуют абсолютных URL. `og_image` хранится root-relative (`/media/...`), при рендере раскрывается в абсолютные через `Config.SiteURL`.
