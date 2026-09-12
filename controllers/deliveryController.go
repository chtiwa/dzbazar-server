package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func publicDeliveryRatesCacheKey(shopID uuid.UUID) string {
	return fmt.Sprintf("delivery-rates:public:shop=%s", shopID.String())
}

func invalidateDeliveryRatesCache(shopID uuid.UUID) {
	if err := initializers.RClient.Del(initializers.Ctx, publicDeliveryRatesCacheKey(shopID)).Err(); err != nil {
		fmt.Println("Failed to delete delivery rates cache key:", err)
	}
}

type UpdateDeliveryRateInput struct {
	WilayaID     int     `json:"wilayaId" binding:"required"`
	IsActive     bool    `json:"isActive"`
	HasDoorstep  bool    `json:"hasDoorstep"`
	DoorstepRate float64 `json:"doorstepRate"`
	HasStopdesk  bool    `json:"hasStopdesk"`
	StopdeskRate float64 `json:"stopdeskRate"`
}

type BulkUpdateDeliveryRatesInput struct {
	Rates []UpdateDeliveryRateInput `json:"rates" binding:"required,dive"`
}

// effectiveIsActive forces a rate inactive when both prices are 0 -- a free
// shipping method the shop never intended, since the form has no "set price
// to 0 on purpose" affordance. A shop that wants free shipping should use a
// coupon/offer instead.
func effectiveIsActive(isActive bool, doorstepRate, stopdeskRate float64) bool {
	if doorstepRate == 0 && stopdeskRate == 0 {
		return false
	}
	return isActive
}

func GetDeliveryRates(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	var rates []models.DeliveryRate
	if err := initializers.DB.Where("shop_id = ?", shopID).Find(&rates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to fetch rates"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    rates,
	})
}

// GetPublicDeliveryRates is the unauthenticated counterpart of GetDeliveryRates,
// used by the public storefront checkout to load shipping options for a shop.
// freeDeliveryEnabled rides alongside the untouched per-wilaya rates (rather
// than zeroing them) so the admin's configured prices stay intact if the
// merchant later flips the toggle back off.
func GetPublicDeliveryRates(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	var shop models.Shop
	if err := initializers.DB.Select("free_delivery_enabled").First(&shop, "id = ?", shopID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to fetch shop"})
		return
	}

	cacheKey := publicDeliveryRatesCacheKey(shopID)
	if val, err := initializers.RClient.Get(initializers.Ctx, cacheKey).Result(); err == nil {
		var cachedRates []models.DeliveryRate
		if unmarshalErr := json.Unmarshal([]byte(val), &cachedRates); unmarshalErr == nil {
			c.JSON(http.StatusOK, gin.H{"success": true, "data": cachedRates, "freeDeliveryEnabled": shop.FreeDeliveryEnabled})
			return
		}
	}

	var rates []models.DeliveryRate
	if err := initializers.DB.Where("shop_id = ? AND is_active = ?", shopID, true).Find(&rates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to fetch rates"})
		return
	}

	if jsonData, err := json.Marshal(rates); err == nil {
		_ = initializers.RClient.Set(initializers.Ctx, cacheKey, jsonData, 10*time.Minute).Err()
	}

	c.JSON(http.StatusOK, gin.H{
		"success":             true,
		"data":                rates,
		"freeDeliveryEnabled": shop.FreeDeliveryEnabled,
	})
}

func UpdateDeliveryRate(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid shop ID",
		})
		return
	}

	var input UpdateDeliveryRateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid payload",
			"error":   err.Error(),
		})
		return
	}

	updates := map[string]interface{}{
		"is_active":     effectiveIsActive(input.IsActive, input.DoorstepRate, input.StopdeskRate),
		"has_doorstep":  input.HasDoorstep,
		"doorstep_rate": input.DoorstepRate,
		"has_stopdesk":  input.HasStopdesk,
		"stopdesk_rate": input.StopdeskRate,
	}

	result := initializers.DB.
		Model(&models.DeliveryRate{}).
		Where("shop_id = ? AND wilaya_id = ?", shopID, input.WilayaID).
		Updates(updates)

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to update delivery rate",
			"error":   result.Error.Error(),
		})
		return
	}

	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Delivery rate not found for this shop and wilaya",
		})
		return
	}

	var rate models.DeliveryRate
	if err := initializers.DB.
		Where("shop_id = ? AND wilaya_id = ?", shopID, input.WilayaID).
		First(&rate).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Delivery rate updated but failed to reload record",
			"error":   err.Error(),
		})
		return
	}

	invalidateDeliveryRatesCache(shopID)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Delivery rate updated",
		"data":    rate,
	})
}

func BulkUpdateDeliveryRates(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid shop ID",
		})
		return
	}

	var input BulkUpdateDeliveryRatesInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid payload",
			"error":   err.Error(),
		})
		return
	}

	err = initializers.DB.Transaction(func(tx *gorm.DB) error {
		for _, item := range input.Rates {
			result := tx.Model(&models.DeliveryRate{}).
				Where("shop_id = ? AND wilaya_id = ?", shopID, item.WilayaID).
				Updates(map[string]interface{}{
					"is_active":     effectiveIsActive(item.IsActive, item.DoorstepRate, item.StopdeskRate),
					"has_doorstep":  item.HasDoorstep,
					"doorstep_rate": item.DoorstepRate,
					"has_stopdesk":  item.HasStopdesk,
					"stopdesk_rate": item.StopdeskRate,
				})

			if result.Error != nil {
				return result.Error
			}

			if result.RowsAffected == 0 {
				return fmt.Errorf("delivery rate not found for wilaya_id=%d", item.WilayaID)
			}
		}

		return nil
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to update delivery rates",
			"error":   err.Error(),
		})
		return
	}

	var rates []models.DeliveryRate
	if err := initializers.DB.
		Where("shop_id = ?", shopID).
		Order("wilaya_id ASC").
		Find(&rates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Delivery rates updated but failed to reload records",
			"error":   err.Error(),
		})
		return
	}

	invalidateDeliveryRatesCache(shopID)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Delivery rates updated successfully",
		"data":    rates,
	})
}
