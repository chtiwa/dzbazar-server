package models

import "github.com/google/uuid"

// FlaggedClient remembers a platform browser ID — Facebook's _fbp cookie or
// TikTok's _ttp cookie — that has once placed an order with a cussword in the
// fullName field. Unlike name, phone, or IP, these ids stay stable for the
// same browser/device across orders, so they're what lets a repeat troll be
// caught even after they clean up their name on a later order. fbclid/ttclid
// are NOT used here since they're per-ad-click and change every time.
type FlaggedClient struct {
	BaseModel
	ShopID   uuid.UUID `gorm:"not null;uniqueIndex:idx_flagged_client_shop_platform_cid" json:"shopId"`
	Platform string    `gorm:"not null;uniqueIndex:idx_flagged_client_shop_platform_cid" json:"platform"` // "facebook" | "tiktok"
	ClientID string    `gorm:"not null;uniqueIndex:idx_flagged_client_shop_platform_cid" json:"clientId"` // fbp or ttp value
}
