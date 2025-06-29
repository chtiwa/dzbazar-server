package models

import (
	"github.com/google/uuid"
)

type Variant struct {
	BaseModel
	ProductID    uuid.UUID     `gorm:"not null" json:"productId"`
	Product      Product       `gorm:"foreignKey:ProductID;references:ID" json:"product"`
	Title        string        `gorm:"not null" json:"title"` // Capacity - Color - Size
	VariantItems []VariantItem `gorm:"foreignKey:VariantID;constraint:OnDelete:CASCADE" json:"variantItems"`
}

// TODO : when the order is cancelled, get the variant item
type VariantItem struct {
	BaseModel
	VariantID uuid.UUID `json:"variantId"`
	Variant   Variant   `gorm:"foreignKey:VariantID;references:ID" json:"variant"`
	Value     string    `gorm:"not null" json:"value"` // 100ml - blue - 6.5
	Price     float64   `gorm:"default:0" json:"price"`
	Quantity  int       `gorm:"default:0" json:"quantity"` //
}
