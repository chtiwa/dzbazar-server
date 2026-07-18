package controllers

import (
	"testing"

	"github.com/chtiwa/dzbazar-server/models"
	"github.com/google/uuid"
)

func TestPricedOrderItem(t *testing.T) {
	combo := models.ProductVariantCombination{Price: 2900}

	t.Run("no offerId falls back to combo price", func(t *testing.T) {
		item := OrderItemInput{Quantity: 1}
		unitPrice, lineTotal := pricedOrderItem(combo, item, map[uuid.UUID]models.Offer{})
		if unitPrice != 2900 || lineTotal != 2900 {
			t.Errorf("got (%v, %v), want (2900, 2900)", unitPrice, lineTotal)
		}
	})

	t.Run("unresolvable offerId falls back to combo price", func(t *testing.T) {
		id := uuid.New().String()
		item := OrderItemInput{Quantity: 1, OfferID: &id}
		unitPrice, lineTotal := pricedOrderItem(combo, item, map[uuid.UUID]models.Offer{})
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
		item := OrderItemInput{Quantity: 3, OfferID: &idStr, Price: 1}
		unitPrice, lineTotal := pricedOrderItem(combo, item, offers)
		if unitPrice != 2610 || lineTotal != 7830 {
			t.Errorf("got (%v, %v), want (2610, 7830) — 10%% off 2900, client-sent price of 1 must be ignored", unitPrice, lineTotal)
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
		item := OrderItemInput{Quantity: 2, OfferID: &idStr, Price: 1}
		unitPrice, lineTotal := pricedOrderItem(combo, item, offers)
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
		item := OrderItemInput{Quantity: 3, OfferID: &idStr}
		unitPrice, lineTotal := pricedOrderItem(combo, item, offers)
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
		item := OrderItemInput{Quantity: 5, OfferID: &idStr}
		unitPrice, lineTotal := pricedOrderItem(combo, item, offers)
		if unitPrice != 2900 || lineTotal != 14500 {
			t.Errorf("got (%v, %v), want (2900, 14500) — tampered quantity with no matching tier must not get a discount", unitPrice, lineTotal)
		}
	})
}
