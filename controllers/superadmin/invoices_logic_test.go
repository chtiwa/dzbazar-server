package superadmin

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestRenewedExpiry(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	planA := uuid.New()
	planB := uuid.New()

	future := now.AddDate(0, 0, 10) // still has 10 days left
	past := now.AddDate(0, 0, -5)   // lapsed 5 days ago

	cases := []struct {
		name             string
		currentPlanID    uuid.UUID
		currentPrice     float64
		targetPlanID     uuid.UUID
		currentExpiresAt *time.Time
		want             time.Time
	}{
		{
			name:             "same-plan renewal, still active -> extends from current ExpiresAt +30d",
			currentPlanID:    planA,
			currentPrice:     1000,
			targetPlanID:     planA,
			currentExpiresAt: &future,
			want:             future.AddDate(0, 0, 30),
		},
		{
			name:             "same-plan renewal, already expired -> extends from now +30d, not from the stale date",
			currentPlanID:    planA,
			currentPrice:     1000,
			targetPlanID:     planA,
			currentExpiresAt: &past,
			want:             now.AddDate(0, 0, 30),
		},
		{
			name:             "same-plan renewal, no expiry on file -> now +30d",
			currentPlanID:    planA,
			currentPrice:     1000,
			targetPlanID:     planA,
			currentExpiresAt: nil,
			want:             now.AddDate(0, 0, 30),
		},
		{
			name:             "free plan active -> different plan -> now +30d",
			currentPlanID:    planA,
			currentPrice:     0,
			targetPlanID:     planB,
			currentExpiresAt: &future,
			want:             now.AddDate(0, 0, 30),
		},
		{
			name:             "paid plan expired -> different plan -> now +30d",
			currentPlanID:    planA,
			currentPrice:     1000,
			targetPlanID:     planB,
			currentExpiresAt: &past,
			want:             now.AddDate(0, 0, 30),
		},
		{
			name:             "paid plan active -> upgrade -> keeps existing ExpiresAt untouched",
			currentPlanID:    planA,
			currentPrice:     1000,
			targetPlanID:     planB,
			currentExpiresAt: &future,
			want:             future,
		},
		{
			name:             "upgrade with no existing expiry -> falls back to now +30d",
			currentPlanID:    planA,
			currentPrice:     1000,
			targetPlanID:     planB,
			currentExpiresAt: nil,
			want:             now.AddDate(0, 0, 30),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := renewedExpiry(tc.currentPlanID, tc.targetPlanID, tc.currentPrice, tc.currentExpiresAt, now)
			if !got.Equal(tc.want) {
				t.Fatalf("renewedExpiry() = %v, want %v", got, tc.want)
			}
		})
	}
}
