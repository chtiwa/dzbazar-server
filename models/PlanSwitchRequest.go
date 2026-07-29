package models

import (
	"time"

	"github.com/google/uuid"
)

// PlanSwitchRequest is created when a shop owner asks to switch plans.
// SubscribeShopToPlan only ever creates one of these (status "pending"); a
// super admin approving/rejecting it is what actually touches
// ShopSubscription. Status: pending | approved | rejected.
type PlanSwitchRequest struct {
	BaseModel
	ShopID uuid.UUID `gorm:"type:uuid;not null" json:"shopId"`
	PlanID uuid.UUID `gorm:"type:uuid;not null" json:"planId"`
	Plan   Plan      `gorm:"foreignKey:PlanID;references:ID" json:"plan"`

	Status string `gorm:"not null;default:pending" json:"status"`

	ReviewedBy *uuid.UUID `gorm:"type:uuid" json:"reviewedBy"`
	ReviewedAt *time.Time `json:"reviewedAt"`
}
