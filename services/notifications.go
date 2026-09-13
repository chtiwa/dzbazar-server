package services

import (
	"log"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/google/uuid"
)

// CreateNotificationsForShop inserts one persistent Notification row per
// distinct shop member, for either of the two known event types
// (order_created, abandoned_lead_created). Runs unconditionally — unlike the
// merchant email, it is not gated by environment or staff-order status.
// Best-effort: logs and returns on error, never breaks order/lead creation.
func CreateNotificationsForShop(shopID uuid.UUID, notifType string, referenceID uuid.UUID, payload map[string]any) {
	var members []models.ShopMember
	if err := initializers.DB.Preload("User").Where("shop_id = ?", shopID).Find(&members).Error; err != nil {
		log.Printf("notifications: failed to load shop members shop=%s: %v", shopID, err)
		return
	}

	seen := make(map[uuid.UUID]struct{})
	rows := make([]models.Notification, 0, len(members))
	for _, m := range members {
		if _, ok := seen[m.UserID]; ok {
			continue
		}
		seen[m.UserID] = struct{}{}
		rows = append(rows, models.Notification{
			RecipientUserID: m.UserID,
			ShopID:          shopID,
			Type:            notifType,
			ReferenceID:     referenceID,
			Payload:         payload,
		})
	}

	if len(rows) == 0 {
		return
	}

	if err := initializers.DB.Create(&rows).Error; err != nil {
		log.Printf("notifications: failed to insert rows shop=%s type=%s: %v", shopID, notifType, err)
	}
}
