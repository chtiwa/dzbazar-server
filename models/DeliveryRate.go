package models

import "github.com/google/uuid"

type DeliveryRate struct {
	BaseModel
	ShopID uuid.UUID `gorm:"type:uuid;not null;index:idx_shop_wilaya,unique" json:"shopId"`
	Shop   Shop      `gorm:"foreignKey:ShopID;references:ID" json:"-"`

	WilayaID   int    `gorm:"not null;index:idx_shop_wilaya,unique" json:"wilayaId"` // e.g., 16 for Alger
	WilayaName string `gorm:"not null" json:"wilayaName"`

	IsActive bool `gorm:"default:true" json:"isActive"`

	HasDoorstep  bool    `gorm:"default:true" json:"hasDoorstep"`
	DoorstepRate float64 `gorm:"default:0" json:"doorstepRate"`

	HasStopdesk  bool    `gorm:"default:false" json:"hasStopdesk"`
	StopdeskRate float64 `gorm:"default:0" json:"stopdeskRate"`
}
