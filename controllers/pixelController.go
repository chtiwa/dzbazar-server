package controllers

import (
	"net/http"
	"strings"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type UpdatePixelInput struct {
	Title       string `json:"title"`
	PixelID     string `json:"pixelId"`
	AccessToken string `json:"accessToken"`
}

func GetPixelsByShop(c *gin.Context) {
	// get the shop id from the user in the context
	// 1. Extract and validate Shop ID context from path parameter to enforce isolation
	shopIDStr := c.Param("shopId")
	shopID, err := uuid.Parse(shopIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid Shop ID parameter format",
		})
		return
	}

	// Optional filtering: Allow the frontend admin to query pixels of a specific platform (e.g., ?platform=facebook)
	platformFilter := c.Query("platform")

	var pixels []models.Pixel

	// 2. Build thread-safe, isolated database query context
	query := initializers.DB.Model(&models.Pixel{}).Where("shop_id = ?", shopID)

	if platformFilter != "" {
		query = query.Where("platform = ?", platformFilter)
	}

	// 3. Retrieve pixel list (ordering by newest configuration profile first)
	if err := query.Order("created_at DESC").Find(&pixels).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Error while retrieving pixel configurations",
			"error":   err.Error(),
		})
		return
	}

	// 4. Return clean payload collection to the frontend UI components
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Pixel profiles retrieved successfully",
		"count":   len(pixels),
		"data":    pixels,
	})
}

func IndexPixel(c *gin.Context) {
	// get the shop id from the user in the context
	// 1. Enforce multi-tenancy constraints explicitly
	shopIDStr := c.Param("shopId")
	shopID, err := uuid.Parse(shopIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid or missing Shop ID framework context",
		})
		return
	}

	// 2. Extract specific primary identification key
	pixelIDStr := c.Param("id")
	pixelUUID, err := uuid.Parse(pixelIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid Pixel Reference ID configuration format",
		})
		return
	}

	var pixel models.Pixel

	// 3. Safe, tenant-isolated lookups to lock access down securely
	err = initializers.DB.Model(&models.Pixel{}).
		Where("id = ? AND shop_id = ?", pixelUUID, shopID).
		First(&pixel).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "Pixel element not found or doesn't belong to this store partition",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed looking up profile records down-stream",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Pixel configuration loaded successfully",
		"data":    pixel,
	})
}

func CreatePixel(c *gin.Context) {
	// 1. Authenticate and authorize context session
	user, ok := c.Get("user")
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Error while fetching the session user",
		})
		return
	}

	userData, ok := user.(models.User)
	// Ensuring user has administrative privileges within their workspace context
	if !ok || userData.Role != "Admin" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Unauthorized access to tenant tracking controls",
		})
		return
	}

	// 2. Map and parse incoming JSON payload structure
	var body struct {
		ShopID      string `json:"shopId" binding:"required"`
		Platform    string `json:"platform" binding:"required"` // e.g., "facebook", "tiktok"
		Title       string `json:"title" binding:"required"`    // Custom descriptive identifier
		PixelID     string `json:"pixelId" binding:"required"`
		AccessToken string `json:"accessToken"` // Optional, used for Server-Side Conversions API
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request body parameters",
			"error":   err.Error(),
		})
		return
	}

	// 3. Parse and validate UUID configurations safely

	// Standardize platform values to lowercase to guarantee query uniformity
	platformNormalized := strings.ToLower(strings.TrimSpace(body.Platform))
	accessTokenClean := strings.TrimSpace(body.AccessToken)
	hasToken := accessTokenClean != ""

	// 4. Build exact database model structure mapping schema constraints
	pixel := models.Pixel{
		ShopID:         *userData.ShopID,
		Platform:       platformNormalized,
		Title:          body.Title,
		PixelID:        strings.TrimSpace(body.PixelID),
		HasAccessToken: hasToken,
		AccessToken:    accessTokenClean,
	}

	// 5. Execute insert operation against persistence engine
	result := initializers.DB.Create(&pixel)
	if result.Error != nil {
		// Catch unique composite constraint conflicts (e.g., duplicate pixel per platform)
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"message": "Failed to register tracking component. Verify it isn't already declared.",
			"error":   result.Error.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "Tracking element instantiated successfully",
		"data":    pixel,
	})
}

func UpdatePixel(c *gin.Context) {
	// 1. Authenticate and authorize context session
	user, ok := c.Get("user")
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Error while fetching the session user",
		})
		return
	}

	userData, ok := user.(models.User)
	if !ok || userData.Role != "Admin" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Unauthorized access to tenant tracking controls",
		})
		return
	}

	// 2. Extract context path parameters to enforce strict multi-tenancy boundaries

	pixelIDStr := c.Param("id") // The primary key (UUID) of the Pixel row itself
	pixelUUID, err := uuid.Parse(pixelIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid Pixel Record ID format"})
		return
	}

	// 3. Bind the incoming partial update body context
	var input UpdatePixelInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request parameters provided",
			"error":   err.Error(),
		})
		return
	}

	// 4. Locate the existing tracking configuration record within tenant bounds
	var pixel models.Pixel
	err = initializers.DB.Where("id = ? AND shop_id = ?", pixelUUID, userData.ShopID).First(&pixel).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "Tracking element not found or does not belong to this shop workspace",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Database lookup execution error",
		})
		return
	}

	// 5. Build dynamic update structural changes selectively
	updateData := make(map[string]interface{})

	if input.Title != "" {
		updateData["title"] = strings.TrimSpace(input.Title)
	}

	if input.PixelID != "" {
		updateData["pixel_id"] = strings.TrimSpace(input.PixelID)
	}

	// If the access token is explicitly included in the request body, process it
	if input.AccessToken != "" {
		cleanToken := strings.TrimSpace(input.AccessToken)
		updateData["access_token"] = cleanToken
		updateData["has_access_token"] = true
	} else if c.Copy().Keys["clearToken"] == true {
		// Optional fallback block logic if your dashboard frontend explicitly passes an indicator to purge token states
		updateData["access_token"] = ""
		updateData["has_access_token"] = false
	}

	// 6. Persist execution parameters cleanly via GORM Updates
	if len(updateData) > 0 {
		if err := initializers.DB.Model(&pixel).Updates(updateData).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Failed updating tracking configuration fields down-stream",
				"error":   err.Error(),
			})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Tracking element updated successfully",
		"data":    pixel,
	})
}

func DeletePixel(c *gin.Context) {
	// 1. Authenticate and authorize context session user
	user, ok := c.Get("user")
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Error while fetching the session user",
		})
		return
	}

	userData, ok := user.(models.User)
	if !ok || userData.Role != "Admin" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Unauthorized access to tenant tracking controls",
		})
		return
	}

	// 2. Extract context path parameters to enforce strict multi-tenancy boundaries
	shopIDStr := c.Param("shopId")
	shopID, err := uuid.Parse(shopIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid Shop ID parameter format",
		})
		return
	}

	pixelIDStr := c.Param("id") // The primary key (UUID) of the Pixel target
	pixelUUID, err := uuid.Parse(pixelIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid Pixel Record ID format",
		})
		return
	}

	// 3. Locate the pixel target ensuring it belongs strictly to this tenant boundary
	var pixel models.Pixel
	err = initializers.DB.Where("id = ? AND shop_id = ?", pixelUUID, shopID).First(&pixel).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "Tracking element not found or does not belong to this shop workspace",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Database lookup execution error",
		})
		return
	}

	// 4. Perform the deletion operation
	// If your BaseModel implements gorm.DeletedAt, this will execute a safe Soft Delete automatically.
	if err := initializers.DB.Delete(&pixel).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to remove the pixel configuration from the server database",
			"error":   err.Error(),
		})
		return
	}

	// 5. Return clean confirmation state back to your admin client dashboard
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Tracking element deleted successfully",
	})
}
