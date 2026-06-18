package controllers

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type CreateAbandonedLeadInput struct {
	ShopID           string  `json:"shopId" binding:"required"`
	ProductID        string  `json:"productId" binding:"required"`
	ProductTitle     string  `json:"productTitle"`
	Price            float64 `json:"price"`
	CombinationStr   string  `json:"combinationStr"`
	State            string  `json:"state"`
	City             string  `json:"city"`
	ShippingMethod   string  `json:"shippingMethod"`
	Quantity         int     `json:"quantity"`
	FullName         string  `json:"fullName" binding:"required"`
	PhoneNumber      string  `json:"phoneNumber" binding:"required"`
	FBclid           string  `json:"fbclid"`
	FBp              string  `json:"fbp"`
	FBc              string  `json:"fbc"`
	ConversionSource string  `json:"conversionSource"`
}

const abandonedLeadWindow = 1 * time.Hour

func abandonedLeadKey(shopID uuid.UUID, phone string) string {
	return fmt.Sprintf("ratelimit:abandoned:phone:%s:%s", shopID, phone)
}

func CreateAbandonedLead(c *gin.Context) {
	var body CreateAbandonedLeadInput
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}

	parsedShopID, err := uuid.Parse(body.ShopID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	parsedProductID, err := uuid.Parse(body.ProductID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid product ID"})
		return
	}

	// Rate limit: 1 abandoned lead per phone+shop per hour; silent accept on breach
	key := abandonedLeadKey(parsedShopID, body.PhoneNumber)
	set, redisErr := initializers.RClient.SetNX(initializers.Ctx, key, 1, abandonedLeadWindow).Result()
	if redisErr == nil && !set {
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "Lead recorded"})
		return
	}

	lead := models.AbandonedLead{
		ShopID:           parsedShopID,
		ProductID:        parsedProductID,
		ProductTitle:     body.ProductTitle,
		Price:            body.Price,
		CombinationStr:   body.CombinationStr,
		State:            body.State,
		City:             body.City,
		ShippingMethod:   body.ShippingMethod,
		Quantity:         body.Quantity,
		FullName:         body.FullName,
		PhoneNumber:      body.PhoneNumber,
		FBclid:           body.FBclid,
		FBp:              body.FBp,
		FBc:              body.FBc,
		ConversionSource: body.ConversionSource,
	}

	if err := initializers.DB.Create(&lead).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to save lead"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Lead recorded"})
}

func GetAbandonedLeadsByShopID(c *gin.Context) {
	shopIDStr := c.Param("shopId")
	shopID, err := uuid.Parse(shopIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	page := 1
	perPage := 10

	if p := c.Query("page"); p != "" {
		if parsed, parseErr := strconv.Atoi(p); parseErr == nil && parsed > 0 {
			page = parsed
		}
	}
	if pp := c.Query("perPage"); pp != "" {
		if parsed, parseErr := strconv.Atoi(pp); parseErr == nil && parsed > 0 {
			perPage = parsed
		}
	}

	baseQuery := initializers.DB.Model(&models.AbandonedLead{}).Where("shop_id = ?", shopID)

	var totalRows int64
	if err := baseQuery.Count(&totalRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to count leads"})
		return
	}

	totalPages := int(math.Ceil(float64(totalRows) / float64(perPage)))
	if totalPages == 0 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}
	offset := (page - 1) * perPage

	var leads []models.AbandonedLead
	if err := baseQuery.Order("created_at DESC").Limit(perPage).Offset(offset).Find(&leads).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to fetch leads"})
		return
	}

	pagination := utils.GetPaginationData(page, totalPages, "/abandoned-leads")

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"message":    "Leads retrieved successfully",
		"data":       leads,
		"pagination": pagination,
	})
}

func DeleteAbandonedLead(c *gin.Context) {
	shopIDStr := c.Param("shopId")
	shopID, err := uuid.Parse(shopIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	leadIDStr := c.Param("id")
	leadID, err := uuid.Parse(leadIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid lead ID"})
		return
	}

	var lead models.AbandonedLead
	if err := initializers.DB.First(&lead, "id = ? AND shop_id = ?", leadID, shopID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Lead not found"})
		return
	}

	if err := initializers.DB.Delete(&lead).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to delete lead"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Lead deleted"})
}
