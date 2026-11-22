// Package whatnow holds the pages of the dashboard. The Python skill reads them from skill/, the desktop app
// (app/) embeds them from here, so both always show the same pages.
package whatnow

import "embed"

//go:embed skill/template.html skill/template-index.html skill/side.js
var Web embed.FS
