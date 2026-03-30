package api

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed ui/*
var uiFS embed.FS

// uiHandler returns an http.Handler that serves the embedded UI files.
// fs.Sub strips the "ui/" prefix so /ui/index.html maps to ui/index.html
// in the embedded filesystem, not ui/ui/index.html.
func uiHandler() http.Handler {
	sub, _ := fs.Sub(uiFS, "ui")
	return http.FileServer(http.FS(sub))
}
