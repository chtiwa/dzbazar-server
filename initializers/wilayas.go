package initializers

import (
	"embed"
	"log"
)

// static_wilayas.json is no longer read at runtime (see services.GetWilayas,
// which now reads the wilayas DB table seeded by
// migrate/migrations/00019_wilayas_table.sql) — root CLAUDE.md forbids
// hardcoding the wilaya list. The file stays in the repo as the historical
// seed source but is intentionally left out of this embed. Osen/ZR carrier
// municipality/territory reference data is unrelated (per-carrier lookup
// tables, not the general wilaya list) and still loads from JSON below.
//
//go:embed data/osen_municipalities.json data/zr_territories.json

var staticFiles embed.FS

// InitStaticData validates the still-JSON-backed carrier reference data at
// boot so a malformed file fails fast instead of surfacing later as a
// confusing 500. Wilaya data now lives in Postgres and is warmed by
// services.GetWilayas() after migrations run (see main.go init()) — it
// can't be checked here because the wilayas table doesn't exist yet at this
// point in startup.
func InitStaticData() {
	if _, err := GetOsenMunicipalities(); err != nil {
		log.Fatalf("failed to initialize osen municipalities: %v", err)
	}
	if _, err := GetZrTerritories(); err != nil {
		log.Fatalf("failed to initialize zr territories: %v", err)
	}
}
