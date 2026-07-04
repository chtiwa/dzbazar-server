package controllers

import (
	"testing"

	"github.com/chtiwa/dzbazar-server/models"
)

func TestMatchesPredicate(t *testing.T) {
	req := EvaluateOffersRequest{
		ProductID:        "prod-123",
		LandingPageID:    strPtr("lp-456"),
		SelectedVariants: []string{"combo-A", "combo-B"},
		Quantity:         3,
		UTMSource:        "facebook",
		Wilaya:           intPtr(16),
	}

	cases := []struct {
		name string
		pred models.OfferPredicate
		want bool
	}{
		{
			name: "current_product eq match",
			pred: models.OfferPredicate{Field: "current_product", Op: "eq", Value: "prod-123"},
			want: true,
		},
		{
			name: "current_product eq mismatch",
			pred: models.OfferPredicate{Field: "current_product", Op: "eq", Value: "other"},
			want: false,
		},
		{
			name: "current_landing_page eq match",
			pred: models.OfferPredicate{Field: "current_landing_page", Op: "eq", Value: "lp-456"},
			want: true,
		},
		{
			name: "selected_variant in match",
			pred: models.OfferPredicate{Field: "selected_variant", Op: "in", Value: []interface{}{"combo-A", "combo-C"}},
			want: true,
		},
		{
			name: "selected_variant in no match",
			pred: models.OfferPredicate{Field: "selected_variant", Op: "in", Value: []interface{}{"combo-X"}},
			want: false,
		},
		{
			name: "selected_quantity gte satisfied",
			pred: models.OfferPredicate{Field: "selected_quantity", Op: "gte", Value: float64(2)},
			want: true,
		},
		{
			name: "selected_quantity gte not satisfied",
			pred: models.OfferPredicate{Field: "selected_quantity", Op: "gte", Value: float64(5)},
			want: false,
		},
		{
			name: "customer_wilaya in match",
			pred: models.OfferPredicate{Field: "customer_wilaya", Op: "in", Value: []interface{}{float64(16), float64(31)}},
			want: true,
		},
		{
			name: "customer_wilaya in no match",
			pred: models.OfferPredicate{Field: "customer_wilaya", Op: "in", Value: []interface{}{float64(9)}},
			want: false,
		},
		{
			name: "utm_source eq match",
			pred: models.OfferPredicate{Field: "utm_source", Op: "eq", Value: "facebook"},
			want: true,
		},
		{
			name: "utm_source eq mismatch",
			pred: models.OfferPredicate{Field: "utm_source", Op: "eq", Value: "tiktok"},
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := matchesPredicate(tc.pred, req)
			if got != tc.want {
				t.Errorf("matchesPredicate(%+v) = %v, want %v", tc.pred, got, tc.want)
			}
		})
	}
}

func TestMatchesConditions_EmptyAlwaysTrue(t *testing.T) {
	conds := models.OfferConditions{Match: "all", Predicates: []models.OfferPredicate{}}
	req := EvaluateOffersRequest{ProductID: "anything"}
	if !matchesConditions(conds, req) {
		t.Error("empty predicates should always match")
	}
}

func TestComputeOfferedPrice(t *testing.T) {
	cases := []struct {
		base, value float64
		dtype       string
		want        float64
	}{
		{2500, 10, "percent", 2250},
		{2500, 500, "fixed", 2000},
		{2500, 900, "override_price", 900},
		{2500, 0, "percent", 2500},
	}
	for _, tc := range cases {
		got := computeOfferedPrice(tc.base, tc.dtype, tc.value)
		if got != tc.want {
			t.Errorf("computeOfferedPrice(%v, %q, %v) = %v, want %v", tc.base, tc.dtype, tc.value, got, tc.want)
		}
	}
}

func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }
