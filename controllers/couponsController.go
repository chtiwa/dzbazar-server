package controllers

import (
	"math"
	"net/http"
	"strings"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// couponScopeMatches reports whether coupon applies to an order containing the given
// product IDs. Empty scope (no linked products/landing pages) means shop-wide.
func couponScopeMatches(coupon models.Coupon, productIDs []uuid.UUID) bool {
	if len(coupon.Products) == 0 && len(coupon.LandingPages) == 0 {
		return true
	}

	productSet := make(map[uuid.UUID]struct{}, len(productIDs))
	for _, id := range productIDs {
		productSet[id] = struct{}{}
	}

	for _, p := range coupon.Products {
		if _, ok := productSet[p.ID]; ok {
			return true
		}
	}
	for _, lp := range coupon.LandingPages {
		if _, ok := productSet[lp.ProductID]; ok {
			return true
		}
	}
	return false
}

// couponDiscount is the single place the matching + rounding rule lives — used by both
// the public validate preview and authoritative order creation, so they can't drift.
func couponDiscount(coupon models.Coupon, orderProductIDs []uuid.UUID, subtotal float64) (discount float64, matched bool) {
	if !coupon.Active {
		return 0, false
	}
	if !couponScopeMatches(coupon, orderProductIDs) {
		return 0, false
	}
	return math.Round(subtotal * float64(coupon.Percent) / 100), true
}

func findCouponByCode(shopID uuid.UUID, code string) (*models.Coupon, error) {
	var coupon models.Coupon
	err := initializers.DB.
		Preload("Products").
		Preload("LandingPages").
		Where("shop_id = ? AND UPPER(code) = ?", shopID, strings.ToUpper(strings.TrimSpace(code))).
		First(&coupon).Error
	if err != nil {
		return nil, err
	}
	return &coupon, nil
}

type couponBody struct {
	Code           *string  `json:"code"`
	Percent        *int     `json:"percent"`
	Active         *bool    `json:"active"`
	ProductIDs     []string `json:"productIds"`
	LandingPageIDs []string `json:"landingPageIds"`
}

func parseUUIDs(raw []string) ([]uuid.UUID, error) {
	ids := make([]uuid.UUID, 0, len(raw))
	for _, r := range raw {
		id, err := uuid.Parse(r)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func CreateCoupon(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	var body couponBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid request body", "error": err.Error()})
		return
	}

	if body.Code == nil || strings.TrimSpace(*body.Code) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Code is required"})
		return
	}
	if body.Percent == nil || *body.Percent < 1 || *body.Percent > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Percent must be between 1 and 100"})
		return
	}

	code := strings.ToUpper(strings.TrimSpace(*body.Code))

	var existing int64
	initializers.DB.Model(&models.Coupon{}).Where("shop_id = ? AND UPPER(code) = ?", shopID, code).Count(&existing)
	if existing > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "A coupon with this code already exists"})
		return
	}

	productIDs, err := parseUUIDs(body.ProductIDs)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid product ID in scope"})
		return
	}
	landingPageIDs, err := parseUUIDs(body.LandingPageIDs)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid landing page ID in scope"})
		return
	}
	if productIDs, err = productsOwnedByShop(shopID, productIDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to validate product scope", "error": err.Error()})
		return
	}
	if landingPageIDs, err = landingPagesOwnedByShop(shopID, landingPageIDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to validate landing page scope", "error": err.Error()})
		return
	}

	active := true
	if body.Active != nil {
		active = *body.Active
	}

	coupon := models.Coupon{
		ShopID:  shopID,
		Code:    code,
		Percent: *body.Percent,
		Active:  active,
	}

	err = initializers.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&coupon).Error; err != nil {
			return err
		}
		if err := replaceCouponProducts(tx, coupon.ID, productIDs); err != nil {
			return err
		}
		return replaceCouponLandingPages(tx, coupon.ID, landingPageIDs)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to create coupon", "error": err.Error()})
		return
	}

	initializers.DB.Preload("Products").Preload("LandingPages").First(&coupon, "id = ?", coupon.ID)

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Coupon created successfully", "data": coupon})
}

// productsOwnedByShop filters productIDs down to ones that actually belong to shopID,
// so a coupon can never be scoped to another shop's products via a guessed UUID.
func productsOwnedByShop(shopID uuid.UUID, ids []uuid.UUID) ([]uuid.UUID, error) {
	if len(ids) == 0 {
		return ids, nil
	}
	var owned []uuid.UUID
	if err := initializers.DB.Model(&models.Product{}).
		Where("id IN ? AND shop_id = ?", ids, shopID).
		Pluck("id", &owned).Error; err != nil {
		return nil, err
	}
	return owned, nil
}

func landingPagesOwnedByShop(shopID uuid.UUID, ids []uuid.UUID) ([]uuid.UUID, error) {
	if len(ids) == 0 {
		return ids, nil
	}
	var owned []uuid.UUID
	if err := initializers.DB.Model(&models.LandingPage{}).
		Where("id IN ? AND shop_id = ?", ids, shopID).
		Pluck("id", &owned).Error; err != nil {
		return nil, err
	}
	return owned, nil
}

// replaceCouponProducts and replaceCouponLandingPages write the join tables directly
// instead of going through GORM's Association().Append/Replace: that path calls Create()
// on stub Product/LandingPage structs, and this codebase's BaseModel.BeforeCreate hook
// unconditionally overwrites any caller-supplied ID with a fresh one — which turns
// "attach this existing product" into "insert a phantom product with a zero ShopID",
// violating the products→shops FK. Raw join-table writes sidestep that hook entirely.
func replaceCouponProducts(tx *gorm.DB, couponID uuid.UUID, productIDs []uuid.UUID) error {
	if err := tx.Exec("DELETE FROM coupon_products WHERE coupon_id = ?", couponID).Error; err != nil {
		return err
	}
	for _, id := range productIDs {
		if err := tx.Exec(
			"INSERT INTO coupon_products (coupon_id, product_id) VALUES (?, ?) ON CONFLICT DO NOTHING",
			couponID, id,
		).Error; err != nil {
			return err
		}
	}
	return nil
}

func replaceCouponLandingPages(tx *gorm.DB, couponID uuid.UUID, landingPageIDs []uuid.UUID) error {
	if err := tx.Exec("DELETE FROM coupon_landing_pages WHERE coupon_id = ?", couponID).Error; err != nil {
		return err
	}
	for _, id := range landingPageIDs {
		if err := tx.Exec(
			"INSERT INTO coupon_landing_pages (coupon_id, landing_page_id) VALUES (?, ?) ON CONFLICT DO NOTHING",
			couponID, id,
		).Error; err != nil {
			return err
		}
	}
	return nil
}

func GetCouponsByShop(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	var coupons []models.Coupon
	if err := initializers.DB.
		Where("shop_id = ?", shopID).
		Preload("Products").
		Preload("LandingPages").
		Order("created_at DESC").
		Find(&coupons).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to retrieve coupons", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": coupons})
}

func UpdateCoupon(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}
	couponID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid coupon ID"})
		return
	}

	var coupon models.Coupon
	if err := initializers.DB.Where("id = ? AND shop_id = ?", couponID, shopID).First(&coupon).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Coupon not found"})
		return
	}

	var body couponBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid request body", "error": err.Error()})
		return
	}

	updates := map[string]interface{}{}

	if body.Code != nil {
		code := strings.ToUpper(strings.TrimSpace(*body.Code))
		if code == "" {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Code cannot be empty"})
			return
		}
		var existing int64
		initializers.DB.Model(&models.Coupon{}).
			Where("shop_id = ? AND UPPER(code) = ? AND id <> ?", shopID, code, couponID).
			Count(&existing)
		if existing > 0 {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "A coupon with this code already exists"})
			return
		}
		updates["code"] = code
	}

	if body.Percent != nil {
		if *body.Percent < 1 || *body.Percent > 100 {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Percent must be between 1 and 100"})
			return
		}
		updates["percent"] = *body.Percent
	}

	if body.Active != nil {
		updates["active"] = *body.Active
	}

	if len(updates) > 0 {
		if err := initializers.DB.Model(&coupon).Updates(updates).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to update coupon", "error": err.Error()})
			return
		}
	}

	if body.ProductIDs != nil {
		productIDs, err := parseUUIDs(body.ProductIDs)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid product ID in scope"})
			return
		}
		if productIDs, err = productsOwnedByShop(shopID, productIDs); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to validate product scope", "error": err.Error()})
			return
		}
		if err := replaceCouponProducts(initializers.DB, coupon.ID, productIDs); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to update product scope", "error": err.Error()})
			return
		}
	}

	if body.LandingPageIDs != nil {
		landingPageIDs, err := parseUUIDs(body.LandingPageIDs)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid landing page ID in scope"})
			return
		}
		if landingPageIDs, err = landingPagesOwnedByShop(shopID, landingPageIDs); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to validate landing page scope", "error": err.Error()})
			return
		}
		if err := replaceCouponLandingPages(initializers.DB, coupon.ID, landingPageIDs); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to update landing page scope", "error": err.Error()})
			return
		}
	}

	initializers.DB.Preload("Products").Preload("LandingPages").First(&coupon, "id = ?", coupon.ID)

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Coupon updated successfully", "data": coupon})
}

func DeleteCoupon(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}
	couponID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid coupon ID"})
		return
	}

	result := initializers.DB.Where("id = ? AND shop_id = ?", couponID, shopID).Delete(&models.Coupon{})
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to delete coupon", "error": result.Error.Error()})
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Coupon not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Coupon deleted successfully"})
}

// CouponAvailableForProduct lets the storefront decide whether to show the coupon field
// at all — true only if at least one active coupon currently applies to this product.
// Doesn't reveal codes, just a boolean.
func CouponAvailableForProduct(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	productID, err := uuid.Parse(c.Query("productId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid product ID"})
		return
	}

	var coupons []models.Coupon
	if err := initializers.DB.
		Where("shop_id = ? AND active = ?", shopID, true).
		Preload("Products").
		Preload("LandingPages").
		Find(&coupons).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to check coupons", "error": err.Error()})
		return
	}

	available := false
	for _, coupon := range coupons {
		if couponScopeMatches(coupon, []uuid.UUID{productID}) {
			available = true
			break
		}
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"available": available}})
}

type validateCouponBody struct {
	Code      string  `json:"code" binding:"required"`
	ProductID string  `json:"productId" binding:"required"`
	Subtotal  float64 `json:"subtotal"`
}

// ValidateCouponPublic lets the storefront preview a discount before checkout. It always
// returns 200 — "invalid code" is a normal outcome, not a server error.
func ValidateCouponPublic(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	var body validateCouponBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid request body", "error": err.Error()})
		return
	}

	productID, err := uuid.Parse(body.ProductID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid product ID"})
		return
	}

	coupon, err := findCouponByCode(shopID, body.Code)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"valid": false, "message": "Invalid coupon code"}})
		return
	}

	discount, matched := couponDiscount(*coupon, []uuid.UUID{productID}, body.Subtotal)
	if !matched {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"valid": false, "message": "This coupon does not apply here"}})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"valid":    true,
			"percent":  coupon.Percent,
			"discount": discount,
			"message":  "Coupon applied",
		},
	})
}
