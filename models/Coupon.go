package models

import "github.com/google/uuid"

type Coupon struct {
	BaseModel
	ShopID  uuid.UUID `gorm:"not null;index" json:"shopId"`
	Code    string    `gorm:"not null;index" json:"code"` // stored upper-cased + trimmed
	Percent int       `gorm:"not null" json:"percent"`    // 1-100
	Active  bool      `gorm:"default:true" json:"active"`

	// Empty Products + LandingPages means the coupon applies shop-wide.
	Products     []Product     `gorm:"many2many:coupon_products;" json:"products"`
	LandingPages []LandingPage `gorm:"many2many:coupon_landing_pages;" json:"landingPages"`
}
