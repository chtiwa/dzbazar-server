package models

import "github.com/google/uuid"

type Product struct {
	BaseModel
	ShopID      uuid.UUID      `gorm:"not null" json:"shopId"`
	Title       string         `gorm:"not null" json:"title"`
	Description string         `gorm:"not null" json:"description"`
	Price       float64        `gorm:"not null" json:"price"`
	OldPrice    *float64       `gorm:"default:0" json:"oldPrice"`
	Active      bool           `gorm:"default:true" json:"active"`
	Images      []ProductImage `gorm:"foreignKey:ProductID;references:ID;constraint:OnDelete:CASCADE" json:"images"`
	Variants    []Variant      `gorm:"foreignKey:ProductID;constraint:OnDelete:CASCADE" json:"variants"` // can be null when the product doesn't have any variants

	Combinations []ProductVariantCombination `gorm:"foreignKey:ProductID;constraint:OnDelete:CASCADE" json:"combinations"`

	// Count of non-deleted orders containing this product. Computed per-request, not stored.
	Orders int64 `gorm:"-" json:"orders"`
	// % of shipped orders containing this product that reached "Livré". Nil when no shipped orders yet.
	DeliveryRate *float64 `gorm:"-" json:"deliveryRate"`
	// Unique visitors to this product's page (all-time). Computed per-request, not stored.
	Views int64 `gorm:"-" json:"views"`
	// Orders / Views * 100. Nil when there are no views yet.
	ConversionRate *float64 `gorm:"-" json:"conversionRate"`
	// % of orders containing this product that were ever confirmed (audit-log based). Nil when no orders yet.
	ConfirmationRate *float64 `gorm:"-" json:"confirmationRate"`
}

type ProductImage struct {
	BaseModel
	ProductID  uuid.UUID `gorm:"not null" json:"productId"`
	URL        string    `gorm:"not null" json:"url"`
	OrderIndex int       `gorm:"not null;default:0" json:"orderIndex"`
}

type LandingPage struct {
	BaseModel
	ShopID    uuid.UUID          `gorm:"not null" json:"shopId"`
	Shop      Shop               `gorm:"foreignKey:ShopID;references:ID" json:"shop,omitempty"`
	ProductID uuid.UUID          `gorm:"not null" json:"productId"`
	Product   Product            `gorm:"foreignKey:ProductID;references:ID" json:"product"`
	Title     string             `gorm:"not null" json:"title"`
	Images    []LandingPageImage `gorm:"foreignKey:LandingPageID;constraint:OnDelete:CASCADE" json:"images"`
	Active    bool               `gorm:"default:true" json:"active"`

	// ExperimentID nil = standalone page. Non-nil = this page is one "set" of an
	// A/B test, and ExperimentPosition is its stable round-robin slot (0,1,2...).
	ExperimentID       *uuid.UUID `gorm:"type:uuid;index" json:"experimentId"`
	ExperimentPosition int        `gorm:"not null;default:0" json:"experimentPosition"`

	// Count of non-deleted orders containing this landing page's product. Computed per-request, not stored.
	Orders int64 `gorm:"-" json:"orders"`
	// Unique visitors to this landing page (all-time). Computed per-request, not stored.
	Views int64 `gorm:"-" json:"views"`
	// Orders attributed to this landing page / Views * 100. Nil when there are no views yet.
	ConversionRate *float64 `gorm:"-" json:"conversionRate"`
}

type LandingPageImage struct {
	BaseModel
	LandingPageID uuid.UUID `gorm:"not null" json:"landingPageID"`
	URL           string    `gorm:"not null" json:"url"`
	OrderIndex    int       `gorm:"not null;default:0" json:"orderIndex"`
}
