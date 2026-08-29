package models

import "github.com/google/uuid"

type LandingPageImageGenUsage struct {
	BaseModel
	ShopID     uuid.UUID  `gorm:"type:uuid;not null;index" json:"shopId"`
	UserID     *uuid.UUID `gorm:"type:uuid" json:"userId"`
	URL        string     `json:"url"`
	CampaignID *uuid.UUID `gorm:"type:uuid;index" json:"campaignId"`
	ImageIndex int        `gorm:"not null;default:0" json:"imageIndex"`
}
