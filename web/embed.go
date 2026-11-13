//go:build !dev

package webspa

import "embed"

//go:embed all:build/spa
var SPA embed.FS
