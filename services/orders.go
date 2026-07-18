package services

import (
	"fmt"
	"math"

	"github.com/chtiwa/dzbazar-server/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ResolveShipping picks the authoritative shipping price off a shop's
// DeliveryRate row for the wilaya being shipped to — never trust a
// client-sent shippingPrice. Fails closed if the wilaya or the requested
// method is disabled.
func ResolveShipping(rate models.DeliveryRate, method string) (float64, error) {
	if !rate.IsActive {
		return 0, fmt.Errorf("delivery is inactive for wilaya %d", rate.WilayaID)
	}
	if method == "Domicile" {
		if !rate.HasDoorstep {
			return 0, fmt.Errorf("doorstep delivery unavailable for wilaya %d", rate.WilayaID)
		}
		return rate.DoorstepRate, nil
	}
	if !rate.HasStopdesk {
		return 0, fmt.Errorf("stopdesk delivery unavailable for wilaya %d", rate.WilayaID)
	}
	return rate.StopdeskRate, nil
}

// FetchCombinationsForShop batch-loads ProductVariantCombination rows by id,
// scoped to shopID via a join to products, so a combination id belonging to
// another shop (or one that doesn't exist) can never be used to price an
// order item. Callers must check the returned map for each requested id —
// a miss means the request referenced an unknown/foreign combination and
// the order should be rejected, not silently skipped.
func FetchCombinationsForShop(tx *gorm.DB, shopID uuid.UUID, comboIDs []uuid.UUID) (map[uuid.UUID]models.ProductVariantCombination, error) {
	var combos []models.ProductVariantCombination
	if err := tx.
		Joins("JOIN products ON products.id = product_variant_combinations.product_id").
		Where("product_variant_combinations.id IN ? AND products.shop_id = ?", comboIDs, shopID).
		Find(&combos).Error; err != nil {
		return nil, err
	}

	comboByID := make(map[uuid.UUID]models.ProductVariantCombination, len(combos))
	for _, combo := range combos {
		comboByID[combo.ID] = combo
	}
	return comboByID, nil
}

// DecrementOrderItemsStock reduces the stock quantity of each ordered variant
// combination by the quantity ordered. Called once, when an order transitions
// to "shipped" for the first time. Returns the first write error encountered
// so a failed decrement can never pass silently as a successful ship.
func DecrementOrderItemsStock(tx *gorm.DB, items []models.OrderItem) error {
	for _, item := range items {
		if err := tx.Model(&models.ProductVariantCombination{}).
			Where("id = ?", item.ProductVariantCombinationID).
			UpdateColumn("quantity", gorm.Expr("quantity - ?", item.Quantity)).Error; err != nil {
			return err
		}
	}
	return nil
}

// FetchPublishedOffersForShop batch-fetches every offer referenced by
// offerIDs, scoped to this shop and to currently-published offers only. A
// nil/unparseable/draft/archived/foreign-shop offer id simply won't be in
// the map, so PricedOrderItem falls back to the combo's own price.
func FetchPublishedOffersForShop(tx *gorm.DB, shopID uuid.UUID, offerIDs []*string) (map[uuid.UUID]models.Offer, error) {
	ids := make([]uuid.UUID, 0, len(offerIDs))
	for _, raw := range offerIDs {
		if raw == nil {
			continue
		}
		id, err := uuid.Parse(*raw)
		if err != nil {
			continue
		}
		ids = append(ids, id)
	}
	byID := map[uuid.UUID]models.Offer{}
	if len(ids) == 0 {
		return byID, nil
	}
	var offers []models.Offer
	if err := tx.Where("id IN ? AND shop_id = ? AND status = 'published' AND deleted_at IS NULL", ids, shopID).
		Find(&offers).Error; err != nil {
		return nil, err
	}
	for _, o := range offers {
		byID[o.ID] = o
	}
	return byID, nil
}

// PackageForQuantity finds the tier matching an exact requested quantity.
// Shared by offer evaluation (to display) and order creation (to reprice
// server-side) so the two can never compute a different price for the same
// tier.
func PackageForQuantity(pkgs []models.OfferQuantityPackage, quantity int) (models.OfferQuantityPackage, bool) {
	for _, p := range pkgs {
		if p.Quantity == quantity {
			return p, true
		}
	}
	return models.OfferQuantityPackage{}, false
}

// ComputeOfferedPrice applies an offer's discount rule to a base price.
func ComputeOfferedPrice(basePrice float64, discountType string, discountValue float64) float64 {
	switch discountType {
	case "percent":
		return math.Round(basePrice * (1 - discountValue/100))
	case "fixed":
		return math.Max(0, basePrice-discountValue)
	case "override_price":
		return discountValue
	}
	return basePrice
}

// PricedOrderItem is the single authority for what a checkout line actually
// costs. combo.Price is the always-safe default; an offerID that resolves to
// a published offer in offerByID can only lower/override it through the
// offer's own server-known rule — never from a client-sent price.
//
// It returns both the per-unit price to store on the OrderItem row and the
// exact line total to add to the order's total — these can differ for a
// quantity_upsell tier, since TotalPrice doesn't always divide evenly by
// quantity (e.g. 7700/3 = 2566.67). Charging round(unitPrice)*quantity would
// drift from the merchant's configured total (7701 instead of 7700); the
// line total must always be the exact configured amount, the per-unit price
// is a rounded display value only.
func PricedOrderItem(combo models.ProductVariantCombination, quantity uint, offerID *string, offerByID map[uuid.UUID]models.Offer) (unitPrice float64, lineTotal float64) {
	fallback := func() (float64, float64) { return combo.Price, combo.Price * float64(quantity) }

	if offerID == nil {
		return fallback()
	}
	id, err := uuid.Parse(*offerID)
	if err != nil {
		return fallback()
	}
	offer, ok := offerByID[id]
	if !ok {
		return fallback()
	}
	if offer.OfferType != nil && *offer.OfferType == "quantity_upsell" {
		pkg, found := PackageForQuantity(offer.QuantityPackages, int(quantity))
		if !found {
			return fallback()
		}
		return math.Round(pkg.TotalPrice / float64(quantity)), pkg.TotalPrice
	}
	unitPrice = ComputeOfferedPrice(combo.Price, offer.DiscountType, offer.DiscountValue)
	return unitPrice, unitPrice * float64(quantity)
}
