package controllers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/services"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type CreatePixelInput struct {
	Platform    string `json:"platform" binding:"required"`
	Title       string `json:"title" binding:"required"`
	PixelID     string `json:"pixelId" binding:"required"`
	AccessToken string `json:"accessToken"`
}

type UpdatePixelInput struct {
	Title       *string `json:"title"`
	PixelID     *string `json:"pixelId"`
	IsActive    *bool   `json:"isActive"`
	AccessToken *string `json:"accessToken"`
	ClearToken  *bool   `json:"clearToken"`
}

func GetPixelsByShop(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid shop ID",
		})
		return
	}

	platform := strings.ToLower(strings.TrimSpace(c.Query("platform")))

	var pixels []models.Pixel
	query := initializers.DB.Where("shop_id = ?", shopID)

	if platform != "" {
		query = query.Where("platform = ?", platform)
	}

	if err := query.Order("created_at DESC").Find(&pixels).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to retrieve pixels",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"count":   len(pixels),
		"data":    pixels,
	})
}

func IndexPixel(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid shop ID",
		})
		return
	}

	pixelID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid pixel ID",
		})
		return
	}

	var pixel models.Pixel
	err = initializers.DB.
		Where("id = ? AND shop_id = ?", pixelID, shopID).
		First(&pixel).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "Pixel not found",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to retrieve pixel",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    pixel,
	})
}

func IndexActivePixelByShop(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid shop ID",
		})
		return
	}

	platform := strings.ToLower(strings.TrimSpace(c.Query("platform")))
	if platform == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Platform is required",
		})
		return
	}

	if platform != "facebook" && platform != "tiktok" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Unsupported platform",
		})
		return
	}

	var pixel models.Pixel
	err = initializers.DB.
		Where("shop_id = ? AND platform = ? AND is_active = ?", shopID, platform, true).
		Order("created_at DESC").
		First(&pixel).Error

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Active pixel not found",
		})
		return
	}

	// Public endpoint — never expose the access token, only whether one is set.
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"id":             pixel.ID,
			"shopId":         pixel.ShopID,
			"platform":       pixel.Platform,
			"title":          pixel.Title,
			"pixelId":        pixel.PixelID,
			"isActive":       pixel.IsActive,
			"hasAccessToken": pixel.HasAccessToken,
		},
	})
}

func CreatePixel(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid shop ID",
		})
		return
	}

	var body CreatePixelInput
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request body",
			"error":   err.Error(),
		})
		return
	}

	platform := strings.ToLower(strings.TrimSpace(body.Platform))
	title := strings.TrimSpace(body.Title)
	pixelID := strings.TrimSpace(body.PixelID)
	accessToken := strings.TrimSpace(body.AccessToken)

	if err := services.CheckPixelLimit(shopID, platform); err != nil {
		if errors.Is(err, services.ErrPlanLimitReached) {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "Pixel limit reached for your plan. Upgrade to add more pixels.",
				"code":    "PLAN_LIMIT_REACHED",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to verify plan limits", "error": err.Error()})
		return
	}

	pixel := models.Pixel{
		ShopID:         shopID,
		Platform:       platform,
		Title:          title,
		PixelID:        pixelID,
		HasAccessToken: accessToken != "",
		AccessToken:    accessToken,
	}

	if err := initializers.DB.Create(&pixel).Error; err != nil {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"message": "Failed to create pixel. It may already exist for this shop.",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "Pixel created successfully",
		"data":    pixel,
	})
}

func UpdatePixel(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid shop ID",
		})
		return
	}

	pixelID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid pixel ID",
		})
		return
	}

	var input UpdatePixelInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request body",
			"error":   err.Error(),
		})
		return
	}

	var pixel models.Pixel
	err = initializers.DB.
		Where("id = ? AND shop_id = ?", pixelID, shopID).
		First(&pixel).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "Pixel not found",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to load pixel",
			"error":   err.Error(),
		})
		return
	}

	tx := initializers.DB.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to start transaction",
			"error":   tx.Error.Error(),
		})
		return
	}

	updateData := map[string]interface{}{}

	if input.Title != nil {
		updateData["title"] = strings.TrimSpace(*input.Title)
	}

	if input.PixelID != nil {
		updateData["pixel_id"] = strings.TrimSpace(*input.PixelID)
	}

	if input.AccessToken != nil {
		cleanToken := strings.TrimSpace(*input.AccessToken)
		updateData["access_token"] = cleanToken
		updateData["has_access_token"] = cleanToken != ""
	}

	if input.ClearToken != nil && *input.ClearToken {
		updateData["access_token"] = ""
		updateData["has_access_token"] = false
	}

	if input.IsActive != nil {
		if *input.IsActive {
			// Deactivate other active pixels in same shop + platform
			if err := tx.Model(&models.Pixel{}).
				Where("shop_id = ? AND platform = ? AND id <> ?", pixel.ShopID, pixel.Platform, pixel.ID).
				Update("is_active", false).Error; err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{
					"success": false,
					"message": "Failed to deactivate other pixels",
					"error":   err.Error(),
				})
				return
			}

			updateData["is_active"] = true
		} else {
			updateData["is_active"] = false
		}
	}

	if len(updateData) == 0 {
		tx.Rollback()
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "No changes provided",
			"data":    pixel,
		})
		return
	}

	if err := tx.Model(&pixel).Updates(updateData).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to update pixel",
			"error":   err.Error(),
		})
		return
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to commit pixel update",
			"error":   err.Error(),
		})
		return
	}

	if err := initializers.DB.First(&pixel, "id = ?", pixel.ID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Pixel updated but failed to reload record",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Pixel updated successfully",
		"data":    pixel,
	})
}

func DeletePixel(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid shop ID",
		})
		return
	}

	pixelID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid pixel ID",
		})
		return
	}

	var pixel models.Pixel
	err = initializers.DB.
		Where("id = ? AND shop_id = ?", pixelID, shopID).
		First(&pixel).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "Pixel not found",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to load pixel",
			"error":   err.Error(),
		})
		return
	}

	if err := initializers.DB.Delete(&pixel).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to delete pixel",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Pixel deleted successfully",
	})
}
