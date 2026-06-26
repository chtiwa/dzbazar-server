package controllers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// truncateForLog caps a logged response body so a large payload doesn't flood
// the server log; only enough to see the JSON shape is needed.
func truncateForLog(body []byte) string {
	const max = 2000
	if len(body) > max {
		return string(body[:max]) + "...(truncated)"
	}
	return string(body)
}

// ── ZR geo lookup (territories + hubs) ────────────────────────────────────────
// Territory IDs (wilaya + commune) are a static, nationwide catalog — ZR's
// real ID space for them is dumped once from POST /territories/search into
// initializers/data/zr_territories.json (see initializers/zr_geo.go) and
// matched in-memory, the same static-file pattern Osen uses. This replaced an
// earlier live/Redis-cached approach that derived IDs from GET
// /delivery-pricing/rates: that endpoint uses a *different* ID namespace
// (pricing-tier IDs, not territory GUIDs) and also omits Alger's wilaya-level
// row, which caused both the "wilaya Alger not found" error and a recurring
// empty-400 crash (a non-GUID string fed into a Guid-typed field crashes
// ASP.NET's deserializer before validation runs).
//
// Hubs (pickup points) are genuinely per-account, so they're still fetched
// live and cached in Redis with a 24h TTL.

const zrGeoCacheTTL = 24 * time.Hour
const zrGeoMaxPages = 5
const zrGeoPageSize = 1000

// zrHub mirrors the confirmed-live POST /hubs/search item shape. Hubs key off
// wilaya via address.cityTerritoryId (a real territory GUID, same ID space as
// initializers.ZrTerritorySeed.ID) and optionally a finer district/commune via
// address.districtTerritoryId — not a free-text bureau name, so resolution
// matches on territory ID rather than fuzzy name.
type zrHub struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Type          string `json:"type"`
	IsPickupPoint bool   `json:"isPickupPoint"`
	IsVisible     bool   `json:"isVisible"`
	Address       struct {
		City                string `json:"city"`
		CityTerritoryID     string `json:"cityTerritoryId"`
		District            string `json:"district"`
		DistrictTerritoryID string `json:"districtTerritoryId"`
	} `json:"address"`
}

func zrHubsCacheKey(shopID uuid.UUID) string {
	return fmt.Sprintf("zr:geo:hubs:%s", shopID)
}

// loadZrHubs returns the shop's ZR hub list, served from a 24h Redis cache and
// lazily refetched on miss. A stale cached copy is served on fetch error
// rather than failing the caller outright.
func loadZrHubs(shopID uuid.UUID, integration *models.DeliveryCompany) ([]zrHub, error) {
	cacheKey := zrHubsCacheKey(shopID)

	if cached, err := initializers.RClient.Get(initializers.Ctx, cacheKey).Result(); err == nil {
		var hubs []zrHub
		if json.Unmarshal([]byte(cached), &hubs) == nil {
			return hubs, nil
		}
	}

	hubs, err := fetchZrHubs(integration)
	if err != nil {
		if cached, cacheErr := initializers.RClient.Get(initializers.Ctx, cacheKey).Result(); cacheErr == nil {
			var stale []zrHub
			if json.Unmarshal([]byte(cached), &stale) == nil {
				return stale, nil
			}
		}
		return nil, err
	}

	if encoded, err := json.Marshal(hubs); err == nil {
		initializers.RClient.Set(initializers.Ctx, cacheKey, encoded, zrGeoCacheTTL)
	}

	return hubs, nil
}

func fetchZrHubs(integration *models.DeliveryCompany) ([]zrHub, error) {
	httpClient := &http.Client{Timeout: 15 * time.Second}
	all := make([]zrHub, 0, zrGeoPageSize)

	for page := 1; page <= zrGeoMaxPages; page++ {
		// Confirmed live: ZR's search endpoints 500 if optional filter fields
		// are sent as null, and 400 if sent as {} — omitting them entirely is
		// the only shape that works, so the body is just pagination.
		reqBody, _ := json.Marshal(map[string]any{
			"pageSize":   zrGeoPageSize,
			"pageNumber": page,
		})

		req, _ := http.NewRequest("POST", zrBaseURL+"/api/v1/hubs/search", bytes.NewBuffer(reqBody))
		zrAuthHeaders(req, integration)
		req.Header.Set("Content-Type", "application/json")

		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("impossible de joindre ZR Express: %w", err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("ZR Express hubs/search a échoué (%d)", resp.StatusCode)
		}

		var pageResult struct {
			Items      []zrHub `json:"items"`
			TotalCount int     `json:"totalCount"`
		}
		if err := json.Unmarshal(body, &pageResult); err != nil {
			return nil, fmt.Errorf("réponse hubs/search invalide: %w", err)
		}

		if len(pageResult.Items) == 0 {
			log.Printf("zr geo: hubs/search page %d returned 0 parsed rows, raw body: %s", page, truncateForLog(body))
		}

		all = append(all, pageResult.Items...)
		if len(pageResult.Items) < zrGeoPageSize || len(all) >= pageResult.TotalCount {
			break
		}
	}

	return all, nil
}

// findZrWilayaTerritory returns the wilaya-level territory for an Algerian
// wilaya code, sourced from the static zr_territories.json.
func findZrWilayaTerritory(stateCode string) (initializers.ZrTerritorySeed, bool) {
	territories, err := initializers.GetZrTerritories()
	if err != nil {
		return initializers.ZrTerritorySeed{}, false
	}

	// Compare numerically, not as strings: the storefront sends zero-padded
	// codes ("06" for Bejaia) which never equal ZR's unpadded int code (6).
	code, err := strconv.Atoi(strings.TrimSpace(stateCode))
	if err != nil {
		return initializers.ZrTerritorySeed{}, false
	}

	for _, t := range territories {
		if t.Level == "wilaya" && t.Code == code {
			return t, true
		}
	}
	return initializers.ZrTerritorySeed{}, false
}

// findZrCommuneTerritory returns the commune-level territory within a wilaya
// matching cityName: exact match, then contains, then falls back to the
// first commune in that wilaya (same ladder as Osen's findOsenMunicipalityID).
func findZrCommuneTerritory(wilayaID, cityName string) (initializers.ZrTerritorySeed, bool) {
	territories, err := initializers.GetZrTerritories()
	if err != nil {
		return initializers.ZrTerritorySeed{}, false
	}

	var communes []initializers.ZrTerritorySeed
	for _, t := range territories {
		if t.Level == "commune" && t.ParentID == wilayaID {
			communes = append(communes, t)
		}
	}
	if len(communes) == 0 {
		return initializers.ZrTerritorySeed{}, false
	}

	cityLower := strings.ToLower(strings.TrimSpace(cityName))
	if cityLower != "" {
		for _, c := range communes {
			if strings.ToLower(c.Name) == cityLower {
				return c, true
			}
		}
		for _, c := range communes {
			cLower := strings.ToLower(c.Name)
			if strings.Contains(cLower, cityLower) || strings.Contains(cityLower, cLower) {
				return c, true
			}
		}
	}
	return communes[0], true
}

// resolveZrTerritoryID resolves both IDs ZR's create-parcel API requires: the
// wilaya-level territory (deliveryAddress.cityTerritoryId / stateId) and the
// commune-level district territory (deliveryAddress.districtTerritoryId).
// Both are pure in-memory lookups against the static territory list — no
// network call, no per-shop caching needed.
func resolveZrTerritoryID(stateCode, stateName, cityName string) (wilayaID string, districtID string, err error) {
	wilaya, ok := findZrWilayaTerritory(stateCode)
	if !ok {
		return "", "", fmt.Errorf("aucune correspondance pour la wilaya %s", stateName)
	}

	commune, ok := findZrCommuneTerritory(wilaya.ID, cityName)
	if !ok {
		return "", "", fmt.Errorf("commune introuvable pour %s", cityName)
	}

	return wilaya.ID, commune.ID, nil
}

// resolveZrHubID picks a pickup-point hub for the order's wilaya. Hubs stay
// live-fetched (per-account), but the wilaya ID feeding the match is now the
// static territory GUID instead of a live/guessed one.
func resolveZrHubID(shopID uuid.UUID, integration *models.DeliveryCompany, stateCode, stateName, cityName string) (string, error) {
	wilaya, ok := findZrWilayaTerritory(stateCode)
	if !ok {
		return "", fmt.Errorf("wilaya ZR Express introuvable pour %s", stateName)
	}

	hubs, err := loadZrHubs(shopID, integration)
	if err != nil {
		return "", err
	}
	if len(hubs) == 0 {
		return "", fmt.Errorf("aucun point de relais ZR Express disponible")
	}

	cityLower := strings.ToLower(strings.TrimSpace(cityName))
	var fallback string
	for _, h := range hubs {
		if !h.IsPickupPoint || !h.IsVisible {
			continue
		}
		if h.Address.CityTerritoryID != wilaya.ID {
			continue
		}
		if fallback == "" {
			fallback = h.ID
		}
		if cityLower != "" && strings.Contains(strings.ToLower(h.Address.District), cityLower) {
			return h.ID, nil
		}
	}
	if fallback != "" {
		return fallback, nil
	}

	return "", fmt.Errorf("aucun point de relais ZR Express dans la wilaya %s", stateName)
}

// RefreshZrGeo force-refreshes the shop's cached ZR hub list, letting an
// operator pick up a new hub without waiting out the 24h TTL. Territories are
// static (compiled in), so there's nothing to refresh there.
func RefreshZrGeo(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	integration, err := findZrIntegration(shopID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "ZR Express n'est pas connecté à cette boutique"})
		return
	}

	initializers.RClient.Del(initializers.Ctx, zrHubsCacheKey(shopID))

	hubs, err := loadZrHubs(shopID, integration)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": err.Error()})
		return
	}

	territories, _ := initializers.GetZrTerritories()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"hubsCount":        len(hubs),
			"territoriesCount": len(territories),
		},
	})
}

// DebugZrGeo is a diagnostic endpoint: it calls ZR Express's hubs/search,
// parcels/search, and delivery-pricing/rates directly (no caching, no parsing
// into our structs) and returns the raw status + body of each, so the real
// response shape can be inspected from the Network tab when server console
// access isn't available.
func DebugZrGeo(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	integration, err := findZrIntegration(shopID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "ZR Express n'est pas connecté à cette boutique"})
		return
	}

	httpClient := &http.Client{Timeout: 15 * time.Second}
	rawCall := func(method, path string, reqBody []byte) gin.H {
		var bodyReader io.Reader
		if reqBody != nil {
			bodyReader = bytes.NewBuffer(reqBody)
		}
		req, err := http.NewRequest(method, zrBaseURL+path, bodyReader)
		if err != nil {
			return gin.H{"error": err.Error()}
		}
		zrAuthHeaders(req, integration)
		if reqBody != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			return gin.H{"error": err.Error()}
		}
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)

		var parsed any
		if json.Unmarshal(respBody, &parsed) == nil {
			return gin.H{"status": resp.StatusCode, "body": parsed}
		}
		return gin.H{"status": resp.StatusCode, "rawBody": string(respBody)}
	}

	// Confirmed live: ZR's generic search endpoints 500 on null optional
	// fields and 400 on {} for advancedFilter — omitting them entirely
	// (pagination only) is the shape that works.
	minimalSearchBody, _ := json.Marshal(map[string]any{"pageSize": 5, "pageNumber": 1})

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"rates":   rawCall("GET", "/api/v1/delivery-pricing/rates", nil),
			"hubs":    rawCall("POST", "/api/v1/hubs/search", minimalSearchBody),
			"parcels": rawCall("POST", "/api/v1/parcels/search", minimalSearchBody),
		},
	})
}
