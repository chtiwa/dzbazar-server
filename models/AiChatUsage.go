package models

import "github.com/google/uuid"

// AiChatUsage logs one successful help-chat model call, billed at
// services.CreditCostChat each (see creditsUsed).
type AiChatUsage struct {
	BaseModel
	ShopID           uuid.UUID  `gorm:"type:uuid;not null;index" json:"shopId"`
	UserID           *uuid.UUID `gorm:"type:uuid" json:"userId"`
	PromptTokens     int        `gorm:"not null;default:0" json:"promptTokens"`
	CompletionTokens int        `gorm:"not null;default:0" json:"completionTokens"`
	TotalTokens      int        `gorm:"not null;default:0" json:"totalTokens"`
}
