package models

import "github.com/google/uuid"

// LandingPageImageCampaign groups the images from one sequential-generation
// set so the merchant can fetch/regenerate the set as a unit. Each image is
// driven by its own free-text prompt (not stored here) — this row is just
// the grouping + count.
type LandingPageImageCampaign struct {
	BaseModel
	ShopID uuid.UUID  `gorm:"type:uuid;not null;index" json:"shopId"`
	UserID *uuid.UUID `gorm:"type:uuid" json:"userId"`

	ImageCount int `gorm:"not null" json:"imageCount"`

	Images []LandingPageImageGenUsage `gorm:"foreignKey:CampaignID" json:"images,omitempty"`
}
