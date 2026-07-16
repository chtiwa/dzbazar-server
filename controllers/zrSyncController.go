package controllers

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/realtime"
	"github.com/chtiwa/dzbazar-server/utils"
	"github.com/google/uuid"
)

// ── ZR status sync ────────────────────────────────────────────────────────────
// Same structure as Osen's sync (osenSyncController.go): ZR has no documented
// webhook either, so the only way to learn a parcel was delivered/cancelled is
// to poll. Matching is done on externalId (the local order UUID, set at ship
// time in shipOrderToZr) rather than tracking number, since externalId is
// guaranteed to round-trip even if ZR's tracking field name turns out to
// differ from what shipOrderToZr guessed.

const (
	zrSyncInterval  = 15 * time.Minute
	zrSyncPageSize  = 100
	zrSyncMaxPages  = 5
	zrShippedStatus = "Expedié"
	zrSyncLockKey   = "lock:tick:zr_sync"
)

type zrParcelListItem struct {
	ExternalID string `json:"externalId"`
	Status     string `json:"status"`
}

// zrParcelsListResp mirrors the same {items, totalCount, ...} pagination
// envelope confirmed live on hubs/search — ZR's search endpoints appear to
// share one generic paged-result wrapper.
type zrParcelsListResp struct {
	Items []zrParcelListItem `json:"items"`
}

// StartZrStatusSync runs forever, periodically syncing local order statuses
// with ZR Express for orders that were shipped but not yet resolved
// (delivered/cancelled). Intended to be run in its own goroutine.
func StartZrStatusSync() {
	ticker := time.NewTicker(zrSyncInterval)
	defer ticker.Stop()

	for range ticker.C {
		if !utils.TryAcquireTickLock(zrSyncLockKey, zrSyncInterval-time.Minute) {
			continue
		}
		syncZrOrderStatuses()
	}
}

// syncZrOrderStatuses finds shops with orders pending a ZR status update and
// syncs each one. It does nothing if no order qualifies.
func syncZrOrderStatuses() {
	var shopIDs []uuid.UUID
	if err := initializers.DB.Model(&models.Order{}).
		Where("is_shipped = ? AND status = ? AND tracking_number <> ''", true, zrShippedStatus).
		Distinct().
		Pluck("shop_id", &shopIDs).Error; err != nil {
		log.Printf("zr sync: failed to list pending shops: %v", err)
		return
	}

	for _, shopID := range shopIDs {
		syncShopZrOrders(shopID)
	}
}

// mapZrStatusToLocal maps a ZR parcel status to the local order status it
// implies. Returns "" if the ZR status doesn't map to a local change. Seeded
// with a best guess — ZR's real status enum wasn't visible in public docs, so
// this is the single place to correct once real responses are observed.
func mapZrStatusToLocal(zrStatus string) string {
	switch strings.ToLower(strings.TrimSpace(zrStatus)) {
	case "delivered":
		return "Livré"
	case "cancelled", "canceled", "returned":
		return "Annulé"
	default:
		return ""
	}
}

// syncShopZrOrders pages through a single shop's ZR parcels and updates the
// status of any local order whose externalId matches a resolved ZR parcel.
func syncShopZrOrders(shopID uuid.UUID) {
	integration, err := findZrIntegration(shopID)
	if err != nil {
		return
	}

	var orders []models.Order
	if err := initializers.DB.
		Where("shop_id = ? AND is_shipped = ? AND status = ? AND tracking_number <> ''", shopID, true, zrShippedStatus).
		Find(&orders).Error; err != nil || len(orders) == 0 {
		return
	}

	pending := make(map[string]*models.Order, len(orders))
	for i := range orders {
		pending[orders[i].ID.String()] = &orders[i]
	}

	httpClient := &http.Client{Timeout: 15 * time.Second}

	for page := 1; page <= zrSyncMaxPages && len(pending) > 0; page++ {
		// Confirmed live: omit optional search fields entirely — sending them
		// as null 500s and as {} 400s on ZR's generic search endpoints.
		reqBody, _ := json.Marshal(map[string]any{
			"pageSize":   zrSyncPageSize,
			"pageNumber": page,
		})

		req, _ := http.NewRequest("POST", zrBaseURL+"/api/v1/parcels/search", bytes.NewBuffer(reqBody))
		zrAuthHeaders(req, integration)
		req.Header.Set("Content-Type", "application/json")

		resp, err := httpClient.Do(req)
		if err != nil {
			log.Printf("zr sync: shop %s request failed: %v", shopID, err)
			return
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return
		}

		var listResp zrParcelsListResp
		if err := json.Unmarshal(body, &listResp); err != nil || len(listResp.Items) == 0 {
			return
		}

		for _, parcel := range listResp.Items {
			order, ok := pending[parcel.ExternalID]
			if !ok {
				continue
			}

			newStatus := mapZrStatusToLocal(parcel.Status)
			if newStatus != "" {
				if err := initializers.DB.Model(&models.Order{}).
					Where("id = ?", order.ID).
					Update("status", newStatus).Error; err == nil {
					invalidateOrdersListCache(shopID)
					realtime.Broadcast <- realtime.Message{
						Event:  "order_status_synced",
						ShopID: shopID.String(),
						Data: map[string]any{
							"orderId": order.ID,
							"status":  newStatus,
						},
					}
				}
			}

			delete(pending, parcel.ExternalID)
		}

		if len(listResp.Items) < zrSyncPageSize {
			return
		}
	}
}
