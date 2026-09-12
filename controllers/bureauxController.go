package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// normalizeBureauName trims a typed desk name and collapses internal
// whitespace runs, so "OUM EL   BOUGHI" and "OUM EL BOUGHI" collide on the
// unique index instead of both landing in the dropdown.
func normalizeBureauName(raw string) string {
	return strings.Join(strings.Fields(raw), " ")
}

// publicBureauxCacheKey is scoped by shop+wilaya, not just shop, because
// ListPublicBureaux filters server-side (unlike delivery-rates' whole-shop
// cache) — caching the filtered result means the key must carry the filter.
func publicBureauxCacheKey(shopID uuid.UUID, wilayaID int) string {
	return fmt.Sprintf("bureaux:public:shop=%s:wilaya=%d", shopID.String(), wilayaID)
}

func invalidatePublicBureauxCache(shopID uuid.UUID, wilayaID int) {
	if err := initializers.RClient.Del(initializers.Ctx, publicBureauxCacheKey(shopID, wilayaID)).Err(); err != nil {
		fmt.Println("Failed to delete bureaux cache key:", err)
	}
}

type CreateBureauInput struct {
	WilayaID int    `json:"wilayaId" binding:"required"`
	Name     string `json:"name" binding:"required"`
}

type UpdateBureauInput struct {
	Name string `json:"name" binding:"required"`
}

// ListBureaux returns the shop's bureaux, optionally narrowed to one wilaya.
// The order form calls it with wilayaId (the desks for the customer's
// wilaya); the management page calls it without, to render every wilaya's
// desks grouped.
func ListBureaux(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	query := initializers.DB.Where("shop_id = ?", shopID)

	if raw := c.Query("wilayaId"); raw != "" {
		wilayaID, parseErr := strconv.Atoi(raw)
		if parseErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid wilaya ID"})
			return
		}
		query = query.Where("wilaya_id = ?", wilayaID)
	}

	bureaux := []models.Bureau{}
	if err := query.Order("wilaya_id ASC, name ASC").Find(&bureaux).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to fetch bureaux",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": bureaux})
}

// ListPublicBureaux is the unauthenticated counterpart for the anonymous
// storefront checkout. Unlike ListBureaux it requires wilayaId — the
// storefront always knows the customer's wilaya by the time it asks, and a
// required filter keeps the cache key space bounded to real wilayas.
func ListPublicBureaux(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	wilayaID, err := strconv.Atoi(c.Query("wilayaId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "wilayaId is required"})
		return
	}

	cacheKey := publicBureauxCacheKey(shopID, wilayaID)
	if val, err := initializers.RClient.Get(initializers.Ctx, cacheKey).Result(); err == nil {
		var cached []models.Bureau
		if unmarshalErr := json.Unmarshal([]byte(val), &cached); unmarshalErr == nil {
			c.JSON(http.StatusOK, gin.H{"success": true, "data": cached})
			return
		}
	}

	bureaux := []models.Bureau{}
	if err := initializers.DB.
		Where("shop_id = ? AND wilaya_id = ?", shopID, wilayaID).
		Order("name ASC").
		Find(&bureaux).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to fetch bureaux",
			"error":   err.Error(),
		})
		return
	}

	if jsonData, err := json.Marshal(bureaux); err == nil {
		_ = initializers.RClient.Set(initializers.Ctx, cacheKey, jsonData, 10*time.Minute).Err()
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": bureaux})
}

// CreateBureau adds one extra desk name to a shop's wilaya. Shop ownership is
// implicit: the row is written with the :shopId from the route, which
// RequireShopAccess + RequireShopPermission("bureaux.edit") have already
// verified the caller belongs to, so a caller can only ever create rows under
// their own shop.
func CreateBureau(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	var input CreateBureauInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid payload",
			"error":   err.Error(),
		})
		return
	}

	name := normalizeBureauName(input.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Bureau name is required"})
		return
	}

	var wilaya models.Wilaya
	if err := initializers.DB.Where("id = ?", input.WilayaID).First(&wilaya).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Unknown wilaya"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Database error",
			"error":   err.Error(),
		})
		return
	}

	bureau := models.Bureau{
		ShopID:   shopID,
		WilayaID: input.WilayaID,
		Name:     name,
	}

	if err := initializers.DB.Create(&bureau).Error; err != nil {
		// idx_bureaux_unique_name (migration 00036) rejects the exact same
		// name twice for one shop + wilaya.
		if strings.Contains(strings.ToLower(err.Error()), "duplicate key") {
			c.JSON(http.StatusConflict, gin.H{
				"success": false,
				"message": "This bureau already exists for that wilaya",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to create bureau",
			"error":   err.Error(),
		})
		return
	}

	invalidatePublicBureauxCache(shopID, input.WilayaID)

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "Bureau created",
		"data":    bureau,
	})
}

// UpdateBureau renames an existing desk. The wilaya is immutable — moving a
// desk to a different wilaya is a delete + create, not an edit.
func UpdateBureau(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	bureauID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid bureau ID"})
		return
	}

	var input UpdateBureauInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid payload",
			"error":   err.Error(),
		})
		return
	}

	name := normalizeBureauName(input.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Bureau name is required"})
		return
	}

	var bureau models.Bureau
	if err := initializers.DB.
		Where("id = ? AND shop_id = ?", bureauID, shopID).
		First(&bureau).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Bureau not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Database error",
			"error":   err.Error(),
		})
		return
	}

	bureau.Name = name
	if err := initializers.DB.Save(&bureau).Error; err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate key") {
			c.JSON(http.StatusConflict, gin.H{
				"success": false,
				"message": "This bureau already exists for that wilaya",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to update bureau",
			"error":   err.Error(),
		})
		return
	}

	invalidatePublicBureauxCache(shopID, bureau.WilayaID)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Bureau updated",
		"data":    bureau,
	})
}

func DeleteBureau(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	bureauID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid bureau ID"})
		return
	}

	// Scoped by shop_id as well as id, so one shop can never delete a row
	// belonging to a different shop.
	var bureau models.Bureau
	if err := initializers.DB.
		Where("id = ? AND shop_id = ?", bureauID, shopID).
		First(&bureau).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Bureau not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Database error",
			"error":   err.Error(),
		})
		return
	}

	if err := initializers.DB.Delete(&bureau).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to delete bureau",
			"error":   err.Error(),
		})
		return
	}

	invalidatePublicBureauxCache(shopID, bureau.WilayaID)

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Bureau deleted"})
}
