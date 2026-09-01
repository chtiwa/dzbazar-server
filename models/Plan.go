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

	// One unified AI credits allowance per subscription period (-1 =
	// unlimited). Spent by AI features at services.CreditCost* rates;
	// "used" is a weighted SUM over the ai_description_usages and
	// landing_page_image_gen_usages logs since ShopSubscription.StartedAt
	// (see services.CheckCreditBudget) — there is no stored balance.
	CreditsPerMonth int `gorm:"not null;default:0" json:"creditsPerMonth"`

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

	ExpiryReminderSentAt *time.Time `json:"expiryReminderSentAt"` // set when the 3-day-before email is sent; reset to nil whenever ExpiresAt changes
}
