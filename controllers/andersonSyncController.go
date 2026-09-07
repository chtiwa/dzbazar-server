package controllers

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/realtime"
	"github.com/chtiwa/dzbazar-server/utils"
	"github.com/google/uuid"
)

// Ecotrack has no webhook, so status sync is a slow poll — same shape as the
// Osen/ZR sync. Ecotrack's get/orders endpoint has no "give me these tracking
// IDs" batch filter, so each pending order is queried individually by its own
// tracking param instead of paging through the full list.
const (
	andersonSyncInterval  = 15 * time.Minute
	andersonShippedStatus = "Expedié"
	andersonSyncLockKey   = "lock:tick:anderson_sync"
)

type andersonOrderStatusResp struct {
	Data []struct {
		Tracking string `json:"tracking"`
		Status   string `json:"status"`
	} `json:"data"`
}

// StartAndersonStatusSync runs forever, periodically syncing local order
// statuses with Anderson (Ecotrack) for orders shipped but not yet resolved.
func StartAndersonStatusSync() {
	ticker := time.NewTicker(andersonSyncInterval)
	defer ticker.Stop()

	for range ticker.C {
		if !utils.TryAcquireTickLock(andersonSyncLockKey, andersonSyncInterval-time.Minute) {
			continue
		}
		syncAndersonOrderStatuses()
	}
}

func syncAndersonOrderStatuses() {
	var shopIDs []uuid.UUID
	if err := initializers.DB.Model(&models.Order{}).
		Where("is_shipped = ? AND status = ? AND tracking_number <> ''", true, andersonShippedStatus).
		Distinct().
		Pluck("shop_id", &shopIDs).Error; err != nil {
		log.Printf("anderson sync: failed to list pending shops: %v", err)
		return
	}

	for _, shopID := range shopIDs {
		syncShopAndersonOrders(shopID)
	}
}

// mapAndersonStatusToLocal maps an Ecotrack order status to the local status
// it implies. Seeded with the statuses documented by Ecotrack; extend here
// once more real values are observed.
func mapAndersonStatusToLocal(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "livré", "livree", "payé_et_archivé", "paye_et_archive":
		return "Livré"
	case "retour", "retourné", "retournee", "retour_recu", "annulé", "annule":
		return "Retour"
	default:
		return ""
	}
}

func syncShopAndersonOrders(shopID uuid.UUID) {
	integration, err := findAndersonIntegration(shopID)
	if err != nil {
		return
	}

	var orders []models.Order
	if err := initializers.DB.
		Where("shop_id = ? AND is_shipped = ? AND status = ? AND tracking_number <> ''", shopID, true, andersonShippedStatus).
		Find(&orders).Error; err != nil || len(orders) == 0 {
		return
	}

	httpClient := &http.Client{Timeout: 15 * time.Second}

	for i := range orders {
		order := &orders[i]

		reqURL := fmt.Sprintf("%s/api/v1/get/orders?api_token=%s&tracking=%s",
			andersonBaseURL, url.QueryEscape(integration.Token), url.QueryEscape(order.TrackingNumber))

		resp, err := httpClient.Get(reqURL)
		if err != nil {
			log.Printf("anderson sync: shop %s order %s request failed: %v", shopID, order.ID, err)
			continue
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			continue
		}

		var listResp andersonOrderStatusResp
		if err := json.Unmarshal(body, &listResp); err != nil || len(listResp.Data) == 0 {
			continue
		}

		newStatus := mapAndersonStatusToLocal(listResp.Data[0].Status)
		if newStatus == "" {
			continue
		}

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
}
