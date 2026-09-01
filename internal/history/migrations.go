package history

import _ "embed"

const currentSchemaVersion = 1

//go:embed migrations/001_initial.sql
var migrationOne string
