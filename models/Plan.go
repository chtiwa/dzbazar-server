package models

import (
	"time"

	"github.com/google/uuid"
)

type Plan struct {
	BaseModel
	Name     string  `gorm:"not null;uniqueIndex" json:"name"`
	Price    float64 `gorm:"not null;default:0" json:"price"`
	IsActive bool    `gorm:"default:true" json:"isActive"`

	// ---- Caps (-1 = unlimited) ----
	MaxShops        int `gorm:"not null;default:1" json:"maxShops"`
	MaxProducts     int `gorm:"not null;default:-1" json:"maxProducts"`
	MaxOrders       int `gorm:"not null;default:-1" json:"maxOrders"`
	MaxLandingPages int `gorm:"not null;default:-1" json:"maxLandingPages"`
	MaxUsers        int `gorm:"not null;default:-1" json:"maxUsers"` // shop members

	// ---- Pixels ----
	MaxFacebookPixels int `gorm:"not null;default:1" json:"maxFacebookPixels"`
	MaxTikTokPixels   int `gorm:"not null;default:1" json:"maxTikTokPixels"`

	// ---- Order features ----
	HasConfirmationOrders bool `gorm:"default:true" json:"hasConfirmationOrders"`
	HasAbandonedOrders    bool `gorm:"default:false" json:"hasAbandonedOrders"`
	HasOrderTracking      bool `gorm:"default:false" json:"hasOrderTracking"` // suivi

	// ---- Client features ----
	// Saves clients on order + tracks delivery status — paid tiers only
	HasClientTracking bool `gorm:"default:false" json:"hasClientTracking"`
}

type ShopSubscription struct {
	BaseModel
	ShopID    uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex" json:"shopId"`
	PlanID    uuid.UUID  `gorm:"type:uuid;not null" json:"planId"`
	Plan      Plan       `gorm:"foreignKey:PlanID;references:ID" json:"plan"`
	StartedAt time.Time  `gorm:"not null" json:"startedAt"`
	ExpiresAt *time.Time `json:"expiresAt"` // null = no expiry
}
