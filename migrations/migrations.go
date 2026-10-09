// Package migrations embeds the SQL migrations of the database schema, applied
// by goose through `magpie migrate`. Migrations are never edited once merged:
// a change is a new numbered file with its Up and Down parts.
package migrations

import "embed"

// FS holds the migration files.
//
//go:embed *.sql
var FS embed.FS
