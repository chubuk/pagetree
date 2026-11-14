# Формат контента PageTree

[Русский](CONTENT.md)

## Структура каталогов

```
content/
  pages/
    about.md              → /pages/about
    about.json            # sidecar метаданные
    about_media/          # картинки этой страницы
    docs/
      install.md          → /pages/docs/install
      install.json
      section.json        # метаданные папки
      index.md            → /pages/docs/ (каталог раздела)
  news/
    releases/
      v1.2.md             → /news/releases/v1.2
      v1.2.json
  media/                  # общие картинки
```

## Правила URL

- `pages/about.md` → `/pages/about`
- `pages/docs/install.md` → `/pages/docs/install`
- `pages/docs/index.md` → `/pages/docs/` (index обслуживает «каталог»)
- Trailing slash → 301 на канонический (без `/`)
- Неизвестный путь → 404 (не SPA-fallback)

## Sidecar JSON (about.json)

Файл с тем же именем, но `.json`. Редактируется отдельной формой (не в markdown).

```json
{
  "title": "О нас",
  "menu": true,
  "order": 20,
  "date": "2026-11-01",
  "modified": "2026-11-14",
  "draft": false,
  "seo": {
    "meta_title": "О нас — компания N",
    "meta_description": "Краткое описание для поисковиков",
    "robots": "index, follow",
    "og_title": "О нас",
    "og_description": "Для соцсетей",
    "og_image": "/media/og-about.jpg",
    "og_type": "website",
    "twitter_card": "summary_large_image",
    "canonical": "https://site/pages/about",
    "breadcrumb_title": "О компании"
  }
}
```

## Section.json (метаданные папки)

```json
{ "title": "Документация", "order": 10, "menu": true }
```

Задаёт имя раздела в меню/крошках. Fallback: `section.json.title` → `title` из `index.md` → humanized slug.

## Жизненный цикл

| Состояние | Поведение |
|---|---|
| `.md` без `.json` | Валидно: title из H1, `menu:false`, `draft:false` |
| `.json` без `.md` | Сирота: не рендерится, видна в админке |
| rename/delete | Транзакционно на тройку (md+json+_media/) |
| Запись | Атомарно: `{md,json}` одним вызовом, tmp+rename |

## Картинки

- **Root-relative пути везде**: `/media/photo.jpg`, `/pages/about_media/photo.jpg`
- Общий каталог: `media/`
- Каталог страницы: `about_media/` (рядом с `about.md`)
- Форматы: jpg/jpeg/png/webp/gif
- Для размеров/выравнивания — HTML `<img>` внутри markdown:
  `<img src="/media/photo.jpg" width="600" height="400" align="center" alt="...">`

## Markdown

Чистый markdown без front matter. Поддерживается raw HTML (проходит через sanitizer). Рендер — через pluggable `Renderer` (gomarkdown, goldmark и др.).
