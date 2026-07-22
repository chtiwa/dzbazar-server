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

	// Active gates round-robin eligibility (services.EligibleConfirmatrices) —
	// an owner flips this off for a confirmatrice who's out (sick day, leave)
	// without deleting the account or touching her product scope.
	Active bool `gorm:"not null;default:true" json:"active"`
}

// PermissionAction is the source-of-truth table for gate-able action names,
// mirroring ShopRole. Populates the admin UI's grantable-action list and
// validates override writes. Name is "resource.action" dotted (e.g.
// "orders.delete"); Resource/Label drive the admin UI's section grouping.
type PermissionAction struct {
	Name     string `gorm:"primaryKey" json:"name"`
	Resource string `gorm:"type:text;not null;default:''" json:"resource"`
	Label    string `gorm:"type:text;not null;default:''" json:"label"`
}

// RoleActionDefault is the shop-role x action default, replacing the
// hardcoded switch that used to live in services.roleDefault. No row means
// deny — every gated action must have a seeded row per role.
type RoleActionDefault struct {
	Role   string `gorm:"primaryKey" json:"role"`
	Action string `gorm:"primaryKey" json:"action"`
	Allow  bool   `json:"allow"`
}

// ShopMemberPermission is an additive/subtractive override on top of the
// role default from services.roleDefault(). Row present + Allow=true grants
// beyond role; Allow=false revokes from role; no row = role default.
type ShopMemberPermission struct {
	BaseModel
	ShopMemberID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_member_action" json:"shopMemberId"`
	Action       string    `gorm:"type:text;not null;uniqueIndex:idx_member_action" json:"action"`
	Allow        bool      `json:"allow"`

	ShopMember ShopMember       `gorm:"foreignKey:ShopMemberID;constraint:OnDelete:CASCADE" json:"-"`
	ActionRef  PermissionAction `gorm:"foreignKey:Action;references:Name" json:"-"`
}
