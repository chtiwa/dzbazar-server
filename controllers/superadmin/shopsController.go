package superadmin

import (
	"net/http"
	"strings"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func ListShops(c *gin.Context) {
	search := strings.TrimSpace(c.Query("search"))
	statusFilter := c.Query("status") // "active" | "suspended" | ""
	page, perPage := parsePageParams(c)

	db := initializers.DB.Model(&models.Shop{}).Preload("Owner").Preload("LogoImage")

	if search != "" {
		like := "%" + strings.ToLower(search) + "%"
		db = db.Where("LOWER(name) LIKE ? OR LOWER(slug) LIKE ?", like, like)
	}
	if statusFilter == "active" {
		db = db.Where("is_active = true")
	} else if statusFilter == "suspended" {
		db = db.Where("is_active = false")
	}

	var totalRows int64
	db.Count(&totalRows)

	var shops []models.Shop
	if err := db.Order("created_at DESC").
		Offset((page - 1) * perPage).Limit(perPage).
		Find(&shops).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to fetch shops", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"data":       shops,
		"pagination": paginationMeta(page, perPage, totalRows),
	})
}

func GetShop(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	var shop models.Shop
	if err := initializers.DB.
		Preload("Owner").
		Preload("LogoImage").
		Preload("Members").
		Preload("Members.User").
		First(&shop, "id = ?", shopID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Shop not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Database error", "error": err.Error()})
		return
	}

	var subscription models.ShopSubscription
	subErr := initializers.DB.Preload("Plan").Where("shop_id = ?", shopID).First(&subscription).Error

	var productCount, orderCount int64
	initializers.DB.Model(&models.Product{}).Where("shop_id = ?", shopID).Count(&productCount)
	initializers.DB.Model(&models.Order{}).Where("shop_id = ?", shopID).Count(&orderCount)

	resp := gin.H{
		"shop":         shop,
		"productCount": productCount,
		"orderCount":   orderCount,
	}
	if subErr == nil {
		resp["subscription"] = subscription
	} else {
		resp["subscription"] = nil
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": resp})
}

type UpdateShopStatusInput struct {
	IsActive bool `json:"isActive"`
}

// UpdateShopStatus suspends or reactivates a shop's storefront platform-wide.
func UpdateShopStatus(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	var body UpdateShopStatusInput
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Validation failed", "error": err.Error()})
		return
	}

	var shop models.Shop
	if err := initializers.DB.First(&shop, "id = ?", shopID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Shop not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Database error", "error": err.Error()})
		return
	}

	if err := initializers.DB.Model(&shop).Update("is_active", body.IsActive).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to update shop status", "error": err.Error()})
		return
	}

	action := "shop.suspend"
	if body.IsActive {
		action = "shop.activate"
	}
	utils.LogAudit(c, action, "Shop", &shop.ID, gin.H{"name": shop.Name, "slug": shop.Slug})

	shop.IsActive = body.IsActive
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Shop status updated", "data": shop})
}

// DeleteShopByAdmin permanently deletes a shop and everything that cascades
// from it (members, products, orders, clients, pixels). Irreversible.
func DeleteShopByAdmin(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	var shop models.Shop
	if err := initializers.DB.First(&shop, "id = ?", shopID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Shop not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Database error", "error": err.Error()})
		return
	}

	snapshot := gin.H{"name": shop.Name, "slug": shop.Slug, "ownerId": shop.OwnerID}

	if err := initializers.DB.Delete(&shop).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to delete shop", "error": err.Error()})
		return
	}

	utils.LogAudit(c, "shop.delete", "Shop", &shop.ID, snapshot)

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Shop deleted permanently"})
}
