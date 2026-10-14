package ui

import "embed"

// Embedded contains the web app's pages.
//
//go:embed index.html login.html
var Embedded embed.FS

// OctopusLogo is the Octopus Deploy logo on the login page.
//
//go:embed octopus-deploy-logo.png
var OctopusLogo []byte
