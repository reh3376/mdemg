// Package migrations embeds all Cypher migration files for the MDEMG schema.
package migrations

import "embed"

// FS contains all *.cypher migration files embedded at compile time.
//
//go:embed *.cypher
var FS embed.FS
