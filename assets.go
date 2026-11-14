package pagetree

import "embed"

//go:embed assets/admin.html assets/contentadmin.js assets/vditor/index.min.js assets/vditor/index.css assets/vditor/method.min.js assets/vditor/js/i18n/ru_RU.js
var assetsFS embed.FS
