package models

import "github.com/google/uuid"

type AbandonedLead struct {
	BaseModel
	ShopID           uuid.UUID `gorm:"not null;index" json:"shopId"`
	ProductID        uuid.UUID `gorm:"not null" json:"productId"`
	ProductTitle     string    `json:"productTitle"`
	Price            float64   `json:"price"`
	CombinationStr   string    `json:"combinationStr"`
	State            string    `json:"state"`
	City             string    `json:"city"`
	ShippingMethod   string    `json:"shippingMethod"`
	Quantity         int       `json:"quantity"`
	FullName         string    `gorm:"not null" json:"fullName"`
	PhoneNumber      string    `gorm:"not null" json:"phoneNumber"`
	FBclid           string    `json:"fbclid"`
	FBp              string    `json:"fbp"`
	FBc              string    `json:"fbc"`
	ConversionSource string    `json:"conversionSource"`
}
