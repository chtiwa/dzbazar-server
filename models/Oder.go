package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Client struct {
	FullName    string `json:"fullName"`
	PhoneNumber string `json:"phoneNumber"`
	State       string `json:"state"`
	StateNumber uint   `json:"stateNumber"`
	City        string `json:"city"`
}

type Order struct {
	ID          uuid.UUID  `gorm:"type:uuid;default:uuid_generate_v4();primaryKey" json:"id"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	DeletedAt   *time.Time `gorm:"index" json:"deleted_at"`
	ShopName    string     `json:"shopName"`
	ProductName string     `json:"productName"`
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

func (o *Order) BeforeCreate(tx *gorm.DB) (err error) {
	o.ID = uuid.New()
	return
}
