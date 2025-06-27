package models

import "github.com/google/uuid"

type Product struct {
	BaseModel
	Title       string         `gorm:"not null" json:"title"`
	Description string         `gorm:"not null" json:"description"`
	Price       float64        `gorm:"not null" json:"price"`
	OldPrice    float64        `gorm:"default:0" json:"oldPrice"`
	Images      []ProductImage `gorm:"foreignKey:ProductID;references:ID;constraint:OnDelete:CASCADE" json:"images"`
	CategoryID  uuid.UUID      `gorm:"not null" json:"categoryId"`
	Category    Category       `gorm:"foreignKey:CategoryID;references:ID"`
	Variants    []Variant      `gorm:"foreignKey:ProductID;references:ID;constraint:OnDelete:CASCADE" json:"variants"` // can be null when the product doesn't have any variants
}

type ProductImage struct {
	BaseModel
	ProductID uuid.UUID `gorm:"not null" json:"productId"`
	URL       string    `gorm:"not null" json:"url"`
}
