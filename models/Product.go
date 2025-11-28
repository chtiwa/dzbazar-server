package models

import "github.com/google/uuid"

type Product struct {
	BaseModel
	Title       string         `gorm:"not null" json:"title"`
	Description string         `gorm:"not null" json:"description"`
	Brand       string         `gorm:"omitempty" json:"brand"`
	Price       float64        `gorm:"not null" json:"price"`
	OldPrice    float64        `gorm:"default:0" json:"oldPrice"`
	Active      bool           `gorm:"default:true" json:"active"`
	Images      []ProductImage `gorm:"foreignKey:ProductID;references:ID;constraint:OnDelete:CASCADE" json:"images"`
	Variants    []Variant      `gorm:"foreignKey:ProductID;constraint:OnDelete:CASCADE" json:"variants"` // can be null when the product doesn't have any variants
	Tags        []Tag          `gorm:"many2many:product_tags;" json:"tags"`
}

type ProductImage struct {
	BaseModel
	ProductID uuid.UUID `gorm:"not null" json:"productId"`
	URL       string    `gorm:"not null" json:"url"`
}

type Tag struct {
	ID   uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey" json:"id"`
	Name string    `gorm:"unique;not null" json:"name"`
}
