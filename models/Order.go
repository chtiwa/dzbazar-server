package models

type Client struct {
	FullName    string `json:"fullName"`
	PhoneNumber string `json:"phoneNumber"`
	State       string `json:"state"`
	StateNumber uint   `json:"stateNumber"`
	City        string `json:"city"`
}

type Order struct {
	BaseModel
	ShopName    string `json:"shopName"` // should be shop id
	ProductName string `json:"productName"`
	Client
	Quantity       uint    `json:"quantity"`
	Variant        string  `json:"variant"` // 100ml
	Price          float64 `json:"price"`
	ShippingMethod string  `json:"shippingMethod"`
	ShippingPrice  float64 `json:"shippingPrice"`
	TotalPrice     float64 `json:"totalPrice"`
	Status         string  `gorm:"default:Pending" json:"status"`
	// binding:"oneof=Pending Not Responding Confirmed Canceled Abandoned"
}
