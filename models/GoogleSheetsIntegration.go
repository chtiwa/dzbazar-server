package models

import (
	"time"

	"github.com/google/uuid"
)

// GoogleSheetsIntegration holds one shop's connection to a Google Sheet that
// every new order gets appended to as a row. One per shop (ShopID unique).
// ServiceAccountJSON is the raw Google service-account key the merchant
// pasted — stored as plaintext, matching the existing DeliveryCompany.Token /
// Pixel.AccessToken convention in this codebase, gated by shop-scoped auth
// middleware rather than encrypted at rest.
type GoogleSheetsIntegration struct {
	BaseModel
	ShopID uuid.UUID `gorm:"not null;uniqueIndex" json:"shopId"`
	Shop   Shop      `gorm:"foreignKey:ShopID;references:ID" json:"shop,omitempty"`

	ServiceAccountJSON string `gorm:"not null" json:"-"`
	SpreadsheetID      string `gorm:"not null" json:"spreadsheetId"`
	SheetName          string `gorm:"not null;default:Orders" json:"sheetName"`

	IsActive bool `gorm:"not null;default:true" json:"isActive"`

	LastSyncedAt *time.Time `json:"lastSyncedAt"`
	LastError    string     `gorm:"not null;default:''" json:"lastError"`
}
