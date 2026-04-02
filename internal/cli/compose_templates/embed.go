// Package compose_templates embeds Docker Compose files for
// writing to the project directory during `mdemg init`.
package compose_templates

import "embed"

// FS contains all Docker Compose template files embedded at compile time.
//
//go:embed *.yml
var FS embed.FS
