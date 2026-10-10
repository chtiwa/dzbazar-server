package models

import (
	"time"

	"github.com/google/uuid"
)

// SheetColumn is one exported column: Key picks the value, Header is the
// merchant-editable title written to row 1.
type SheetColumn struct {
	Key    string `json:"key"`
	Header string `json:"header"`
}

// GoogleSheetsIntegration is one shop's connection to a Google Sheet, one per
// (shop, kind): "orders" gets a row per new order, "abandoned" a row per
// abandoned lead. ServiceAccountJSON is a LEGACY per-shop key (v1); empty
// means the platform service account (GOOGLE_SHEETS_SERVICE_ACCOUNT_JSON) is
// used. LastError holds a short error code, never a raw Google error.
type GoogleSheetsIntegration struct {
	BaseModel
	ShopID uuid.UUID `gorm:"not null;uniqueIndex:idx_google_sheets_integrations_shop_kind" json:"shopId"`
	Shop   Shop      `gorm:"foreignKey:ShopID;references:ID" json:"-"`
	Kind   string    `gorm:"not null;default:orders;uniqueIndex:idx_google_sheets_integrations_shop_kind" json:"kind"`

	ServiceAccountJSON string        `gorm:"not null" json:"-"`
	SpreadsheetID      string        `gorm:"not null" json:"spreadsheetId"`
	SheetName          string        `gorm:"not null;default:Orders" json:"sheetName"`
	Columns            []SheetColumn `gorm:"serializer:json" json:"columns"`

	IsActive bool `gorm:"not null;default:true" json:"isActive"`

	LastSyncedAt *time.Time `json:"lastSyncedAt"`
	LastError    string     `gorm:"not null;default:''" json:"lastError"`
}
