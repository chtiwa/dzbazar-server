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
	Price    float64 `gorm:"not null" json:"price"`
	Quantity int     `gorm:"default:0" json:"quantity"`

	Option1ID *uuid.UUID   `gorm:"index" json:"option1Id"`
	Option1   *VariantItem `gorm:"foreignKey:Option1ID" json:"option1,omitempty"`

	Option2ID *uuid.UUID   `gorm:"index" json:"option2Id"`
	Option2   *VariantItem `gorm:"foreignKey:Option2ID" json:"option2,omitempty"`

	Option3ID *uuid.UUID   `gorm:"index" json:"option3Id"`
	Option3   *VariantItem `gorm:"foreignKey:Option3ID" json:"option3,omitempty"`

	CombinationString string `json:"combinationString"`
}
