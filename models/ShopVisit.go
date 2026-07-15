package models

import (
	"time"

	"github.com/google/uuid"
)

// ShopVisit = one unique storefront visitor per day per shop.
// Uniqueness (shop_id, day, visitor_id) is enforced by index; the storefront
// beacon upserts with ON CONFLICT DO NOTHING, so COUNT(*) per day = unique
// daily visitors (Shopify's "visitors" metric).
// ponytail: 1 row/visitor/day/shop. Fine to ~10k/day/shop. If storage bites →
// nightly rollup into a counts table and drop raw rows.
type ShopVisit struct {
	ID        uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey" json:"id"`
	ShopID    uuid.UUID `gorm:"type:uuid;not null;index:idx_shop_day_visitor,unique" json:"shopId"`
	Day       time.Time `gorm:"type:date;not null;index:idx_shop_day_visitor,unique" json:"day"`
	VisitorID string    `gorm:"not null;index:idx_shop_day_visitor,unique" json:"visitorId"`
	CreatedAt time.Time `json:"createdAt"`
}
