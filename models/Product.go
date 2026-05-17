package models

import "github.com/google/uuid"

type Product struct {
	BaseModel
	ShopID      uuid.UUID      `gorm:"not null" json:"shopId"`
	Title       string         `gorm:"not null" json:"title"`
	Description string         `gorm:"not null" json:"description"`
	Price       float64        `gorm:"not null" json:"price"`
	OldPrice    float64        `gorm:"default:0" json:"oldPrice"`
	Active      bool           `gorm:"default:true" json:"active"`
	Images      []ProductImage `gorm:"foreignKey:ProductID;references:ID;constraint:OnDelete:CASCADE" json:"images"`
	Variants    []Variant      `gorm:"foreignKey:ProductID;constraint:OnDelete:CASCADE" json:"variants"` // can be null when the product doesn't have any variants
	Tags        []Tag          `gorm:"many2many:product_tags;" json:"tags"`

	// THE MISSING PIECE: The relationship linking to your flattened inventory
	Combinations []ProductVariantCombination `gorm:"foreignKey:ProductID;constraint:OnDelete:CASCADE" json:"combinations"`
}

type ProductImage struct {
	BaseModel
	ProductID  uuid.UUID `gorm:"not null" json:"productId"`
	URL        string    `gorm:"not null" json:"url"`
	OrderIndex int       `gorm:"not null;default:0" json:"orderIndex"`
}

type Tag struct {
	ID   uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey" json:"id"`
	Name string    `gorm:"unique;not null" json:"name"`
}

type LandingPage struct {
	BaseModel
	ShopID    uuid.UUID          `gorm:"not null" json:"shopId"`
	ProductID uuid.UUID          `gorm:"not null" json:"productId"`
	Product   Product            `gorm:"foreignKey:ProductID;references:ID" json:"product"`
	Title     string             `gorm:"not null" json:"title"`
	Images    []LandingPageImage `gorm:"foreignKey:LandingPageID;constraint:OnDelete:CASCADE" json:"images"`
	Active    bool               `gorm:"default:true" json:"active"`
}

type LandingPageImage struct {
	BaseModel
	LandingPageID uuid.UUID `gorm:"not null" json:"landingPageID"`
	URL           string    `gorm:"not null" json:"url"`
	OrderIndex    int       `gorm:"not null;default:0" json:"orderIndex"`
}
