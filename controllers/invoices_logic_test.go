package controllers

import (
	"testing"

	"github.com/chtiwa/dzbazar-server/models"
	"github.com/google/uuid"
)

func TestBilledPlanAmount(t *testing.T) {
	starterID := uuid.New()
	growthID := uuid.New()

	starter := models.Plan{Name: "Starter", Price: 9.99}
	starter.ID = starterID
	growth := models.Plan{Name: "Growth", Price: 19.99}
	growth.ID = growthID

	cases := []struct {
		name          string
		target        models.Plan
		current       models.Plan
		hasActiveSub  bool
		remainingDays int
		want          float64
		wantErr       bool
	}{
		{
			name:         "no subscription at all -> full price",
			target:       starter,
			current:      models.Plan{},
			hasActiveSub: false,
			want:         9.99,
		},
		{
			name:         "expired subscription -> full price, not the diff",
			target:       growth,
			current:      starter,
			hasActiveSub: false, // caller passes false once ExpiresAt has lapsed
			want:         19.99,
		},
		{
			name:          "same plan renewal -> full price, never 0",
			target:        starter,
			current:       starter,
			hasActiveSub:  true,
			remainingDays: 15,
			want:          9.99,
		},
		{
			name:          "upgrade within active period -> prorated diff",
			target:        growth,
			current:       starter,
			hasActiveSub:  true,
			remainingDays: 15, // half a 30-day period
			want:          5.0,
		},
		{
			name:          "upgrade with 0 remaining days -> never negative",
			target:        growth,
			current:       starter,
			hasActiveSub:  true,
			remainingDays: 0,
			want:          0,
		},
		{
			name:          "upgrade with full 30 remaining days -> full diff",
			target:        growth,
			current:       starter,
			hasActiveSub:  true,
			remainingDays: 30,
			want:          10.0,
		},
		{
			name:          "downgrade while active -> rejected",
			target:        starter,
			current:       growth,
			hasActiveSub:  true,
			remainingDays: 15,
			wantErr:       true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := billedPlanAmount(tc.target, tc.current, tc.hasActiveSub, tc.remainingDays)
			if (err != nil) != tc.wantErr {
				t.Fatalf("billedPlanAmount() error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if got != tc.want {
				t.Fatalf("billedPlanAmount() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRoundToCents(t *testing.T) {
	cases := []struct {
		in   float64
		want float64
	}{
		{5.005, 5.01},
		{5.004, 5.0},
		{9.999, 10.0},
		{0, 0},
	}
	for _, tc := range cases {
		if got := roundToCents(tc.in); got != tc.want {
			t.Errorf("roundToCents(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
