package models

import "github.com/google/uuid"

type Shop struct {
	BaseModel

	Name        string    `gorm:"not null" json:"name"`
	Slug        string    `gorm:"uniqueIndex;not null" json:"slug"`
	Description string    `json:"description"`
	OwnerID     uuid.UUID `gorm:"type:uuid;not null;index" json:"ownerId"`
	Owner       User      `gorm:"foreignKey:OwnerID;references:ID" json:"owner"`
	LogoURL     string    `json:"logoUrl"`
	IsActive    bool      `gorm:"default:true" json:"isActive"`
	IsVerified  bool      `gorm:"default:false" json:"isVerified"`

	// LogoImage ShopLogoImage

	Members  []ShopMember `gorm:"foreignKey:ShopID;constraint:OnDelete:CASCADE" json:"members,omitempty"`
	Products []Product    `gorm:"foreignKey:ShopID;constraint:OnDelete:CASCADE" json:"products"`
	Orders   []Order      `gorm:"foreignKey:ShopID;constraint:OnDelete:CASCADE" json:"orders"`
	Clients  []Client     `gorm:"foreignKey:ShopID;constraint:OnDelete:CASCADE" json:"clients"`
}

// type ShopLogoImage struct {
// 	BaseModel
// 	ShopID uuid.UUID `gorm:"not null" json:"shopId"`
// 	URL    string    `gorm:"not null" json:"url"`
// }
