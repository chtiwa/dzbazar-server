package controllers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// isBureauEligibleCarrier reports whether a carrier, identified by its
// AvailableDeliveryCompany.Name, is allowed to own bureau rows.
//
// ZR Express is the one exclusion: it resolves stopdesk hubs live via its own
// API (resolveZrHubID in zrGeoController.go), so a typed bureau list for ZR
// would be dead data that silently diverges from the real hub network. The
// case-insensitive substring match mirrors the carrier filter already used in
// admin/src/pages/orders/BatchShipModal.tsx, since the catalog stores display
// names ("ZR Express") rather than stable slugs.
func isBureauEligibleCarrier(availableCompanyName string) bool {
	name := strings.ToLower(strings.TrimSpace(availableCompanyName))
	if name == "" {
		return false
	}
	return !strings.Contains(name, "zr")
}

// normalizeBureauName trims a typed desk name and collapses internal
// whitespace runs, so "OUM EL   BOUGHI" and "OUM EL BOUGHI" collide on the
// unique index instead of both landing in the dropdown.
func normalizeBureauName(raw string) string {
	return strings.Join(strings.Fields(raw), " ")
}

type CreateBureauInput struct {
	DeliveryCompanyID string `json:"deliveryCompanyId" binding:"required"`
	WilayaID          int    `json:"wilayaId" binding:"required"`
	Name              string `json:"name" binding:"required"`
}

// findEligibleShopCarrier loads one of the shop's connected carrier rows by id
// and verifies both that it belongs to this shop (tenant isolation) and that
// it is allowed to own bureaux (not ZR). Mirrors the ownership-check shape of
// findZrIntegration in zrController.go. The refusal string is empty on success.
func findEligibleShopCarrier(shopID, deliveryCompanyID uuid.UUID) (*models.DeliveryCompany, string) {
	var company models.DeliveryCompany
	err := initializers.DB.
		Preload("AvailableDeliveryCompany").
		Where("id = ? AND shop_id = ?", deliveryCompanyID, shopID).
		First(&company).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, "Delivery company not found for this shop"
		}
		return nil, "Database error"
	}

	if !isBureauEligibleCarrier(company.AvailableDeliveryCompany.Name) {
		return nil, "ZR Express resolves its stopdesk hubs automatically and does not use bureaux"
	}

	return &company, ""
}

// ListBureaux returns the shop's bureaux, optionally narrowed to one carrier
// and/or one wilaya. The order form calls it with wilayaId only (union across
// the shop's non-ZR carriers, since the order form has no carrier selection);
// the management page calls it with deliveryCompanyId.
func ListBureaux(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	query := initializers.DB.
		Preload("DeliveryCompany.AvailableDeliveryCompany").
		Where("shop_id = ?", shopID)

	if raw := c.Query("deliveryCompanyId"); raw != "" {
		deliveryCompanyID, parseErr := uuid.Parse(raw)
		if parseErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid delivery company ID"})
			return
		}
		query = query.Where("delivery_company_id = ?", deliveryCompanyID)
	}

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

	deliveryCompanyID, err := uuid.Parse(input.DeliveryCompanyID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid delivery company ID"})
		return
	}

	name := normalizeBureauName(input.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Bureau name is required"})
		return
	}

	company, refusal := findEligibleShopCarrier(shopID, deliveryCompanyID)
	if refusal != "" {
		status := http.StatusBadRequest
		if refusal == "Database error" {
			status = http.StatusInternalServerError
		}
		c.JSON(status, gin.H{"success": false, "message": refusal})
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
		ShopID:            shopID,
		DeliveryCompanyID: company.ID,
		WilayaID:          input.WilayaID,
		Name:              name,
	}

	if err := initializers.DB.Create(&bureau).Error; err != nil {
		// idx_bureaux_unique_name (migration 00036) rejects the exact same
		// name twice for one shop + carrier + wilaya.
		if strings.Contains(strings.ToLower(err.Error()), "duplicate key") {
			c.JSON(http.StatusConflict, gin.H{
				"success": false,
				"message": "This bureau already exists for that carrier and wilaya",
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

	if err := initializers.DB.
		Preload("DeliveryCompany.AvailableDeliveryCompany").
		First(&bureau, "id = ?", bureau.ID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Bureau created but failed to reload record",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "Bureau created",
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

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Bureau deleted"})
}
