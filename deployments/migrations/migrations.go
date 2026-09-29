package migrations

import "embed"

// FS embeds all SQL migration files in this directory for use with the migration engine.
//
//go:embed *.sql
var FS embed.FS
