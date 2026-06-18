package models

import "github.com/google/uuid"

// AuditLog records every critical Super Admin action: shop suspension/deletion,
// user suspension/deletion, plan changes, feature flag toggles, impersonation
// start/end, and settings changes. Written, never edited or deleted via the API.
type AuditLog struct {
	BaseModel
	ActorID    uuid.UUID  `gorm:"type:uuid;not null;index" json:"actorId"`
	ActorEmail string     `gorm:"not null" json:"actorEmail"`
	Action     string     `gorm:"not null;index" json:"action"`
	TargetType string     `gorm:"not null;index" json:"targetType"`
	TargetID   *uuid.UUID `gorm:"type:uuid;index" json:"targetId"`
	Metadata   string     `json:"metadata"` // free-form JSON blob, e.g. before/after values
	IPAddress  string     `json:"ipAddress"`
}
