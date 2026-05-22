package models

import "github.com/google/uuid"

type Pixel struct {
	BaseModel
	ShopID uuid.UUID `gorm:"not null" json:"shopId"`
	Shop   Shop      `gorm:"foreignKey:ShopID;references:ID" json:"shop"`

	Platform string `gorm:"not null" json:"platform"`
	Title    string `gorm:"not null" json:"title"`
	PixelID  string `gorm:"not null;index:idx_shop_platform_pixel,unique" json:"pixelId"`

	HasAccessToken bool   `gorm:"default:false" json:"hasAccessToken"`
	AccessToken    string `json:"AccessToken"`
}
