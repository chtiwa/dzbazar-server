package migrate

import (
	"testing"

	"github.com/pressly/goose/v3"
)

// Same collection step goose.Up runs at boot; fails on duplicate versions
// here instead of crash-looping prod.
func TestMigrationsNoDuplicateVersions(t *testing.T) {
	goose.SetBaseFS(migrationsFS)
	if _, err := goose.CollectMigrations("migrations", 0, goose.MaxVersion); err != nil {
		t.Fatal(err)
	}
}
