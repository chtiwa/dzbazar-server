package models

// Wilaya is the source-of-truth table for the 58 Algerian wilayas
// (provinces) and their default delivery pricing, seeded by
// 00019_wilayas_table.sql from the former static_wilayas.json. Algeria isn't
// getting new provinces, so this is edit-only — no create/delete — same
// pattern as ShopRole/PermissionAction. ID is the real wilaya code (1-58),
// not a generated key.
type Wilaya struct {
	ID   int    `gorm:"primaryKey" json:"id"`
	Name string `gorm:"not null" json:"name"`

	IsActive bool `gorm:"not null;default:true" json:"isActive"`

	HasStopdesk  bool    `gorm:"not null;default:false" json:"hasStopdesk"`
	StopdeskRate float64 `gorm:"not null;default:0" json:"stopdeskRate"`

	HasDoorstep  bool    `gorm:"not null;default:false" json:"hasDoorstep"`
	DoorstepRate float64 `gorm:"not null;default:0" json:"doorstepRate"`
}
