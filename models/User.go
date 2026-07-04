package models

import (
	"time"

	"github.com/google/uuid"
)

// ShopRole is the source-of-truth table for valid shop member roles.
// ponytail: permission logic lives in roleCan(); add columns only if shops need custom permissions.
type ShopRole struct {
	Name string `gorm:"primaryKey" json:"name"` // owner | moderator | confirmation
}

type User struct {
	BaseModel
	FirstName   string `gorm:"not null" json:"firstName"`
	LastName    string `gorm:"not null" json:"lastName"`
	PhoneNumber string `gorm:"not null" json:"phoneNumber"`
	Email       string `gorm:"unique;not null" json:"email"`
	Password    string `json:"password"`
	Role        string `gorm:"default:'moderator'" json:"role"`
	IsVerified  bool   `gorm:"default:false" json:"isVerified"`
	IsSuspended bool   `gorm:"default:false" json:"isSuspended"`

	// PlatformRole is orthogonal to Role/ShopMember.Role: it answers "can this
	// user act across all shops", never "what can they do inside shop X".
	// Values: "" (regular user), "support", "super_admin".
	PlatformRole string `gorm:"default:''" json:"platformRole"`

	EmailOTP          string     `json:"emailOtp"`
	EmailOTPExpiresAt *time.Time `json:"emailOtpExpiresAt"`

	// Relationships
	Memberships []ShopMember `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"memberships,omitempty"`
}

type ShopMember struct {
	BaseModel
	ShopID uuid.UUID `gorm:"type:uuid;not null;index;uniqueIndex:idx_shop_user" json:"shopId"`
	UserID uuid.UUID `gorm:"type:uuid;not null;index;uniqueIndex:idx_shop_user" json:"userId"`

	User User `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"user"`
	Shop Shop `gorm:"foreignKey:ShopID;constraint:OnDelete:CASCADE" json:"shop"`

	Role string `gorm:"type:text;not null;default:'moderator'" json:"role"`
}
