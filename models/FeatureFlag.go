package models

// FeatureFlag is a global on/off switch. Per-shop rollout scoping is not
// supported in v1 — every flag is either on or off for the whole platform.
type FeatureFlag struct {
	BaseModel
	Key         string `gorm:"uniqueIndex;not null" json:"key"`
	Label       string `gorm:"not null" json:"label"`
	Description string `json:"description"`
	IsEnabled   bool   `gorm:"default:false" json:"isEnabled"`
}
