package models

import (
	"time"

	"github.com/google/uuid"
)

// FlaggedClient remembers a platform browser ID — Facebook's _fbp cookie or
// TikTok's _ttp cookie — that has once placed an order with a cussword in the
// fullName field. Unlike name, phone, or IP, these ids stay stable for the
// same browser/device across orders, so they're what lets a repeat troll be
// caught even after they clean up their name on a later order. fbclid/ttclid
// are NOT used here since they're per-ad-click and change every time.
//
// ResolvedAt/ResolvedBy let a super admin or support agent dismiss a flag as
// a false positive from the fraud review page — the row is kept (it's the
// history), but a resolved flag no longer blocks that client's future orders
// (see the resolved_at IS NULL check in ordersController.go CreateOrder).
type FlaggedClient struct {
	BaseModel
	ShopID   uuid.UUID `gorm:"not null;uniqueIndex:idx_flagged_client_shop_platform_cid" json:"shopId"`
	Platform string    `gorm:"not null;uniqueIndex:idx_flagged_client_shop_platform_cid" json:"platform"` // "facebook" | "tiktok"
	ClientID string    `gorm:"not null;uniqueIndex:idx_flagged_client_shop_platform_cid" json:"clientId"` // fbp or ttp value

	ResolvedAt *time.Time `json:"resolvedAt"`
	ResolvedBy *uuid.UUID `gorm:"type:uuid" json:"resolvedBy"`
}
