package history

import _ "embed"

const currentSchemaVersion = 2

//go:embed migrations/001_initial.sql
var migrationOne string

//go:embed migrations/002_pi_session_provenance.sql
var migrationTwo string

type schemaMigration struct {
	version int
	sql     string
}

var schemaMigrations = []schemaMigration{
	{version: 1, sql: migrationOne},
	{version: 2, sql: migrationTwo},
}
