package services

import (
	"errors"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
)

var ErrExperimentNotRunning = errors.New("experiment is not running")

// ForceStopExperiment halts a live A/B test platform-side (super-admin
// shutting down a runaway/abandoned test across tenants). Mirrors the exact
// transition UpdateExperimentByShop makes for a merchant's own "stopped"
// status flip (controllers/landingPageExperimentsController.go) — no new
// lifecycle semantics invented here, same "running" -> "stopped" value.
//
// Unlike ForceUnpublishLandingPage, this never touches LandingPage.Active or
// its cache: only DecideExperimentIfReady's winner path deactivates losing
// sets (services/experiments.go) — a plain stop leaves every set's Active
// field, and therefore the landing-page cache, untouched. So no
// InvalidateLandingPageCaches call is needed here. New visitor assignment
// does stop immediately though: AssignExperimentVariant checks Status itself
// on every call, no caching involved on that path.
//
// Restricted to experiments currently "running" — stopping a "decided" test
// is a no-op (a winner is already picked and served to everyone) and
// stopping an already-"stopped" one is meaningless, so both are rejected
// rather than silently accepted.
func ForceStopExperiment(experiment *models.LandingPageExperiment) error {
	if experiment.Status != models.ExperimentStatusRunning {
		return ErrExperimentNotRunning
	}
	experiment.Status = models.ExperimentStatusStopped
	return initializers.DB.Model(experiment).Select("Status").Updates(experiment).Error
}
