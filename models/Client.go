package models

import "github.com/google/uuid"

type Client struct {
	BaseModel
	// The uniqueIndex ensures a phone number is only registered ONCE per shop.
	ShopID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_shop_phone" json:"shopId"`

	FullName      string `json:"fullName"`
	PhoneNumber   string `gorm:"not null;uniqueIndex:idx_shop_phone" json:"phoneNumber"`
	PhoneNumber2  string `json:"phoneNumber2"`
	State         string `json:"state"`
	StateCode     string `json:"stateCode"`
	City          string `json:"city"`
	StopdeskPoint string `json:"stopdeskPoint"`

	Orders []Order `gorm:"foreignKey:ClientID;constraint:OnDelete:SET NULL" json:"orders,omitempty"`
}
