package models

import (
	"time"

	"github.com/google/uuid"
)

type OrderStatus string

// 1. The Main Order (The Box being shipped)
type Order struct {
	BaseModel
	ShopID uuid.UUID `gorm:"not null;index" json:"shopId"`

	// THE FIX: Link the order to the new Client model
	ClientID uuid.UUID `gorm:"not null;index" json:"clientId"`
	Client   Client    `gorm:"foreignKey:ClientID;references:ID" json:"client"`

	// Financials & Shipping
	ShippingMethod string  `json:"shippingMethod"`
	ShippingPrice  float64 `json:"shippingPrice"`
	TotalPrice     float64 `json:"totalPrice"` // Sum of all items + shipping
	Status         string  `gorm:"default:En attente" json:"status"`

	// Date the order was rescheduled to, set when Status is "Reporté".
	ReportedDate *time.Time `json:"reportedDate"`

	Note string `gorm:"omitempty" json:"note"`

	// Marketing & COD Flags
	TrackingNumber   string `json:"trackingNumber"`
	Ouvrable         bool   `gorm:"default:false" json:"ouvrable"`
	Fragile          bool   `gorm:"default:false" json:"fragile"`
	Essayable        bool   `gorm:"default:false" json:"essayable"`
	FBclid           string `json:"fbclid"`
	FBc              string `json:"fbc"`
	FBp              string `json:"fbp"`
	ConversionSource string `json:"conversionSource"`
	IsShipped        bool   `gorm:"default:false" json:"isShipped"`

	// Carrier the order was actually handed to, and when — set once at
	// shipping time, independent of any later edits to the order (unlike
	// UpdatedAt, which bumps on every unrelated change).
	ShippedAt    *time.Time                `json:"shippedAt"`
	ShippedViaID *uuid.UUID                `gorm:"type:uuid" json:"shippedViaId"`
	ShippedVia   *AvailableDeliveryCompany `gorm:"foreignKey:ShippedViaID;references:ID" json:"shippedVia"`

	// THE FIX: One Order has Many Items
	Items []OrderItem `gorm:"foreignKey:OrderID;constraint:OnDelete:CASCADE" json:"items"`
}

// 2. The Contents of the Box
type OrderItem struct {
	BaseModel
	OrderID uuid.UUID `gorm:"not null;index" json:"orderId"`

	ProductID uuid.UUID `gorm:"not null" json:"productId"`
	Product   Product   `gorm:"foreignKey:ProductID;references:ID" json:"product"` // Optional: to fetch product details easily

	/// 1. The Foreign Key Column (Stores the exact ID pointing to the flattened SKU row)
	ProductVariantCombinationID uuid.UUID `gorm:"type:uuid;not null;index" json:"productVariantCombinationId"`

	// 2. The Relationship Instance (Enables GORM's Preload engine to look up the data)
	ProductVariantCombination ProductVariantCombination `gorm:"foreignKey:ProductVariantCombinationID;references:ID;constraint:OnDelete:RESTRICT" json:"productVariantCombination"`

	Quantity uint    `gorm:"not null;default:1" json:"quantity"`
	Price    float64 `gorm:"not null" json:"price"` // Price of this specific variant at the time of purchase
}
