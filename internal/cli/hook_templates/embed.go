// Package hook_templates embeds Claude Code hook scripts for installation
// via `mdemg hooks install --type claude` and `mdemg init`.
package hook_templates

import "embed"

// FS contains all hook template files embedded at compile time.
//
//go:embed *.sh *.ps1 *.py
var FS embed.FS
