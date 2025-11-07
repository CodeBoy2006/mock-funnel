package web

import "embed"

// Assets 嵌入 web 面板的所有静态资源与模板。
//go:embed templates/* static/*
var Assets embed.FS