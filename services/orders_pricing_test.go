package services

import (
	"testing"

	"github.com/chtiwa/dzbazar-server/models"
	"github.com/google/uuid"
)

func TestPricedOrderItem(t *testing.T) {
	combo := models.ProductVariantCombination{Price: 2900}

	t.Run("no offerId falls back to combo price", func(t *testing.T) {
		unitPrice, lineTotal := PricedOrderItem(combo, 1, nil, map[uuid.UUID]models.Offer{})
		if unitPrice != 2900 || lineTotal != 2900 {
			t.Errorf("got (%v, %v), want (2900, 2900)", unitPrice, lineTotal)
		}
	})

	t.Run("unresolvable offerId falls back to combo price", func(t *testing.T) {
		id := uuid.New().String()
		unitPrice, lineTotal := PricedOrderItem(combo, 1, &id, map[uuid.UUID]models.Offer{})
		if unitPrice != 2900 || lineTotal != 2900 {
			t.Errorf("got (%v, %v), want (2900, 2900) — unresolved offer must never change price", unitPrice, lineTotal)
		}
	})

	t.Run("percent discount offer recomputed server-side", func(t *testing.T) {
		offerID := uuid.New()
		idStr := offerID.String()
		offers := map[uuid.UUID]models.Offer{
			offerID: {DiscountType: "percent", DiscountValue: 10},
		}
		unitPrice, lineTotal := PricedOrderItem(combo, 3, &idStr, offers)
		if unitPrice != 2610 || lineTotal != 7830 {
			t.Errorf("got (%v, %v), want (2610, 7830) — 10%% off 2900, client-sent price must be ignored", unitPrice, lineTotal)
		}
	})

	t.Run("quantity package priced by exact quantity match, unit price rounded", func(t *testing.T) {
		offerID := uuid.New()
		idStr := offerID.String()
		quantityUpsell := "quantity_upsell"
		offers := map[uuid.UUID]models.Offer{
			offerID: {
				OfferType: &quantityUpsell,
				QuantityPackages: []models.OfferQuantityPackage{
					{Quantity: 1, TotalPrice: 2900},
					{Quantity: 2, TotalPrice: 5300},
					{Quantity: 3, TotalPrice: 7500},
				},
			},
		}
		unitPrice, lineTotal := PricedOrderItem(combo, 2, &idStr, offers)
		if unitPrice != 2650 || lineTotal != 5300 {
			t.Errorf("got (%v, %v), want (2650, 5300) — 5300/2 per-unit for the 2-pack tier, exact total", unitPrice, lineTotal)
		}
	})

	t.Run("non-divisible tier: line total stays exact even though unit price rounds", func(t *testing.T) {
		offerID := uuid.New()
		idStr := offerID.String()
		quantityUpsell := "quantity_upsell"
		offers := map[uuid.UUID]models.Offer{
			offerID: {
				OfferType:        &quantityUpsell,
				QuantityPackages: []models.OfferQuantityPackage{{Quantity: 3, TotalPrice: 7700}},
			},
		}
		unitPrice, lineTotal := PricedOrderItem(combo, 3, &idStr, offers)
		// 7700/3 = 2566.67 -> rounds to 2567 for display, but the charged
		// total must stay exactly 7700, not 2567*3=7701 (the regression this
		// test guards: reported as "8501 instead of 8500" with an 800 DA
		// shipping charge added on top of a 7700 DA / 3-unit tier).
		if unitPrice != 2567 {
			t.Errorf("unitPrice = %v, want 2567 (rounded display value)", unitPrice)
		}
		if lineTotal != 7700 {
			t.Errorf("lineTotal = %v, want exactly 7700 (merchant's configured total, no rounding drift)", lineTotal)
		}
	})

	t.Run("quantity mismatching any tier falls back to combo price", func(t *testing.T) {
		offerID := uuid.New()
		idStr := offerID.String()
		quantityUpsell := "quantity_upsell"
		offers := map[uuid.UUID]models.Offer{
			offerID: {
				OfferType:        &quantityUpsell,
				QuantityPackages: []models.OfferQuantityPackage{{Quantity: 1, TotalPrice: 2900}},
			},
		}
		unitPrice, lineTotal := PricedOrderItem(combo, 5, &idStr, offers)
		if unitPrice != 2900 || lineTotal != 14500 {
			t.Errorf("got (%v, %v), want (2900, 14500) — tampered quantity with no matching tier must not get a discount", unitPrice, lineTotal)
		}
	})
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
		got := ComputeOfferedPrice(tc.base, tc.dtype, tc.value)
		if got != tc.want {
			t.Errorf("ComputeOfferedPrice(%v, %q, %v) = %v, want %v", tc.base, tc.dtype, tc.value, got, tc.want)
		}
	}
}

func TestPackageForQuantity(t *testing.T) {
	pkgs := []models.OfferQuantityPackage{
		{Quantity: 1, TotalPrice: 2900},
		{Quantity: 2, TotalPrice: 5300},
		{Quantity: 3, TotalPrice: 7500},
	}

	if pkg, ok := PackageForQuantity(pkgs, 2); !ok || pkg.TotalPrice != 5300 {
		t.Errorf("PackageForQuantity(2) = (%+v, %v), want (TotalPrice:5300, true)", pkg, ok)
	}
	if _, ok := PackageForQuantity(pkgs, 5); ok {
		t.Error("PackageForQuantity(5) should not match — no tier configured for that quantity")
	}
}
