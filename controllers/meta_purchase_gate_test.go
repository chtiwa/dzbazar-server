package controllers

import (
	"testing"
	"time"
)

func TestShouldSendMetaPurchase(t *testing.T) {
	now := time.Now()

	cases := []struct {
		name             string
		status           string
		conversionSource string
		fullName         string
		isHidden         bool
		sentAt           *time.Time
		want             bool
	}{
		{"confirmed facebook fresh", "Confirmé", "facebook", "Ali Ben", false, nil, true},
		{"already sent", "Confirmé", "facebook", "Ali Ben", false, &now, false},
		{"hidden/fake order", "Confirmé", "facebook", "Ali Ben", true, nil, false},
		{"not confirmed yet", "En attente", "facebook", "Ali Ben", false, nil, false},
		{"cancelled", "Annulé", "facebook", "Ali Ben", false, nil, false},
		{"non-facebook source", "Confirmé", "organic", "Ali Ben", false, nil, false},
		{"test order", "Confirmé", "facebook", "Test User", false, nil, false},
	}

	for _, c := range cases {
		if got := shouldSendMetaPurchase(c.status, c.conversionSource, c.fullName, c.isHidden, c.sentAt); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}
