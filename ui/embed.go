package ui

import "embed"

// Embedded contains the web app's pages.
//
//go:embed index.html
var Embedded embed.FS
