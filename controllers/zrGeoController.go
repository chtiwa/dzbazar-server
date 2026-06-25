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
// Unlike Osen, whose province/municipality IDs are static Algerian wilaya codes
// bundled as an embedded JSON file (see initializers/osen_geo.go), ZR's
// territory/hub IDs are its own account-specific UUIDs, so they're fetched live
// from ZR's API and cached per shop in Redis (24h TTL) instead.

const zrGeoCacheTTL = 24 * time.Hour
const zrGeoMaxPages = 5
const zrGeoPageSize = 1000

type zrTerritory struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Code     string `json:"code"`
	ParentID string `json:"parentId"`
	Level    string `json:"level"` // e.g. "wilaya" | "commune"
}

// zrHub mirrors the confirmed-live POST /hubs/search item shape. Hubs key off
// wilaya via address.cityTerritoryId (which matches the wilaya-level
// toTerritoryId from GET /delivery-pricing/rates) and optionally a finer
// district/commune via address.districtTerritoryId — not a free-text bureau
// name, so resolution matches on territory ID rather than fuzzy name.
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

func zrTerritoriesCacheKey(shopID uuid.UUID) string {
	return fmt.Sprintf("zr:geo:territories:%s", shopID)
}

func zrHubsCacheKey(shopID uuid.UUID) string {
	return fmt.Sprintf("zr:geo:hubs:%s", shopID)
}

// loadZrTerritories returns the shop's ZR territory list, served from a 24h
// Redis cache and lazily refetched on miss. A stale cached copy is served on
// fetch error rather than failing the caller outright.
func loadZrTerritories(shopID uuid.UUID, integration *models.DeliveryCompany) ([]zrTerritory, error) {
	cacheKey := zrTerritoriesCacheKey(shopID)

	if cached, err := initializers.RClient.Get(initializers.Ctx, cacheKey).Result(); err == nil {
		var territories []zrTerritory
		if json.Unmarshal([]byte(cached), &territories) == nil {
			return territories, nil
		}
	}

	territories, err := fetchZrTerritories(integration)
	if err != nil {
		if cached, cacheErr := initializers.RClient.Get(initializers.Ctx, cacheKey).Result(); cacheErr == nil {
			var stale []zrTerritory
			if json.Unmarshal([]byte(cached), &stale) == nil {
				return stale, nil
			}
		}
		return nil, err
	}

	if encoded, err := json.Marshal(territories); err == nil {
		initializers.RClient.Set(initializers.Ctx, cacheKey, encoded, zrGeoCacheTTL)
	}

	return territories, nil
}

// zrRateTerritory is the destination-territory shape embedded in each entry
// of GET /delivery-pricing/rates. Confirmed live: ToTerritoryCode is present
// (and matches the Algerian wilaya code 1-58) on wilaya-level entries, and
// absent on commune-level entries (ZR only has commune granularity within
// Alger wilaya — everywhere else is wilaya-level).
type zrRateTerritory struct {
	ToTerritoryID    string `json:"toTerritoryId"`
	ToTerritoryName  string `json:"toTerritoryName"`
	ToTerritoryLevel string `json:"toTerritoryLevel"` // "wilaya" | "commune" | "Unknown"
	ToTerritoryCode  *int   `json:"toTerritoryCode,omitempty"`
}

type zrRatesResp struct {
	Rates []zrRateTerritory `json:"rates"`
}

// fetchZrTerritories derives the territory list from GET /delivery-pricing/rates
// rather than POST /territories/search: the rates endpoint is confirmed to
// return real data (status 200, ~114 entries) and already carries everything
// needed to resolve a wilaya/commune to a ZR territory UUID, whereas
// territories/search's actual response envelope couldn't be confirmed.
func fetchZrTerritories(integration *models.DeliveryCompany) ([]zrTerritory, error) {
	req, _ := http.NewRequest("GET", zrBaseURL+"/api/v1/delivery-pricing/rates", nil)
	zrAuthHeaders(req, integration)

	httpClient := &http.Client{Timeout: 15 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("impossible de joindre ZR Express: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ZR Express delivery-pricing/rates a échoué (%d)", resp.StatusCode)
	}

	var ratesResp zrRatesResp
	if err := json.Unmarshal(body, &ratesResp); err != nil {
		return nil, fmt.Errorf("réponse delivery-pricing/rates invalide: %w", err)
	}

	territories := make([]zrTerritory, 0, len(ratesResp.Rates))
	for _, r := range ratesResp.Rates {
		if r.ToTerritoryID == "" || r.ToTerritoryLevel == "Unknown" {
			continue
		}
		code := ""
		if r.ToTerritoryCode != nil {
			code = strconv.Itoa(*r.ToTerritoryCode)
		}
		territories = append(territories, zrTerritory{
			ID:    r.ToTerritoryID,
			Name:  r.ToTerritoryName,
			Code:  code,
			Level: r.ToTerritoryLevel,
		})
	}

	if len(territories) == 0 {
		log.Printf("zr geo: delivery-pricing/rates parsed to 0 territories, raw body: %s", truncateForLog(body))
	}

	return territories, nil
}

// loadZrHubs returns the shop's ZR hub list, same caching strategy as territories.
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

// findZrWilayaTerritory returns the wilaya-level territory matching a state
// code/name, ignoring commune-level entries.
func findZrWilayaTerritory(territories []zrTerritory, stateCode, stateName string) (zrTerritory, bool) {
	stateLower := strings.ToLower(strings.TrimSpace(stateName))
	for _, t := range territories {
		if t.Level != "wilaya" {
			continue
		}
		if t.Code == stateCode || (stateLower != "" && strings.ToLower(t.Name) == stateLower) {
			return t, true
		}
	}
	return zrTerritory{}, false
}

// zrDistrict is a commune-level territory used only to resolve
// deliveryAddress.districtTerritoryId, which ZR requires on every parcel
// (validated: "DistrictTerritoryId is required... must be a valid GUID").
// GET /delivery-pricing/rates only price-aggregates communes within the Alger
// wilaya, so it can't supply this for the rest of the country — the full
// per-commune hierarchy has to come from POST /territories/search instead.
// Its exact response field names aren't confirmed from public docs, so
// parsing tries a few plausible candidates per item rather than one guess.
type zrDistrict struct {
	ID   string
	Name string
}

func zrDistrictsCacheKey(shopID uuid.UUID) string {
	return fmt.Sprintf("zr:geo:districts:%s", shopID)
}

// loadZrDistricts returns the shop's ZR commune list, same caching strategy
// as territories/hubs.
func loadZrDistricts(shopID uuid.UUID, integration *models.DeliveryCompany) ([]zrDistrict, error) {
	cacheKey := zrDistrictsCacheKey(shopID)

	if cached, err := initializers.RClient.Get(initializers.Ctx, cacheKey).Result(); err == nil {
		var districts []zrDistrict
		if json.Unmarshal([]byte(cached), &districts) == nil {
			return districts, nil
		}
	}

	districts, err := fetchZrDistricts(integration)
	if err != nil {
		if cached, cacheErr := initializers.RClient.Get(initializers.Ctx, cacheKey).Result(); cacheErr == nil {
			var stale []zrDistrict
			if json.Unmarshal([]byte(cached), &stale) == nil {
				return stale, nil
			}
		}
		return nil, err
	}

	if encoded, err := json.Marshal(districts); err == nil {
		initializers.RClient.Set(initializers.Ctx, cacheKey, encoded, zrGeoCacheTTL)
	}

	return districts, nil
}

// zrDistrictIDKeys/zrDistrictNameKeys are tried in order against each raw
// territories/search item, since the exact field names aren't confirmed.
var zrDistrictIDKeys = []string{"id", "territoryId"}
var zrDistrictNameKeys = []string{"name", "territoryName", "label", "title"}

func fetchZrDistricts(integration *models.DeliveryCompany) ([]zrDistrict, error) {
	httpClient := &http.Client{Timeout: 15 * time.Second}
	all := make([]zrDistrict, 0, zrGeoPageSize)

	for page := 1; page <= zrGeoMaxPages; page++ {
		reqBody, _ := json.Marshal(map[string]any{"pageSize": zrGeoPageSize, "pageNumber": page})

		req, _ := http.NewRequest("POST", zrBaseURL+"/api/v1/territories/search", bytes.NewBuffer(reqBody))
		zrAuthHeaders(req, integration)
		req.Header.Set("Content-Type", "application/json")

		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("impossible de joindre ZR Express: %w", err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("ZR Express territories/search a échoué (%d)", resp.StatusCode)
		}

		var pageResult struct {
			Items      []map[string]any `json:"items"`
			TotalCount int              `json:"totalCount"`
		}
		if err := json.Unmarshal(body, &pageResult); err != nil {
			return nil, fmt.Errorf("réponse territories/search invalide: %w", err)
		}

		parsed := 0
		for _, item := range pageResult.Items {
			var id, name string
			for _, k := range zrDistrictIDKeys {
				if v, ok := item[k].(string); ok && v != "" {
					id = v
					break
				}
			}
			for _, k := range zrDistrictNameKeys {
				if v, ok := item[k].(string); ok && v != "" {
					name = v
					break
				}
			}
			if id != "" && name != "" {
				all = append(all, zrDistrict{ID: id, Name: name})
				parsed++
			}
		}

		if len(pageResult.Items) == 0 || parsed == 0 {
			log.Printf("zr geo: territories/search page %d returned 0 usable rows, raw body: %s", page, truncateForLog(body))
		}

		if len(pageResult.Items) < zrGeoPageSize || (pageResult.TotalCount > 0 && len(all) >= pageResult.TotalCount) {
			break
		}
	}

	return all, nil
}

// resolveZrWilayaID finds the wilaya-level territory ID for a state, trying
// three sources in order: the proven-good rates data (works for every wilaya
// except Alger, which rates breaks straight into 58 communes with no wilaya
// aggregate row), the full territories/search-sourced list, and finally the
// hubs list (every hub's address.cityTerritoryId is a real wilaya-level ID,
// confirmed live — e.g. the Birkhadem hub reports city "Alger" for exactly
// this case).
func resolveZrWilayaID(shopID uuid.UUID, integration *models.DeliveryCompany, stateCode, stateName string) (string, error) {
	territories, err := loadZrTerritories(shopID, integration)
	if err != nil {
		return "", err
	}
	if wt, ok := findZrWilayaTerritory(territories, stateCode, stateName); ok {
		return wt.ID, nil
	}

	stateLower := strings.ToLower(strings.TrimSpace(stateName))
	if stateLower != "" {
		if districts, err := loadZrDistricts(shopID, integration); err == nil {
			for _, d := range districts {
				if strings.ToLower(d.Name) == stateLower {
					return d.ID, nil
				}
			}
		}

		if hubs, err := loadZrHubs(shopID, integration); err == nil {
			for _, h := range hubs {
				if strings.ToLower(h.Address.City) == stateLower && h.Address.CityTerritoryID != "" {
					return h.Address.CityTerritoryID, nil
				}
			}
		}
	}

	return "", fmt.Errorf("aucune correspondance pour la wilaya %s", stateName)
}

// resolveZrTerritoryID resolves both IDs ZR's create-parcel API requires: the
// wilaya-level territory (deliveryAddress.cityTerritoryId / stateId) and the
// commune-level district territory (deliveryAddress.districtTerritoryId).
func resolveZrTerritoryID(shopID uuid.UUID, integration *models.DeliveryCompany, stateCode, stateName, cityName string) (wilayaID string, districtID string, err error) {
	wt, err := resolveZrWilayaID(shopID, integration, stateCode, stateName)
	if err != nil {
		return "", "", err
	}

	districts, err := loadZrDistricts(shopID, integration)
	if err != nil {
		return "", "", fmt.Errorf("communes ZR Express indisponibles: %w", err)
	}
	if len(districts) == 0 {
		return "", "", fmt.Errorf("aucune commune ZR Express disponible")
	}

	cityLower := strings.ToLower(strings.TrimSpace(cityName))
	if cityLower != "" {
		for _, d := range districts {
			if strings.ToLower(d.Name) == cityLower {
				return wt, d.ID, nil
			}
		}
		for _, d := range districts {
			dLower := strings.ToLower(d.Name)
			if strings.Contains(dLower, cityLower) || strings.Contains(cityLower, dLower) {
				return wt, d.ID, nil
			}
		}
	}

	return "", "", fmt.Errorf("commune introuvable pour %s", cityName)
}

// resolveZrHubID picks a pickup-point hub for the order's wilaya. ZR hubs are
// organized by wilaya (address.cityTerritoryId matches the wilaya-level
// territory ID), with an optional finer district within it — there's no
// free-text bureau name to match against (Client.StopdeskPoint belongs to a
// different carrier's bureau list), so this matches on territory ID instead,
// preferring a hub whose district name contains cityName when available.
func resolveZrHubID(shopID uuid.UUID, integration *models.DeliveryCompany, stateCode, stateName, cityName string) (string, error) {
	wilayaID, err := resolveZrWilayaID(shopID, integration, stateCode, stateName)
	if err != nil {
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
		if h.Address.CityTerritoryID != wilayaID {
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

// RefreshZrGeo force-refreshes the shop's cached ZR territory/hub lists,
// letting an operator pick up a new hub without waiting out the 24h TTL.
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

	initializers.RClient.Del(initializers.Ctx,
		zrTerritoriesCacheKey(shopID), zrHubsCacheKey(shopID), zrDistrictsCacheKey(shopID))

	territories, err := loadZrTerritories(shopID, integration)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": err.Error()})
		return
	}
	hubs, err := loadZrHubs(shopID, integration)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": err.Error()})
		return
	}
	districts, err := loadZrDistricts(shopID, integration)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"territoriesCount": len(territories),
			"hubsCount":        len(hubs),
			"districtsCount":   len(districts),
		},
	})
}

// DebugZrGeo is a TEMPORARY diagnostic endpoint: it calls ZR Express's
// territories/search, hubs/search, and delivery-pricing/rates directly
// (no caching, no parsing into our structs) and returns the raw status +
// body of each, so the real response shape can be inspected from the
// Network tab when server console access isn't available. Remove once
// zrTerritory/zrHub field mapping is confirmed against a live account.
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
			"rates":       rawCall("GET", "/api/v1/delivery-pricing/rates", nil),
			"hubs":        rawCall("POST", "/api/v1/hubs/search", minimalSearchBody),
			"parcels":     rawCall("POST", "/api/v1/parcels/search", minimalSearchBody),
			"territories": rawCall("POST", "/api/v1/territories/search", minimalSearchBody),
		},
	})
}
