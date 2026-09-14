package models

// Commune is one of Algeria's ~1541 communes, seeded verbatim from the old
// client-embedded static_communes.ts by migration 00044. Fixed reference
// data (edit-only, no create/delete), same shape as Wilaya.
type Commune struct {
	ID       int    `gorm:"primaryKey" json:"id"`
	Name     string `gorm:"not null" json:"name"`
	WilayaID int    `gorm:"not null;index" json:"wilayaId"`
}
