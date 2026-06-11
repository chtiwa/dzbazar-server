package controllers

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/realtime"
	"github.com/google/uuid"
)

// ── Osen status sync ──────────────────────────────────────────────────────────
// Osen Express has no webhook for order status updates, so the only way to learn
// that an order was delivered or cancelled is to poll. To keep this cheap:
//   - The sync only runs on a slow ticker (every osenSyncInterval).
//   - It first checks (single indexed query) whether any shop even has orders
//     awaiting a status update; if none do, it skips the tick entirely.
//   - For shops that do, it pages through that shop's Osen orders (capped) and
//     only updates rows whose tracking ID matches one we're waiting on.
//   - Updates are pushed over the existing WebSocket hub so connected dashboards
//     refresh automatically, instead of clients polling the API.

const (
	osenSyncInterval  = 15 * time.Minute
	osenSyncPageSize  = 100
	osenSyncMaxPages  = 5
	osenShippedStatus = "Expedié"
)

type osenOrderListItem struct {
	TrackingID string `json:"trackingId"`
	Status     string `json:"status"`
}

type osenOrdersListResp struct {
	Data []osenOrderListItem `json:"data"`
}

// StartOsenStatusSync runs forever, periodically syncing local order statuses
// with Osen Express for orders that were shipped but not yet resolved
// (delivered/cancelled). Intended to be run in its own goroutine.
func StartOsenStatusSync() {
	ticker := time.NewTicker(osenSyncInterval)
	defer ticker.Stop()

	for range ticker.C {
		syncOsenOrderStatuses()
	}
}

// syncOsenOrderStatuses finds shops with orders pending an Osen status update
// and syncs each one. It does nothing if no order qualifies.
func syncOsenOrderStatuses() {
	var shopIDs []uuid.UUID
	if err := initializers.DB.Model(&models.Order{}).
		Where("is_shipped = ? AND status = ? AND tracking_number <> ''", true, osenShippedStatus).
		Distinct().
		Pluck("shop_id", &shopIDs).Error; err != nil {
		log.Printf("osen sync: failed to list pending shops: %v", err)
		return
	}

	for _, shopID := range shopIDs {
		syncShopOsenOrders(shopID)
	}
}

// mapOsenStatusToLocal maps an Osen order status to the local order status it
// implies. Returns "" if the Osen status doesn't map to a local change.
func mapOsenStatusToLocal(osenStatus string) string {
	switch osenStatus {
	case "DELIVERED":
		return "Livré"
	case "CANCELLED":
		return "Annulé"
	default:
		return ""
	}
}

// syncShopOsenOrders pages through a single shop's Osen orders and updates the
// status of any local order whose tracking number matches a resolved Osen order.
func syncShopOsenOrders(shopID uuid.UUID) {
	integration, err := findOsenIntegration(shopID)
	if err != nil {
		return
	}

	var orders []models.Order
	if err := initializers.DB.
		Where("shop_id = ? AND is_shipped = ? AND status = ? AND tracking_number <> ''", shopID, true, osenShippedStatus).
		Find(&orders).Error; err != nil || len(orders) == 0 {
		return
	}

	pending := make(map[string]*models.Order, len(orders))
	for i := range orders {
		pending[orders[i].TrackingNumber] = &orders[i]
	}

	httpClient := &http.Client{Timeout: 15 * time.Second}

	for page := 0; page < osenSyncMaxPages && len(pending) > 0; page++ {
		req, _ := http.NewRequest("GET",
			fmt.Sprintf("%s/v1/orders?skip=%d&take=%d", osenBaseURL, page*osenSyncPageSize, osenSyncPageSize), nil)
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(integration.Token))

		resp, err := httpClient.Do(req)
		if err != nil {
			log.Printf("osen sync: shop %s request failed: %v", shopID, err)
			return
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return
		}

		var listResp osenOrdersListResp
		if err := json.Unmarshal(body, &listResp); err != nil || len(listResp.Data) == 0 {
			return
		}

		for _, osenOrder := range listResp.Data {
			order, ok := pending[osenOrder.TrackingID]
			if !ok {
				continue
			}

			newStatus := mapOsenStatusToLocal(osenOrder.Status)
			if newStatus != "" {
				if err := initializers.DB.Model(&models.Order{}).
					Where("id = ?", order.ID).
					Update("status", newStatus).Error; err == nil {
					realtime.Broadcast <- realtime.Message{
						Event: "order_status_synced",
						Data: map[string]any{
							"orderId": order.ID,
							"shopId":  shopID,
							"status":  newStatus,
						},
					}
				}
			}

			delete(pending, osenOrder.TrackingID)
		}

		if len(listResp.Data) < osenSyncPageSize {
			return
		}
	}
}
