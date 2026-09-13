package models

import (
	"time"

	"github.com/google/uuid"
)

// Notification is a persistent, cross-shop admin notification (order-created,
// abandoned-lead-created). Deliberately not embedding BaseModel: rows are
// immutable except ReadAt, nothing ever un-deletes or soft-deletes one.
type Notification struct {
	ID              uuid.UUID      `gorm:"type:uuid;default:uuid_generate_v4();primaryKey" json:"id"`
	RecipientUserID uuid.UUID      `gorm:"not null;index" json:"recipientUserId"`
	ShopID          uuid.UUID      `gorm:"not null" json:"shopId"`
	Type            string         `gorm:"not null" json:"type"`
	ReferenceID     uuid.UUID      `json:"referenceId"`
	Payload         map[string]any `gorm:"serializer:json" json:"payload"`
	ReadAt          *time.Time     `json:"readAt"`
	CreatedAt       time.Time      `json:"createdAt"`
	Shop            Shop           `gorm:"foreignKey:ShopID" json:"shop,omitempty"`
}
