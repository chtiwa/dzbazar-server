package models

import (
	"time"

	"github.com/google/uuid"
)

// Experiment status values.
const (
	ExperimentStatusRunning = "running"
	ExperimentStatusDecided = "decided"
	ExperimentStatusStopped = "stopped"
)

// LandingPageExperiment groups 2+ existing LandingPage rows ("sets") of the
// same product under strict round-robin traffic split. AssignmentCursor is
// the atomic round-robin counter — see services.AssignExperimentVariant.
// Views/conversions/rate per set are never duplicated here: they're read
// straight off the existing LandingPage attribution queries, keyed by
// each set's own ID.
type LandingPageExperiment struct {
	BaseModel
	ShopID              uuid.UUID  `gorm:"not null;index"       json:"shopId"`
	ProductID           uuid.UUID  `gorm:"not null;index"       json:"productId"`
	Name                string     `gorm:"not null"             json:"name"`
	TargetConversions   int        `gorm:"not null;default:100" json:"targetConversions"`
	Status              string     `gorm:"not null;default:'running'" json:"status"` // running|decided|stopped
	WinnerLandingPageID *uuid.UUID `gorm:"type:uuid"            json:"winnerLandingPageId"`
	// AssignmentCursor is internal round-robin state, never exposed to clients.
	AssignmentCursor int64 `gorm:"not null;default:0" json:"-"`

	Sets []LandingPage `gorm:"foreignKey:ExperimentID;references:ID" json:"sets,omitempty"`
}

// LandingPageExperimentAssignment pins one visitor to one set for the life of
// an experiment, keyed by the same persistent visitor_id page_visits uses —
// a reload or return visit never reassigns them to a different set.
type LandingPageExperimentAssignment struct {
	ID            uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey" json:"id"`
	ExperimentID  uuid.UUID `gorm:"not null;uniqueIndex:idx_lpea_experiment_visitor" json:"experimentId"`
	VisitorID     string    `gorm:"not null;uniqueIndex:idx_lpea_experiment_visitor" json:"visitorId"`
	LandingPageID uuid.UUID `gorm:"not null" json:"landingPageId"`
	CreatedAt     time.Time `json:"createdAt"`
}
