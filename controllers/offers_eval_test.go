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

func TestDeriveActionFromOfferType(t *testing.T) {
	cases := []struct {
		offerType  string
		wantAction string
		wantOK     bool
	}{
		{"quantity_upsell", "mutate_qty", true},
		{"variant_upsell", "replace", true},
		{"cross_sell", "append", true},
		{"something_else", "", false},
		{"", "", false},
	}
	for _, tc := range cases {
		action, ok := deriveActionFromOfferType(tc.offerType)
		if ok != tc.wantOK || action != tc.wantAction {
			t.Errorf("deriveActionFromOfferType(%q) = (%q, %v), want (%q, %v)", tc.offerType, action, ok, tc.wantAction, tc.wantOK)
		}
	}
}

func TestValidateQuantityPackages(t *testing.T) {
	cases := []struct {
		name    string
		pkgs    []models.OfferQuantityPackage
		wantErr bool
	}{
		{"empty rejected", []models.OfferQuantityPackage{}, true},
		{
			"valid tiers",
			[]models.OfferQuantityPackage{{Quantity: 1, TotalPrice: 2900}, {Quantity: 2, TotalPrice: 5300}, {Quantity: 3, TotalPrice: 7500}},
			false,
		},
		{"zero quantity rejected", []models.OfferQuantityPackage{{Quantity: 0, TotalPrice: 100}}, true},
		{"negative price rejected", []models.OfferQuantityPackage{{Quantity: 1, TotalPrice: -1}}, true},
		{
			"duplicate quantity rejected",
			[]models.OfferQuantityPackage{{Quantity: 2, TotalPrice: 100}, {Quantity: 2, TotalPrice: 200}},
			true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateQuantityPackages(tc.pkgs)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateQuantityPackages(%+v) error = %v, wantErr %v", tc.pkgs, err, tc.wantErr)
			}
		})
	}
}

func TestValidateOfferTypeConsistency(t *testing.T) {
	quantityUpsell := "quantity_upsell"
	crossSell := "cross_sell"
	invalid := "not_a_real_type"

	// quantity_upsell with no packages must fail.
	if err := validateOfferTypeConsistency(&models.Offer{OfferType: &quantityUpsell}); err == nil {
		t.Error("expected error for quantity_upsell with no packages")
	}
	// quantity_upsell with valid packages must pass.
	if err := validateOfferTypeConsistency(&models.Offer{
		OfferType:        &quantityUpsell,
		QuantityPackages: []models.OfferQuantityPackage{{Quantity: 1, TotalPrice: 2900}},
	}); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	// cross_sell never needs packages.
	if err := validateOfferTypeConsistency(&models.Offer{OfferType: &crossSell}); err != nil {
		t.Errorf("expected no error for cross_sell, got %v", err)
	}
	// nil OfferType (legacy offer) always passes — Action alone governs it.
	if err := validateOfferTypeConsistency(&models.Offer{}); err != nil {
		t.Errorf("expected no error for legacy offer with nil OfferType, got %v", err)
	}
	// unknown offerType must fail.
	if err := validateOfferTypeConsistency(&models.Offer{OfferType: &invalid}); err == nil {
		t.Error("expected error for invalid offerType")
	}
}

func TestPackageForQuantity(t *testing.T) {
	pkgs := []models.OfferQuantityPackage{
		{Quantity: 1, TotalPrice: 2900},
		{Quantity: 2, TotalPrice: 5300},
		{Quantity: 3, TotalPrice: 7500},
	}

	if pkg, ok := packageForQuantity(pkgs, 2); !ok || pkg.TotalPrice != 5300 {
		t.Errorf("packageForQuantity(2) = (%+v, %v), want (TotalPrice:5300, true)", pkg, ok)
	}
	if _, ok := packageForQuantity(pkgs, 5); ok {
		t.Error("packageForQuantity(5) should not match — no tier configured for that quantity")
	}
}

func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }
