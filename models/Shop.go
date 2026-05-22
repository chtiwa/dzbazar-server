package models

import "github.com/google/uuid"

type Shop struct {
	BaseModel

	Name        string         `gorm:"not null" json:"name"`
	Slug        string         `gorm:"uniqueIndex;not null" json:"slug"`
	Description string         `json:"description"`
	OwnerID     uuid.UUID      `gorm:"type:uuid;not null;index" json:"ownerId"`
	Owner       User           `gorm:"foreignKey:OwnerID;references:ID" json:"owner"`
	IsActive    bool           `gorm:"default:true" json:"isActive"`
	IsVerified  bool           `gorm:"default:false" json:"isVerified"`
	LogoImage   *ShopLogoImage `gorm:"foreignKey:ShopID;references:ID" json:"logoImage"`

	Members  []ShopMember `gorm:"foreignKey:ShopID;constraint:OnDelete:CASCADE" json:"members,omitempty"`
	Products []Product    `gorm:"foreignKey:ShopID;constraint:OnDelete:CASCADE" json:"products"`
	Orders   []Order      `gorm:"foreignKey:ShopID;constraint:OnDelete:CASCADE" json:"orders"`
	Clients  []Client     `gorm:"foreignKey:ShopID;constraint:OnDelete:CASCADE" json:"clients"`
	Pixels   []Pixel      `gorm:"foreignKey:ShopID;constraint:OnDelete:CASCADE" json:"pixels"`
}

type ShopLogoImage struct {
	BaseModel
	ShopID uuid.UUID `gorm:"not null" json:"shopId"`
	URL    string    `gorm:"not null" json:"url"`
}
