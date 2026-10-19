// Package web embeds the static single-page UI.
package web

import "embed"

//go:embed index.html app.css app.js manifest.webmanifest icon.svg icon-192.png icon-512.png apple-touch-icon.png favicon-32.png
var FS embed.FS
