package controllers

import (
	"fmt"
	"net/http"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

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
		"is_active":     input.IsActive,
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
					"is_active":     item.IsActive,
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

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Delivery rates updated successfully",
		"data":    rates,
	})
}
