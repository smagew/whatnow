// Package whatnow holds the pages of the dashboard: the program (app/) embeds them from here. skill/demo holds
// pages with examples of the interface to choose from; they are served under /demo/.
package whatnow

import "embed"

//go:embed skill/template.html skill/template-index.html skill/side.js skill/term.js skill/vendor skill/demo
var Web embed.FS
