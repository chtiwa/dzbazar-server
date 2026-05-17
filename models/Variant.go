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

type VariantItem struct {
	BaseModel
	VariantID uuid.UUID `gorm:"not null" json:"variantId"`
	Variant   Variant   `gorm:"foreignKey:VariantID;references:ID" json:"variant"`
	Value     string    `gorm:"not null" json:"value"` // 100ml - blue - 6.5
}

type ProductVariantCombination struct {
	BaseModel
	ProductID uuid.UUID `gorm:"not null;index" json:"productId"`

	SKU      string  `gorm:"uniqueIndex;not null" json:"sku"`
	Price    float64 `gorm:"not null" json:"price"` // Changed to float64 to match your Product model!
	Quantity int     `gorm:"default:0" json:"quantity"`

	// THE SHOPIFY TRICK: Hardcode up to 3 options directly on the row.
	// These are pointers so they can be NULL if a product only has 1 or 2 options.
	Option1ID *uuid.UUID `gorm:"index" json:"option1Id"`
	Option2ID *uuid.UUID `gorm:"index" json:"option2Id"`
	Option3ID *uuid.UUID `gorm:"index" json:"option3Id"`

	// Optional: Store the human-readable string here too (e.g. "Red / XL")
	// so your Order model doesn't have to look it up!
	CombinationString string `json:"combinationString"`
}
