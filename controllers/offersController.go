package controllers

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// offerTypeToAction is the fixed, one-way mapping from the merchant-facing
// business label to the cart mechanic. OfferType is always the source of
// truth when present — Action is derived from it, never set independently,
// so the two fields can never disagree.
var offerTypeToAction = map[string]string{
	"quantity_upsell": "mutate_qty",
	"variant_upsell":  "replace",
	"cross_sell":      "append",
}

func deriveActionFromOfferType(offerType string) (string, bool) {
	action, ok := offerTypeToAction[offerType]
	return action, ok
}

// validateQuantityPackages enforces the invariants a quantity_upsell offer's
// tiers must hold: at least one tier, positive quantities, non-negative
// totals, no duplicate quantity across tiers.
func validateQuantityPackages(pkgs []models.OfferQuantityPackage) error {
	if len(pkgs) == 0 {
		return fmt.Errorf("quantityPackages is required for offerType quantity_upsell")
	}
	seen := make(map[int]bool, len(pkgs))
	for _, p := range pkgs {
		if p.Quantity < 1 {
			return fmt.Errorf("package quantity must be at least 1")
		}
		if p.TotalPrice < 0 {
			return fmt.Errorf("package totalPrice cannot be negative")
		}
		if seen[p.Quantity] {
			return fmt.Errorf("duplicate package quantity: %d", p.Quantity)
		}
		seen[p.Quantity] = true
	}
	return nil
}

// validateOfferTypeConsistency checks the business-label fields after they've
// been merged onto the offer struct. Offers without an OfferType (every offer
// created before this field existed) are untouched — Action alone still
// governs them.
func validateOfferTypeConsistency(offer *models.Offer) error {
	if offer.OfferType == nil {
		return nil
	}
	switch *offer.OfferType {
	case "quantity_upsell":
		return validateQuantityPackages(offer.QuantityPackages)
	case "variant_upsell", "cross_sell":
		return nil
	default:
		return fmt.Errorf("invalid offerType: %s", *offer.OfferType)
	}
}

// ─── Admin: CRUD ─────────────────────────────────────────────────────────────

type offerBody struct {
	InternalName      *string                  `json:"internalName"`
	Status            *string                  `json:"status"`
	Action            *string                  `json:"action"`
	OfferType         *string                  `json:"offerType"` // quantity_upsell|variant_upsell|cross_sell — derives Action when present
	QuantityPackages  *[]models.OfferQuantityPackage `json:"quantityPackages"`
	TriggerProductID  *string                  `json:"triggerProductId"`
	LandingPageID     *string                  `json:"landingPageId"` // omit or null = base offer
	OfferProductID    *string                  `json:"offerProductId"`
	OfferVariantIDs   []string                 `json:"offerVariantIds"`
	QuantityRule      *int                     `json:"quantityRule"`
	DiscountType      *string                  `json:"discountType"`
	DiscountValue     *float64                 `json:"discountValue"`
	Headline          *string                  `json:"headline"`
	Subheadline       *string                  `json:"subheadline"`
	ButtonText        *string                  `json:"buttonText"`
	MediaURL          *string                  `json:"mediaUrl"`
	Placement         *string                  `json:"placement"`
	Priority          *int                     `json:"priority"`
	Conditions        *models.OfferConditions  `json:"conditions"`
	InventoryBehavior *string                  `json:"inventoryBehavior"`
	AnalyticsTag      *string                  `json:"analyticsTag"`
	StartAt           *time.Time               `json:"startAt"`
	EndAt             *time.Time               `json:"endAt"`
}

func CreateOffer(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	var body offerBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid request body", "error": err.Error()})
		return
	}

	if body.OfferType != nil {
		action, ok := deriveActionFromOfferType(*body.OfferType)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid offerType"})
			return
		}
		body.Action = &action
	}

	if body.InternalName == nil || *body.InternalName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "internalName is required"})
		return
	}
	if body.Action == nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "action is required"})
		return
	}
	if body.TriggerProductID == nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "triggerProductId is required"})
		return
	}
	if body.OfferProductID == nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "offerProductId is required"})
		return
	}
	if body.Headline == nil || *body.Headline == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "headline is required"})
		return
	}
	if body.Placement == nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "placement is required"})
		return
	}

	triggerProductID, err := uuid.Parse(*body.TriggerProductID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid triggerProductId"})
		return
	}
	offerProductID, err := uuid.Parse(*body.OfferProductID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid offerProductId"})
		return
	}

	// Validate both products belong to this shop.
	triggerIDs, err := productsOwnedByShop(shopID, []uuid.UUID{triggerProductID})
	if err != nil || len(triggerIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "triggerProductId not found in this shop"})
		return
	}
	offerIDs, err := productsOwnedByShop(shopID, []uuid.UUID{offerProductID})
	if err != nil || len(offerIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "offerProductId not found in this shop"})
		return
	}

	variantIDs, err := parseUUIDs(body.OfferVariantIDs)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid offerVariantIds"})
		return
	}

	offer := models.Offer{
		ShopID:           shopID,
		InternalName:     *body.InternalName,
		Status:           "draft",
		Action:           *body.Action,
		TriggerProductID: triggerProductID,
		OfferProductID:   offerProductID,
		OfferVariantIDs:  variantIDs,
		Headline:         *body.Headline,
		Placement:        *body.Placement,
		Conditions:       models.OfferConditions{Match: "all", Predicates: []models.OfferPredicate{}},
		ButtonText:       "Add to my order",
		DiscountType:     "percent",
		Priority:         100,
		InventoryBehavior: "skip_when_oos",
		QuantityRule:     1,
	}

	applyOfferBodyFields(&offer, body)

	if err := validateOfferTypeConsistency(&offer); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}

	if err := initializers.DB.Create(&offer).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to create offer", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Offer created", "data": offer})
}

func GetOffersByShop(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	query := initializers.DB.Where("shop_id = ? AND deleted_at IS NULL", shopID).
		Preload("TriggerProduct").
		Order("created_at DESC")

	if pid := c.Query("product_id"); pid != "" {
		query = query.Where("trigger_product_id = ?", pid)
	}
	if lpid := c.Query("landing_page_id"); lpid != "" {
		query = query.Where("landing_page_id = ?", lpid)
	}

	var offers []models.Offer
	if err := query.Find(&offers).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to retrieve offers", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": offers})
}

func GetOffer(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}
	offerID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid offer ID"})
		return
	}

	var offer models.Offer
	if err := initializers.DB.
		Preload("TriggerProduct").
		Preload("OfferProduct").
		Where("id = ? AND shop_id = ? AND deleted_at IS NULL", offerID, shopID).
		First(&offer).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Offer not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": offer})
}

func UpdateOffer(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}
	offerID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid offer ID"})
		return
	}

	var offer models.Offer
	if err := initializers.DB.Where("id = ? AND shop_id = ? AND deleted_at IS NULL", offerID, shopID).First(&offer).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Offer not found"})
		return
	}

	var body offerBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid request body", "error": err.Error()})
		return
	}

	if body.OfferType != nil {
		action, ok := deriveActionFromOfferType(*body.OfferType)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid offerType"})
			return
		}
		body.Action = &action
	}

	if body.TriggerProductID != nil {
		id, err := uuid.Parse(*body.TriggerProductID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid triggerProductId"})
			return
		}
		owned, err := productsOwnedByShop(shopID, []uuid.UUID{id})
		if err != nil || len(owned) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "triggerProductId not found in this shop"})
			return
		}
		offer.TriggerProductID = id
	}
	if body.OfferProductID != nil {
		id, err := uuid.Parse(*body.OfferProductID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid offerProductId"})
			return
		}
		owned, err := productsOwnedByShop(shopID, []uuid.UUID{id})
		if err != nil || len(owned) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "offerProductId not found in this shop"})
			return
		}
		offer.OfferProductID = id
	}
	if body.OfferVariantIDs != nil {
		ids, err := parseUUIDs(body.OfferVariantIDs)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid offerVariantIds"})
			return
		}
		offer.OfferVariantIDs = ids
	}

	applyOfferBodyFields(&offer, body)

	if err := validateOfferTypeConsistency(&offer); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}

	if err := initializers.DB.Save(&offer).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to update offer", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Offer updated", "data": offer})
}

func PublishOffer(c *gin.Context) {
	setOfferStatus(c, "published")
}

func ArchiveOffer(c *gin.Context) {
	setOfferStatus(c, "archived")
}

func setOfferStatus(c *gin.Context, status string) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}
	offerID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid offer ID"})
		return
	}

	result := initializers.DB.Model(&models.Offer{}).
		Where("id = ? AND shop_id = ? AND deleted_at IS NULL", offerID, shopID).
		Update("status", status)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to update offer status", "error": result.Error.Error()})
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Offer not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Offer " + status})
}

func DeleteOffer(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}
	offerID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid offer ID"})
		return
	}

	result := initializers.DB.Where("id = ? AND shop_id = ?", offerID, shopID).Delete(&models.Offer{})
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to delete offer", "error": result.Error.Error()})
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Offer not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Offer deleted"})
}

// ─── Admin: Overrides ─────────────────────────────────────────────────────────

type overrideBody struct {
	Enabled        *bool    `json:"enabled"`
	Headline       *string  `json:"headline"`
	Subheadline    *string  `json:"subheadline"`
	ButtonText     *string  `json:"buttonText"`
	OfferProductID *string  `json:"offerProductId"`
	DiscountType   *string  `json:"discountType"`
	DiscountValue  *float64 `json:"discountValue"`
	Placement      *string  `json:"placement"`
}

func UpsertOfferOverride(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}
	offerID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid offer ID"})
		return
	}
	lpID, err := uuid.Parse(c.Param("landingPageId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid landing page ID"})
		return
	}

	// Verify offer belongs to shop.
	var count int64
	initializers.DB.Model(&models.Offer{}).Where("id = ? AND shop_id = ? AND deleted_at IS NULL", offerID, shopID).Count(&count)
	if count == 0 {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Offer not found"})
		return
	}

	var body overrideBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid request body", "error": err.Error()})
		return
	}

	override := models.OfferPageOverride{
		ShopID:        shopID,
		OfferID:       offerID,
		LandingPageID: lpID,
		Enabled:       true,
	}
	if body.Enabled != nil {
		override.Enabled = *body.Enabled
	}
	override.Headline = body.Headline
	override.Subheadline = body.Subheadline
	override.ButtonText = body.ButtonText
	override.DiscountType = body.DiscountType
	override.DiscountValue = body.DiscountValue
	override.Placement = body.Placement
	if body.OfferProductID != nil {
		pid, err := uuid.Parse(*body.OfferProductID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid offerProductId"})
			return
		}
		override.OfferProductID = &pid
	}

	// Upsert by (offer_id, landing_page_id).
	err = initializers.DB.
		Where(models.OfferPageOverride{OfferID: offerID, LandingPageID: lpID}).
		Assign(override).
		FirstOrCreate(&override).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to save override", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Override saved", "data": override})
}

func DeleteOfferOverride(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}
	offerID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid offer ID"})
		return
	}
	lpID, err := uuid.Parse(c.Param("landingPageId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid landing page ID"})
		return
	}

	initializers.DB.Where("shop_id = ? AND offer_id = ? AND landing_page_id = ?", shopID, offerID, lpID).
		Delete(&models.OfferPageOverride{})

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Override deleted"})
}

// ─── Public: Evaluate ─────────────────────────────────────────────────────────

type EvaluateOffersRequest struct {
	PageType         string   `json:"pageType"`         // "product"|"landing_page"
	ProductID        string   `json:"productId"`
	LandingPageID    *string  `json:"landingPageId"`
	SelectedVariants []string `json:"selectedVariants"` // ProductVariantCombination IDs
	Quantity         int      `json:"quantity"`
	UTMSource        string   `json:"utmSource"`
	Wilaya           *int     `json:"wilaya"`
	// Combination IDs already in the pending order (for exclusion).
	ExistingVariants []string `json:"existingVariants"`
}

type OfferedVariant struct {
	ID                uuid.UUID `json:"id"`
	OfferedPrice      float64   `json:"offeredPrice"`
	Available         bool      `json:"available"`
	CombinationString string    `json:"combinationString"`
	ProductID         uuid.UUID `json:"productId"`
}

type OfferResult struct {
	ID            uuid.UUID        `json:"id"`
	Action        string           `json:"action"`
	OfferType     string           `json:"offerType"` // "" for offers created before OfferType existed
	Placement     string           `json:"placement"`
	AnchorVariant *uuid.UUID       `json:"anchorVariant"` // for replace action
	Headline      string           `json:"headline"`
	Subheadline   string           `json:"subheadline"`
	ButtonText    string           `json:"buttonText"`
	MediaURL      string           `json:"mediaUrl"`
	AnalyticsTag  string           `json:"analyticsTag"`
	QuantityRule  int              `json:"quantityRule"`
	Variants      []OfferedVariant `json:"variants"`
	// Packages is populated only for offerType=="quantity_upsell" — the
	// selectable qty/price tiers. Variants stays empty for that offer type,
	// since packages apply to whichever variant the customer already picked,
	// not to a merchant-preselected combination.
	Packages []models.OfferQuantityPackage `json:"packages"`
}

func EvaluateOffersPublic(c *gin.Context) {
	slug := c.Param("slug")
	var shop models.Shop
	if err := initializers.DB.Select("id").Where("slug = ?", slug).First(&shop).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Shop not found"})
		return
	}

	var req EvaluateOffersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid request body", "error": err.Error()})
		return
	}

	productID, err := uuid.Parse(req.ProductID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid productId"})
		return
	}

	results, err := resolveOffers(shop.ID, productID, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to evaluate offers", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": results})
}

// resolveOffers is the pure evaluation pipeline — testable without HTTP context.
func resolveOffers(shopID, productID uuid.UUID, req EvaluateOffersRequest) ([]OfferResult, error) {
	now := time.Now()

	// 1. Fetch published offers for this product, in a fixed deterministic
	// order. Winner-per-placement selection below relies on this order
	// (first offer seen per placement wins) instead of Go map iteration,
	// which is intentionally randomized and must never be a tie-break.
	var offers []models.Offer
	if err := initializers.DB.
		Where("shop_id = ? AND trigger_product_id = ? AND status = 'published' AND deleted_at IS NULL AND (start_at IS NULL OR start_at <= ?) AND (end_at IS NULL OR end_at >= ?)",
			shopID, productID, now, now).
		Order("priority DESC, updated_at DESC, id DESC").
		Find(&offers).Error; err != nil {
		return nil, err
	}
	if len(offers) == 0 {
		return []OfferResult{}, nil
	}

	// 2. Fetch LP overrides for this context (if landing page).
	overrideMap := map[uuid.UUID]models.OfferPageOverride{}
	if req.LandingPageID != nil {
		lpID, err := uuid.Parse(*req.LandingPageID)
		if err == nil {
			offerIDs := make([]uuid.UUID, len(offers))
			for i, o := range offers {
				offerIDs[i] = o.ID
			}
			var overrides []models.OfferPageOverride
			initializers.DB.Where("shop_id = ? AND landing_page_id = ? AND offer_id IN ?", shopID, lpID, offerIDs).Find(&overrides)
			for _, ov := range overrides {
				overrideMap[ov.OfferID] = ov
			}
		}
	}

	// 3. Collect all offered variant IDs for a single inventory batch query.
	// quantity_upsell offers have no OfferVariantIDs of their own — the
	// packages apply to whichever variant the customer already selected — so
	// pull those in too, for the same OOS check.
	selectedVariantIDs, _ := parseUUIDs(req.SelectedVariants)
	allVariantIDs := []uuid.UUID{}
	for _, o := range offers {
		allVariantIDs = append(allVariantIDs, o.OfferVariantIDs...)
		if o.OfferType != nil && *o.OfferType == "quantity_upsell" {
			allVariantIDs = append(allVariantIDs, selectedVariantIDs...)
		}
	}
	comboMap := map[uuid.UUID]models.ProductVariantCombination{}
	if len(allVariantIDs) > 0 {
		var combos []models.ProductVariantCombination
		initializers.DB.Select("id, quantity, price, combination_string, product_id").Where("id IN ?", allVariantIDs).Find(&combos)
		for _, c := range combos {
			comboMap[c.ID] = c
		}
	}

	// 4. Build exclusion set from existing order variants.
	existingSet := make(map[string]struct{}, len(req.ExistingVariants))
	for _, v := range req.ExistingVariants {
		existingSet[v] = struct{}{}
	}

	// 5. Evaluate each offer, apply overrides, check inventory, dedup by placement.
	type candidate struct {
		result   OfferResult
		priority int
	}
	byPlacement := map[string]candidate{}

	for _, offer := range offers {
		// Apply LP override.
		if ov, ok := overrideMap[offer.ID]; ok {
			if !ov.Enabled {
				continue
			}
			applyOverrideToOffer(&offer, ov)
		}

		// Evaluate conditions.
		if !matchesConditions(offer.Conditions, req) {
			continue
		}

		isQuantityUpsell := offer.OfferType != nil && *offer.OfferType == "quantity_upsell"

		var variants []OfferedVariant
		var anchorVariant *uuid.UUID
		var packages []models.OfferQuantityPackage

		if isQuantityUpsell {
			// Packages apply to whichever variant is already selected — check
			// that single combo's stock instead of a merchant-picked list.
			if len(selectedVariantIDs) > 0 {
				combo, found := comboMap[selectedVariantIDs[0]]
				available := found && combo.Quantity > 0
				if !available && offer.InventoryBehavior == "skip_when_oos" {
					continue
				}
			}
			packages = offer.QuantityPackages
		} else {
			// Build offered variants with inventory check.
			var hasAvailable bool
			variants, hasAvailable = buildOfferedVariants(offer, comboMap, existingSet)
			if !hasAvailable {
				continue
			}
			// Anchor variant for replace action.
			if offer.Action == "replace" {
				anchorVariant = findAnchorVariant(req.SelectedVariants, offer.TriggerProductID)
			}
		}

		offerType := ""
		if offer.OfferType != nil {
			offerType = *offer.OfferType
		}

		result := OfferResult{
			ID:            offer.ID,
			Action:        offer.Action,
			OfferType:     offerType,
			Placement:     offer.Placement,
			AnchorVariant: anchorVariant,
			Headline:      offer.Headline,
			Subheadline:   offer.Subheadline,
			ButtonText:    offer.ButtonText,
			MediaURL:      offer.MediaURL,
			AnalyticsTag:  offer.AnalyticsTag,
			QuantityRule:  offer.QuantityRule,
			Variants:      variants,
			Packages:      packages,
		}

		// Dedup: offers arrived pre-sorted by priority desc/updated_at
		// desc/id desc (step 1), so the first offer seen for a given
		// placement is deterministically the winner — no Go map iteration
		// order involved.
		if _, seen := byPlacement[offer.Placement]; !seen {
			byPlacement[offer.Placement] = candidate{result: result, priority: offer.Priority}
		}
	}

	// 6. Collect results — one per placement (there are only 4 valid
	// placement values, so no arbitrary cap), sorted by placement name for a
	// stable, deterministic response order.
	results := make([]OfferResult, 0, len(byPlacement))
	for _, c := range byPlacement {
		results = append(results, c.result)
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Placement < results[j].Placement
	})
	return results, nil
}

// matchesConditions evaluates all predicates in the offer's condition set.
// Empty predicates = always matches.
func matchesConditions(conds models.OfferConditions, req EvaluateOffersRequest) bool {
	for _, pred := range conds.Predicates {
		if !matchesPredicate(pred, req) {
			return false
		}
	}
	return true
}

// matchesPredicate evaluates a single predicate against the request context.
// Exported only through the package-internal test. Pure function, no DB.
func matchesPredicate(pred models.OfferPredicate, req EvaluateOffersRequest) bool {
	switch pred.Field {
	case "current_product":
		v, ok := pred.Value.(string)
		return ok && pred.Op == "eq" && v == req.ProductID

	case "current_landing_page":
		if req.LandingPageID == nil {
			return false
		}
		v, ok := pred.Value.(string)
		return ok && pred.Op == "eq" && v == *req.LandingPageID

	case "selected_variant":
		arr, ok := toStringSlice(pred.Value)
		if !ok {
			return false
		}
		set := make(map[string]struct{}, len(arr))
		for _, s := range arr {
			set[s] = struct{}{}
		}
		// "in": at least one selected variant is in the set.
		if pred.Op == "in" {
			for _, sv := range req.SelectedVariants {
				if _, found := set[sv]; found {
					return true
				}
			}
			return false
		}
		return false

	case "selected_quantity":
		threshold, ok := toFloat64(pred.Value)
		if !ok {
			return false
		}
		if pred.Op == "gte" {
			return float64(req.Quantity) >= threshold
		}
		return false

	case "customer_wilaya":
		if req.Wilaya == nil {
			return false
		}
		arr, ok := toFloat64Slice(pred.Value)
		if !ok {
			return false
		}
		if pred.Op == "in" {
			for _, w := range arr {
				if int(w) == *req.Wilaya {
					return true
				}
			}
			return false
		}
		return false

	case "utm_source":
		v, ok := pred.Value.(string)
		return ok && pred.Op == "eq" && v == req.UTMSource
	}

	return false
}

// buildOfferedVariants returns the variant cards for the offer response,
// skipping OOS variants (or all, per inventory_behavior) and excluded ones.
func buildOfferedVariants(
	offer models.Offer,
	comboMap map[uuid.UUID]models.ProductVariantCombination,
	existingSet map[string]struct{},
) ([]OfferedVariant, bool) {
	variants := make([]OfferedVariant, 0, len(offer.OfferVariantIDs))
	for _, vid := range offer.OfferVariantIDs {
		// Skip if the offered variant is already in the current order.
		if _, excluded := existingSet[vid.String()]; excluded {
			continue
		}
		combo, found := comboMap[vid]
		available := found && combo.Quantity > 0
		if !available && offer.InventoryBehavior == "skip_when_oos" {
			continue
		}
		variants = append(variants, OfferedVariant{
			ID:                vid,
			OfferedPrice:      services.ComputeOfferedPrice(combo.Price, offer.DiscountType, offer.DiscountValue),
			Available:         available,
			CombinationString: combo.CombinationString,
			ProductID:         combo.ProductID,
		})
	}
	return variants, len(variants) > 0
}

// findAnchorVariant returns the first selected variant that belongs to the trigger product.
// Used for "replace" action so the client knows which order_item to swap.
func findAnchorVariant(selectedVariants []string, triggerProductID uuid.UUID) *uuid.UUID {
	if len(selectedVariants) == 0 {
		return nil
	}
	// Fetch which of the selected variants belongs to the trigger product.
	varIDs, err := parseUUIDs(selectedVariants)
	if err != nil || len(varIDs) == 0 {
		return nil
	}
	var combo models.ProductVariantCombination
	if err := initializers.DB.Select("id").
		Where("id IN ? AND product_id = ?", varIDs, triggerProductID).
		First(&combo).Error; err != nil {
		return nil
	}
	return &combo.ID
}

// applyOverrideToOffer merges non-nil override fields onto the base offer (in-place).
func applyOverrideToOffer(offer *models.Offer, ov models.OfferPageOverride) {
	if ov.Headline != nil {
		offer.Headline = *ov.Headline
	}
	if ov.Subheadline != nil {
		offer.Subheadline = *ov.Subheadline
	}
	if ov.ButtonText != nil {
		offer.ButtonText = *ov.ButtonText
	}
	if ov.OfferProductID != nil {
		offer.OfferProductID = *ov.OfferProductID
	}
	if ov.DiscountType != nil {
		offer.DiscountType = *ov.DiscountType
	}
	if ov.DiscountValue != nil {
		offer.DiscountValue = *ov.DiscountValue
	}
	if ov.Placement != nil {
		offer.Placement = *ov.Placement
	}
}

// applyOfferBodyFields copies optional body fields onto an existing offer struct.
func applyOfferBodyFields(offer *models.Offer, body offerBody) {
	if body.Status != nil {
		offer.Status = *body.Status
	}
	if body.InternalName != nil {
		offer.InternalName = *body.InternalName
	}
	if body.Action != nil {
		offer.Action = *body.Action
	}
	if body.OfferType != nil {
		offer.OfferType = body.OfferType
	}
	if body.QuantityPackages != nil {
		offer.QuantityPackages = *body.QuantityPackages
	}
	if body.LandingPageID != nil {
		if *body.LandingPageID == "" {
			offer.LandingPageID = nil
		} else {
			id, err := uuid.Parse(*body.LandingPageID)
			if err == nil {
				offer.LandingPageID = &id
			}
		}
	}
	if body.QuantityRule != nil {
		offer.QuantityRule = *body.QuantityRule
	}
	if body.DiscountType != nil {
		offer.DiscountType = *body.DiscountType
	}
	if body.DiscountValue != nil {
		offer.DiscountValue = *body.DiscountValue
	}
	if body.Headline != nil {
		offer.Headline = *body.Headline
	}
	if body.Subheadline != nil {
		offer.Subheadline = *body.Subheadline
	}
	if body.ButtonText != nil {
		offer.ButtonText = *body.ButtonText
	}
	if body.MediaURL != nil {
		offer.MediaURL = *body.MediaURL
	}
	if body.Placement != nil {
		offer.Placement = *body.Placement
	}
	if body.Priority != nil {
		offer.Priority = *body.Priority
	}
	if body.Conditions != nil {
		offer.Conditions = *body.Conditions
	}
	if body.InventoryBehavior != nil {
		offer.InventoryBehavior = *body.InventoryBehavior
	}
	if body.AnalyticsTag != nil {
		offer.AnalyticsTag = *body.AnalyticsTag
	}
	if body.StartAt != nil {
		offer.StartAt = body.StartAt
	}
	if body.EndAt != nil {
		offer.EndAt = body.EndAt
	}
}

// ─── Public: Track Events ─────────────────────────────────────────────────────

type offerEventBody struct {
	OfferID   string     `json:"offerId"   binding:"required"`
	Event     string     `json:"event"     binding:"required"` // impression|click|accept|revenue
	OrderID   *string    `json:"orderId"`
	VariantID *string    `json:"variantId"`
	Wilaya    *int       `json:"wilaya"`
	Amount    float64    `json:"amount"`
}

func TrackOfferEventPublic(c *gin.Context) {
	slug := c.Param("slug")
	var shop models.Shop
	if err := initializers.DB.Select("id").Where("slug = ?", slug).First(&shop).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Shop not found"})
		return
	}

	var body offerEventBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid request body", "error": err.Error()})
		return
	}

	offerID, err := uuid.Parse(body.OfferID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid offerId"})
		return
	}

	event := models.OfferEvent{
		ShopID:  shop.ID,
		OfferID: offerID,
		Event:   body.Event,
		Amount:  body.Amount,
		Wilaya:  body.Wilaya,
	}
	if body.OrderID != nil {
		id, err := uuid.Parse(*body.OrderID)
		if err == nil {
			event.OrderID = &id
		}
	}
	if body.VariantID != nil {
		id, err := uuid.Parse(*body.VariantID)
		if err == nil {
			event.VariantID = &id
		}
	}

	// Fire-and-forget — same pattern as post-order async tasks.
	go func() {
		defer func() { recover() }()
		initializers.DB.Create(&event)
	}()

	c.Status(http.StatusNoContent)
}

// ─── JSON type helpers ────────────────────────────────────────────────────────

func toFloat64(v interface{}) (float64, bool) {
	f, ok := v.(float64)
	return f, ok
}

func toStringSlice(v interface{}) ([]string, bool) {
	arr, ok := v.([]interface{})
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		s, ok := item.(string)
		if !ok {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}

func toFloat64Slice(v interface{}) ([]float64, bool) {
	arr, ok := v.([]interface{})
	if !ok {
		return nil, false
	}
	out := make([]float64, 0, len(arr))
	for _, item := range arr {
		f, ok := item.(float64)
		if !ok {
			return nil, false
		}
		out = append(out, f)
	}
	return out, true
}
