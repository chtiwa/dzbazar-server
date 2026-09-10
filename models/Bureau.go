package models

import "github.com/google/uuid"

// Bureau is one stopdesk desk name for one shop in one wilaya. Every shop
// gets one row per wilaya at creation (shopsController.go's creation
// transaction, and migration 00036's backfill for shops that predate it),
// named after the wilaya, which the owner can then rename, delete, or add
// more desks beside.
//
// Deliberately NOT tied to a delivery company: the carrier is only chosen
// later, at ship time, so the same desk list serves every carrier.
//
// Uniqueness (shop + wilaya + name, partial on deleted_at IS NULL) lives in
// migration 00036 rather than a GORM uniqueIndex tag, because GORM can't
// express the partial predicate and AutoMigrate is not used here. Multiple
// distinct desk names per wilaya are expected and allowed.
type Bureau struct {
	BaseModel
	ShopID uuid.UUID `gorm:"type:uuid;not null;index:idx_bureaux_shop_wilaya" json:"shopId"`
	Shop   Shop      `gorm:"foreignKey:ShopID;references:ID" json:"-"`

	WilayaID int    `gorm:"not null;index:idx_bureaux_shop_wilaya" json:"wilayaId"` // e.g. 16 for Alger
	Name     string `gorm:"not null" json:"name"`
}

// TableName overrides GORM's default pluralization ("bureaus"), which does
// not match the actual table name created by migration 00036 ("bureaux").
func (Bureau) TableName() string {
	return "bureaux"
}
