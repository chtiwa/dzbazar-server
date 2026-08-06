package models

import "github.com/google/uuid"

type Shop struct {
	BaseModel

	Name         string         `gorm:"not null" json:"name"`
	Slug         string         `gorm:"uniqueIndex;not null" json:"slug"`
	Description  string         `json:"description"`
	Phone        string         `json:"phone"`
	Email        string         `json:"email"`
	Address      string         `json:"address"`
	FacebookURL  string         `json:"facebookUrl"`
	InstagramURL string         `json:"instagramUrl"`
	TiktokURL    string         `json:"tiktokUrl"`
	OwnerID      uuid.UUID      `gorm:"type:uuid;not null;index" json:"ownerId"`
	Owner        User           `gorm:"foreignKey:OwnerID;references:ID" json:"owner"`
	IsActive     bool           `gorm:"default:true" json:"isActive"`
	IsVerified   bool           `gorm:"default:false" json:"isVerified"`
	LogoImage    *ShopLogoImage `gorm:"foreignKey:ShopID;references:ID" json:"logoImage"`

	// Round-robin cursor for confirmatrice auto-assignment (services.AutoAssignOrder).
	ConfirmatriceCursor int64 `gorm:"not null;default:0" json:"-"`

	// Per-shop fraud-signal toggles (services.FraudHiddenReason). Each defaults
	// off — these are best-effort heuristics that shadow-ban real orders when
	// wrong, so an owner opts in deliberately rather than inheriting a platform
	// default.
	BanIncognitoEnabled  bool `gorm:"not null;default:false" json:"banIncognitoEnabled"`
	BanVpnEnabled        bool `gorm:"not null;default:false" json:"banVpnEnabled"`
	BanDatacenterEnabled bool `gorm:"not null;default:false" json:"banDatacenterEnabled"`

	Members  []ShopMember `gorm:"foreignKey:ShopID;constraint:OnDelete:CASCADE" json:"members,omitempty"`
	Products []Product    `gorm:"foreignKey:ShopID;constraint:OnDelete:CASCADE" json:"products"`
	Orders   []Order      `gorm:"foreignKey:ShopID;constraint:OnDelete:CASCADE" json:"orders"`
	Clients  []Client     `gorm:"foreignKey:ShopID;constraint:OnDelete:CASCADE" json:"clients"`
	Pixels   []Pixel      `gorm:"foreignKey:ShopID;constraint:OnDelete:CASCADE" json:"pixels"`
}

type ShopLogoImage struct {
	BaseModel
	ShopID uuid.UUID `gorm:"not null" json:"shopId"`
	URL    string    `gorm:"not null" json:"url"`
}
