package services

import "testing"

func TestTwoProportionZTest(t *testing.T) {
	t.Run("close rates with modest sample are not significant", func(t *testing.T) {
		_, p := TwoProportionZTest(55, 1000, 45, 1000)
		if p <= 0.05 {
			t.Errorf("got p=%v, want > 0.05 — 55/1000 vs 45/1000 should read as noise", p)
		}
	})

	t.Run("clearly different rates are significant", func(t *testing.T) {
		_, p := TwoProportionZTest(120, 1000, 60, 1000)
		if p >= 0.05 {
			t.Errorf("got p=%v, want < 0.05 — 12%% vs 6%% on n=1000 each is a real gap", p)
		}
	})

	t.Run("zero views on either side is treated as inconclusive", func(t *testing.T) {
		_, p := TwoProportionZTest(0, 0, 10, 100)
		if p != 1 {
			t.Errorf("got p=%v, want 1 (no evidence, never a false winner)", p)
		}
	})

	t.Run("identical rates yield p near 1", func(t *testing.T) {
		_, p := TwoProportionZTest(50, 1000, 50, 1000)
		if p < 0.9 {
			t.Errorf("got p=%v, want close to 1 for identical rates", p)
		}
	})
}
