package services

import "testing"

func TestCheckCap(t *testing.T) {
	cases := []struct {
		name    string
		max     int
		count   int64
		wantErr bool
	}{
		{"unlimited", -1, 1_000_000, false},
		{"under cap", 30, 29, false},
		{"at cap", 30, 30, true},
		{"over cap", 30, 31, true},
		{"zero cap blocks immediately", 0, 0, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkCap(tc.max, tc.count)
			if (err != nil) != tc.wantErr {
				t.Fatalf("checkCap(%d, %d) error = %v, wantErr %v", tc.max, tc.count, err, tc.wantErr)
			}
		})
	}
}

// TestExpiredPlanBlocksEverything guards blocker 7: shopSubscription() falls
// back to expiredPlan (not the old, generous unsubscribedPlan) whenever a
// shop has no shop_subscriptions row or a lapsed one. Every zero-valued cap
// must actually block via checkCap, or a shop with no row/an expired row
// would get free access again.
func TestExpiredPlanBlocksEverything(t *testing.T) {
	caps := map[string]int{
		"MaxProducts":       expiredPlan.MaxProducts,
		"MaxOrders":         expiredPlan.MaxOrders,
		"MaxLandingPages":   expiredPlan.MaxLandingPages,
		"MaxUsers":          expiredPlan.MaxUsers,
		"MaxFacebookPixels": expiredPlan.MaxFacebookPixels,
		"MaxTikTokPixels":   expiredPlan.MaxTikTokPixels,
		"CreditsPerMonth":   expiredPlan.CreditsPerMonth,
	}
	for name, max := range caps {
		if err := checkCap(max, 0); err == nil {
			t.Errorf("expiredPlan.%s = %d, checkCap(%d, 0) = nil, want ErrPlanLimitReached", name, max, max)
		}
	}
}
