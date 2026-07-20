package services

import (
	"errors"
	"fmt"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrExperimentStopped = errors.New("experiment is stopped")
	ErrNoActiveSets      = errors.New("experiment has no active sets")
)

// SetTally is one experiment set's conversion/view counts, as read from the
// same orders/page_visits queries the standalone landing-page PagePerf panel
// already uses — never a duplicated counter.
type SetTally struct {
	LandingPageID uuid.UUID
	Position      int
	Conversions   int
	Views         int64
}

// ExperimentEvaluation is the outcome of comparing the rate-leader against
// the aggregated rest of an experiment's active sets.
type ExperimentEvaluation struct {
	LeaderID      uuid.UUID
	PValue        float64
	IsSignificant bool
	// Decided is true only once the leader has both cleared the
	// target_conversions floor AND beaten the rest at SignificanceAlpha —
	// raw conversion count alone never decides a winner.
	Decided  bool
	WinnerID uuid.UUID
}

// EvaluateExperiment picks the conversion-rate leader among sets, tests it
// against the pooled rest, and decides a winner only when both the
// minimum-sample floor (targetConversions) and statistical significance
// (SignificanceAlpha) are met. Pure and DB-free — safe to call on every
// conversion (DecideExperimentIfReady) or read-only for a live admin
// standings display (controllers), all state is passed in.
func EvaluateExperiment(sets []SetTally, targetConversions int) ExperimentEvaluation {
	if len(sets) == 0 {
		return ExperimentEvaluation{PValue: 1}
	}

	leader := sets[0]
	leaderRate := setRate(leader)
	maxConversions := leader.Conversions
	for _, s := range sets[1:] {
		if s.Conversions > maxConversions {
			maxConversions = s.Conversions
		}
		rate := setRate(s)
		if rate > leaderRate ||
			(rate == leaderRate && (s.Conversions > leader.Conversions ||
				(s.Conversions == leader.Conversions && s.Position < leader.Position))) {
			leader = s
			leaderRate = rate
		}
	}

	var restConv int
	var restViews int64
	for _, s := range sets {
		if s.LandingPageID == leader.LandingPageID {
			continue
		}
		restConv += s.Conversions
		restViews += s.Views
	}

	_, p := TwoProportionZTest(leader.Conversions, int(leader.Views), restConv, int(restViews))
	significant := p < SignificanceAlpha
	decided := maxConversions >= targetConversions && significant

	eval := ExperimentEvaluation{
		LeaderID:      leader.LandingPageID,
		PValue:        p,
		IsSignificant: significant,
		Decided:       decided,
	}
	if decided {
		eval.WinnerID = leader.LandingPageID
	}
	return eval
}

// setRate returns -1 (never a leader) for a set with no views yet, so an
// untested set can't win by "0/0" default.
func setRate(s SetTally) float64 {
	if s.Views == 0 {
		return -1
	}
	return float64(s.Conversions) / float64(s.Views)
}

// AssignExperimentVariant resolves which set a visitor should see: a
// returning visitor always gets their first-touch set back (sticky), a new
// one gets the next slot in strict round-robin order. Post-decision, every
// visitor gets the winner. The round-robin cursor bump is a single atomic
// UPDATE...RETURNING so concurrent visitors never read the same slot.
func AssignExperimentVariant(experimentID uuid.UUID, visitorID string) (uuid.UUID, error) {
	var experiment models.LandingPageExperiment
	if err := initializers.DB.First(&experiment, "id = ?", experimentID).Error; err != nil {
		return uuid.Nil, err
	}

	if experiment.Status == models.ExperimentStatusDecided && experiment.WinnerLandingPageID != nil {
		return *experiment.WinnerLandingPageID, nil
	}
	if experiment.Status != models.ExperimentStatusRunning {
		return uuid.Nil, ErrExperimentStopped
	}

	var existing models.LandingPageExperimentAssignment
	if err := initializers.DB.
		Where("experiment_id = ? AND visitor_id = ?", experimentID, visitorID).
		First(&existing).Error; err == nil {
		return existing.LandingPageID, nil
	}

	var landingPageID uuid.UUID
	err := initializers.DB.Transaction(func(tx *gorm.DB) error {
		var sets []models.LandingPage
		if err := tx.Where("experiment_id = ? AND active = ?", experimentID, true).
			Order("experiment_position ASC, id ASC").
			Find(&sets).Error; err != nil {
			return err
		}
		if len(sets) == 0 {
			return ErrNoActiveSets
		}

		var cursor int64
		if err := tx.Raw(
			"UPDATE landing_page_experiments SET assignment_cursor = assignment_cursor + 1 WHERE id = ? RETURNING assignment_cursor",
			experimentID,
		).Scan(&cursor).Error; err != nil {
			return err
		}

		chosen := sets[(cursor-1)%int64(len(sets))]

		if err := tx.Exec(
			`INSERT INTO landing_page_experiment_assignments (id, experiment_id, visitor_id, landing_page_id, created_at)
			 VALUES (uuid_generate_v4(), ?, ?, ?, now())
			 ON CONFLICT (experiment_id, visitor_id) DO NOTHING`,
			experimentID, visitorID, chosen.ID,
		).Error; err != nil {
			return err
		}

		// Re-select rather than trust chosen.ID — a concurrent request for the
		// same never-before-seen visitor_id may have won the insert race.
		// Row().Scan (not gorm's Scan) — uuid.UUID is a [16]byte array, and
		// gorm's generic Scan treats array-kind destinations as a multi-row
		// slice target instead of a single scalar, corrupting the value.
		// Row().Scan goes through database/sql directly and respects
		// uuid.UUID's Scanner interface.
		return tx.Raw(
			"SELECT landing_page_id FROM landing_page_experiment_assignments WHERE experiment_id = ? AND visitor_id = ?",
			experimentID, visitorID,
		).Row().Scan(&landingPageID)
	})

	return landingPageID, err
}

// DecideExperimentIfReady checks whether the experiment owning landingPageID
// (if any) is ready to declare a winner, and does so in-tx. Best-effort: a
// failure here must never fail the checkout it's called from, so it swallows
// its own errors after logging them.
func DecideExperimentIfReady(tx *gorm.DB, landingPageID uuid.UUID) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("DecideExperimentIfReady: recovered panic:", r)
		}
	}()

	var lp models.LandingPage
	if err := tx.Select("id", "experiment_id").First(&lp, "id = ?", landingPageID).Error; err != nil || lp.ExperimentID == nil {
		return
	}

	var experiment models.LandingPageExperiment
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&experiment, "id = ? AND status = ?", *lp.ExperimentID, models.ExperimentStatusRunning).Error; err != nil {
		return // already decided by a concurrent order, stopped, or gone
	}

	var sets []models.LandingPage
	if err := tx.Where("experiment_id = ? AND active = ?", experiment.ID, true).
		Order("experiment_position ASC, id ASC").
		Find(&sets).Error; err != nil || len(sets) == 0 {
		return
	}

	setIDs := make([]uuid.UUID, len(sets))
	for i, s := range sets {
		setIDs[i] = s.ID
	}

	conversions, err := txOrderCountsByLandingPageIDs(tx, setIDs)
	if err != nil {
		fmt.Println("DecideExperimentIfReady: order count query failed:", err)
		return
	}
	views, err := txViewCountsByLandingPageIDs(tx, setIDs)
	if err != nil {
		fmt.Println("DecideExperimentIfReady: view count query failed:", err)
		return
	}

	tallies := make([]SetTally, len(sets))
	for i, s := range sets {
		tallies[i] = SetTally{
			LandingPageID: s.ID,
			Position:      s.ExperimentPosition,
			Conversions:   int(conversions[s.ID]),
			Views:         views[s.ID],
		}
	}

	eval := EvaluateExperiment(tallies, experiment.TargetConversions)
	if !eval.Decided {
		return
	}

	if err := tx.Model(&models.LandingPageExperiment{}).
		Where("id = ?", experiment.ID).
		Updates(map[string]interface{}{
			"status":                 models.ExperimentStatusDecided,
			"winner_landing_page_id": eval.WinnerID,
		}).Error; err != nil {
		fmt.Println("DecideExperimentIfReady: failed to record decision:", err)
		return
	}

	if err := tx.Model(&models.LandingPage{}).
		Where("experiment_id = ? AND id <> ?", experiment.ID, eval.WinnerID).
		Update("active", false).Error; err != nil {
		fmt.Println("DecideExperimentIfReady: failed to deactivate losing sets:", err)
		return
	}

	// Every set's cache (IndexLandingPage caches by ID for 10min, unconditionally
	// on a cache hit) must be invalidated now, or a losing set can keep being
	// served stale-active straight from Redis until it expires on its own.
	for _, id := range setIDs {
		InvalidateLandingPageCaches(experiment.ShopID, id)
	}
}

func txOrderCountsByLandingPageIDs(tx *gorm.DB, landingPageIDs []uuid.UUID) (map[uuid.UUID]int64, error) {
	counts := make(map[uuid.UUID]int64, len(landingPageIDs))
	var rows []struct {
		LandingPageID uuid.UUID
		Count         int64
	}
	if err := tx.Table("orders").
		Where("landing_page_id IN ? AND deleted_at IS NULL", landingPageIDs).
		Select("landing_page_id, COUNT(*) AS count").
		Group("landing_page_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		counts[r.LandingPageID] = r.Count
	}
	return counts, nil
}

func txViewCountsByLandingPageIDs(tx *gorm.DB, landingPageIDs []uuid.UUID) (map[uuid.UUID]int64, error) {
	views := make(map[uuid.UUID]int64, len(landingPageIDs))
	var rows []struct {
		EntityID uuid.UUID
		Count    int64
	}
	if err := tx.Table("page_visits").
		Where("page_type = ? AND entity_id IN ?", "landing_page", landingPageIDs).
		Select("entity_id, COUNT(DISTINCT visitor_id) AS count").
		Group("entity_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		views[r.EntityID] = r.Count
	}
	return views, nil
}
