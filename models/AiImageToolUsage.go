package models

import "github.com/google/uuid"

// AiImageToolUsage logs one successful AI image generation. Billing
// bookkeeping + audit only — the merchant's reference photo is never stored
// anywhere, including here.
type AiImageToolUsage struct {
	BaseModel
	ShopID uuid.UUID  `gorm:"type:uuid;not null;index" json:"shopId"`
	UserID *uuid.UUID `gorm:"type:uuid" json:"userId"`
	Prompt string     `gorm:"type:text;not null;default:''" json:"prompt"`
	Model  string     `gorm:"type:text;not null;default:''" json:"model"`
}
