package models

import (
	"time"

	"github.com/google/uuid"
)

// Invoice is a merchant's proof of a manual Redot payment for a paid plan.
// Status: pending | approved | denied. Approval upserts ShopSubscription the
// same way superadmin.ApprovePlanSwitchRequest does.
type Invoice struct {
	BaseModel
	ShopID uuid.UUID `gorm:"type:uuid;not null" json:"shopId"`
	PlanID uuid.UUID `gorm:"type:uuid;not null" json:"planId"`
	Plan   Plan      `gorm:"foreignKey:PlanID;references:ID" json:"plan"`

	Amount        float64 `gorm:"not null" json:"amount"`
	PaymentMethod string  `gorm:"not null;default:redot" json:"paymentMethod"`
	Status        string  `gorm:"not null;default:pending" json:"status"`

	ProofScreenshotURL string `json:"proofScreenshotUrl"`

	ReviewedBy *uuid.UUID `gorm:"type:uuid" json:"reviewedBy"`
	ReviewedAt *time.Time `json:"reviewedAt"`
}
