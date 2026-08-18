package services

import (
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// GetClientDetail loads a single client for the super-admin client-detail
// drill-in: full profile plus a lightweight order history — id/total/status/
// date only, never the item breakdown (that's what the order-detail view is
// for). Read-only, cross-tenant like GetOrderDetail — super-admin scope isn't
// shop-scoped like the tenant admin API.
func GetClientDetail(db *gorm.DB, clientID uuid.UUID) (models.Client, error) {
	var client models.Client
	err := db.
		Preload("Orders", func(tx *gorm.DB) *gorm.DB {
			return tx.Select("id", "client_id", "total_price", "status", "created_at").
				Order("created_at DESC")
		}).
		First(&client, "id = ?", clientID).Error
	return client, err
}

// SetClientBanned flips Client.Banned — the phone-based ban fallback
// mechanism documented on the model — to the given value and returns the
// updated row. Takes the desired end state rather than blindly toggling, so
// a doubled request (double-click, retry) is idempotent instead of flapping
// the flag back and forth.
func SetClientBanned(db *gorm.DB, clientID uuid.UUID, banned bool) (models.Client, error) {
	var client models.Client
	if err := db.First(&client, "id = ?", clientID).Error; err != nil {
		return client, err
	}
	if err := db.Model(&client).Update("banned", banned).Error; err != nil {
		return client, err
	}
	client.Banned = banned
	return client, nil
}
