package migrate

import (
	"database/sql"
	"embed"
	"log"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate runs versioned SQL migrations from migrations/ via goose. Replaces
// the old GORM AutoMigrate-every-boot approach; 00001_baseline.sql is a
// pg_dump of the schema AutoMigrate had already built, so new schema changes
// from here on are goose migrations, not ad-hoc ALTERs or model-tag edits.
func Migrate() {
	db, err := initializers.DB.DB()
	if err != nil {
		log.Fatalf("Migrate: could not get underlying sql.DB: %v", err)
	}

	if err := goose.SetDialect("postgres"); err != nil {
		log.Fatalf("Migrate: %v", err)
	}
	goose.SetBaseFS(migrationsFS)

	baselineExistingDB(db)

	if err := goose.Up(db, "migrations"); err != nil {
		log.Fatalf("Migrate: goose up failed: %v", err)
	}
	log.Println("🚀 Database migrations up to date")
}

// baselineExistingDB marks 00001_baseline.sql as already applied, without
// running it, when goose hasn't tracked this DB yet but the schema clearly
// already exists (the live Railway DB, built over time by the old
// AutoMigrate). A genuinely fresh DB has no `users` table yet, so goose runs
// 00001 normally there.
func baselineExistingDB(db *sql.DB) {
	var tracked bool
	if err := db.QueryRow(`SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'goose_db_version')`).Scan(&tracked); err != nil {
		log.Fatalf("Migrate: checking goose_db_version: %v", err)
	}
	if tracked {
		return
	}

	var preExisting bool
	if err := db.QueryRow(`SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'users')`).Scan(&preExisting); err != nil {
		log.Fatalf("Migrate: checking pre-existing schema: %v", err)
	}
	if !preExisting {
		return
	}

	if _, err := db.Exec(`CREATE TABLE goose_db_version (
		id serial PRIMARY KEY,
		version_id bigint NOT NULL,
		is_applied boolean NOT NULL,
		tstamp timestamp DEFAULT now()
	)`); err != nil {
		log.Fatalf("Migrate: creating goose_db_version: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO goose_db_version (version_id, is_applied) VALUES (0, true), (1, true)`); err != nil {
		log.Fatalf("Migrate: baselining goose_db_version: %v", err)
	}
	log.Println("🔖 Baselined existing schema at migration 00001 (not re-run)")
}
