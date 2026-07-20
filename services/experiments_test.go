package services

import (
	"testing"

	"github.com/google/uuid"
)

func tally(id string, position, conversions int, views int64) SetTally {
	return SetTally{LandingPageID: uuid.MustParse(id), Position: position, Conversions: conversions, Views: views}
}

func TestEvaluateExperiment(t *testing.T) {
	t.Run("clear leader past target and significant wins", func(t *testing.T) {
		sets := []SetTally{
			tally("11111111-1111-1111-1111-111111111111", 0, 120, 1000),
			tally("22222222-2222-2222-2222-222222222222", 1, 60, 1000),
		}
		eval := EvaluateExperiment(sets, 100)
		if !eval.Decided {
			t.Fatalf("want decided, got not decided (p=%v)", eval.PValue)
		}
		if eval.WinnerID != sets[0].LandingPageID {
			t.Errorf("want set 0 as winner, got %v", eval.WinnerID)
		}
	})

	t.Run("below target floor never decides even if rates differ", func(t *testing.T) {
		sets := []SetTally{
			tally("11111111-1111-1111-1111-111111111111", 0, 5, 100),
			tally("22222222-2222-2222-2222-222222222222", 1, 4, 100),
		}
		eval := EvaluateExperiment(sets, 100)
		if eval.Decided {
			t.Errorf("want not decided — neither set reached the target_conversions floor")
		}
	})

	t.Run("target reached but not statistically significant stays running", func(t *testing.T) {
		sets := []SetTally{
			tally("11111111-1111-1111-1111-111111111111", 0, 55, 1000),
			tally("22222222-2222-2222-2222-222222222222", 1, 45, 1000),
		}
		eval := EvaluateExperiment(sets, 50)
		if eval.Decided {
			t.Errorf("want not decided — 55 vs 45 on n=1000 is noise, raw count alone must not decide")
		}
	})

	t.Run("N sets: leader vs aggregated rest", func(t *testing.T) {
		sets := []SetTally{
			tally("11111111-1111-1111-1111-111111111111", 0, 150, 1000), // clear leader
			tally("22222222-2222-2222-2222-222222222222", 1, 50, 1000),
			tally("33333333-3333-3333-3333-333333333333", 2, 55, 1000),
		}
		eval := EvaluateExperiment(sets, 100)
		if !eval.Decided || eval.WinnerID != sets[0].LandingPageID {
			t.Errorf("want set 0 decided winner vs the other two combined, got decided=%v winner=%v", eval.Decided, eval.WinnerID)
		}
	})

	t.Run("tie on rate breaks by lower position", func(t *testing.T) {
		sets := []SetTally{
			tally("22222222-2222-2222-2222-222222222222", 1, 100, 1000),
			tally("11111111-1111-1111-1111-111111111111", 0, 100, 1000),
		}
		eval := EvaluateExperiment(sets, 100)
		if eval.LeaderID != sets[1].LandingPageID {
			t.Errorf("want set at position 0 to win the tie, got leader %v", eval.LeaderID)
		}
	})

	t.Run("zero-view set never becomes leader but does not crash", func(t *testing.T) {
		sets := []SetTally{
			tally("11111111-1111-1111-1111-111111111111", 0, 0, 0),
			tally("22222222-2222-2222-2222-222222222222", 1, 120, 1000),
		}
		eval := EvaluateExperiment(sets, 100)
		if eval.LeaderID != sets[1].LandingPageID {
			t.Errorf("want the set with actual views to lead, got %v", eval.LeaderID)
		}
	})

	t.Run("empty set list is a no-op", func(t *testing.T) {
		eval := EvaluateExperiment(nil, 100)
		if eval.Decided {
			t.Errorf("want not decided for empty input")
		}
	})
}
