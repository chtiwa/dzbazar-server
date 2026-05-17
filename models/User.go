package models

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	BaseModel
	FirstName   string `gorm:"not null" json:"firstName"`
	LastName    string `gorm:"not null" json:"lastName"`
	PhoneNumber string `gorm:"not null" json:"phoneNumber"`
	Email       string `gorm:"unique;not null" json:"email"`
	Password    string `json:"password"`
	Role        string `gorm:"default:User" json:"role" binding:"oneof=Admin Moderator User"`
	IsVerified  bool   `gorm:"default:false" json:"isVerified"`

	EmailOTP          string     `json:"emailOtp"`
	EmailOTPExpiresAt *time.Time `json:"emailOtpExpiresAt"`

	// Relationships
	Memberships []ShopMember `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"memberships,omitempty"`
}

type ShopMember struct {
	BaseModel
	ShopID uuid.UUID `gorm:"type:uuid;not null;index;uniqueIndex:idx_shop_user" json:"shopId"`
	UserID uuid.UUID `gorm:"type:uuid;not null;index;uniqueIndex:idx_shop_user" json:"userId"`
	User   User      `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"user"`

	// Role is local to THIS shop. A user can be an 'Owner' in Shop A, but 'Logistics' in Shop B.
	Role string `gorm:"type:text;not null;default:'Staff'" json:"role"`
}
