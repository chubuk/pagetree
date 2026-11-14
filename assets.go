package pagetree

import "embed"

//go:embed assets/admin.html assets/contentadmin.js assets/vditor/index.min.js assets/vditor/index.css assets/vditor/method.min.js
var assetsFS embed.FS
