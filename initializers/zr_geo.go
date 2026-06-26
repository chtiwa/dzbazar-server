package initializers

import (
	"encoding/json"
	"fmt"
	"sync"
)

// ZrTerritorySeed mirrors one raw item from ZR Express's real
// POST /api/v1/territories/search endpoint (data/zr_territories.json is an
// untouched dump of it — 1585 rows, every Algerian wilaya + commune). This is
// the canonical territory ID space: cityTerritoryId/districtTerritoryId/
// stateId on a parcel must be one of these GUIDs, NOT the toTerritoryId from
// GET /delivery-pricing/rates, which is a different (pricing-tier) ID
// namespace and also omits Alger's wilaya-level row entirely.
type ZrTerritorySeed struct {
	ID         string `json:"id"`
	Code       int    `json:"code"`
	Level      string `json:"level"` // "wilaya" | "commune"
	Name       string `json:"name"`
	NameArabic string `json:"nameArabic"`
	ParentID   string `json:"parentId"` // wilaya ID, for commune-level rows
	PostalCode string `json:"postalCode"`
	Delivery   struct {
		CanSend         bool `json:"canSend"`
		HasHomeDelivery bool `json:"hasHomeDelivery"`
		HasPickupPoint  bool `json:"hasPickupPoint"`
	} `json:"delivery"`
}

var loadZrTerritoriesOnce = sync.OnceValues(func() ([]ZrTerritorySeed, error) {
	data, err := staticFiles.ReadFile("data/zr_territories.json")
	if err != nil {
		return nil, fmt.Errorf("failed to read static zr territories json: %w", err)
	}

	var territories []ZrTerritorySeed
	if err := json.Unmarshal(data, &territories); err != nil {
		return nil, fmt.Errorf("failed to unmarshal static zr territories json: %w", err)
	}

	return territories, nil
})

func GetZrTerritories() ([]ZrTerritorySeed, error) {
	return loadZrTerritoriesOnce()
}
