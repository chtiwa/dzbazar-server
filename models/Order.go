package models

import "github.com/google/uuid"

type Client struct {
	FullName    string `json:"fullName"`
	PhoneNumber string `json:"phoneNumber"`
	State       string `json:"state"`
	StateNumber string `json:"stateNumber"`
	StateId     string `json:"stateId"`
	City        string `json:"city"`
	CityId      string `json:"cityId"`
	HubId       string `json:"hubId"`
}

type Order struct {
	BaseModel
	ShopName    string    `json:"shopName"` // should be shop id
	ProductID   uuid.UUID `json:"productId"`
	ProductName string    `json:"productName"`
	Client
	Quantity         uint    `json:"quantity"`
	Variant          string  `json:"variant"` // 100ml
	Price            float64 `json:"price"`
	ShippingMethod   string  `json:"shippingMethod"`
	ShippingPrice    float64 `json:"shippingPrice"`
	TotalPrice       float64 `json:"totalPrice"`
	Status           string  `gorm:"default:En attente" json:"status"`
	Note             string  `gorm:"omitempty" json:"note"`
	FBclid           string  `json:"fbclid"`
	FBc              string  `json:"fbc"`
	FBp              string  `json:"fbp"`
	Ttclid           string  `json:"ttclid"`
	ConversionSource string  `gorm:"binding:tiktok facebook organic" json:"conversionSource"`
	IsShipped        bool    `gorm:"default:false" json:"isShipped"`
	// binding:"oneof=Pending Not Responding Confirmed Canceled Abandoned"
}
