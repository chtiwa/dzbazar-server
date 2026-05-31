package models

import "github.com/google/uuid"

type AvailableDeliveryCompany struct {
	BaseModel
	Name  string                         `gorm:"not null" json:"name"`
	URL   string                         `gorm:"not null" json:"url"`
	Image *AvailableDeliveryCompanyImage `gorm:"foreignKey:AvailableDeliveryCompanyID;references:ID" json:"image"`
}

type AvailableDeliveryCompanyImage struct {
	BaseModel
	AvailableDeliveryCompanyID uuid.UUID `gorm:"not null" json:"availableDeliveryCompanyId"`
	URL                        string    `gorm:"not null" json:"url"`
}

type DeliveryCompany struct {
	BaseModel
	ShopID                     uuid.UUID                `gorm:"type:uuid;not null;index" json:"shopId"`
	AvailableDeliveryCompanyID uuid.UUID                `gorm:"type:uuid;not null" json:"availableDeliveryCompanyId"`
	AvailableDeliveryCompany   AvailableDeliveryCompany `gorm:"foreignKey:AvailableDeliveryCompanyID;references:ID" json:"availableDeliveryCompany"`
	Token                      string                   `json:"token"`
	MerchantID                 string                   `json:"merchantId"`
}
