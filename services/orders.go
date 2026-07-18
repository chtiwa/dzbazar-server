package services

import (
	"fmt"

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
