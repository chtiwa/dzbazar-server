package controllers

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type CreateShopInput struct {
	Name        string `json:"name" binding:"required,min=3,max=50"`
	Slug        string `json:"slug" binding:"required"`
	Description string `json:"description" binding:"max=500"`
	LogoURL     string `json:"logoUrl"`
}

type UpdateShopInput struct {
	Name        string `json:"name" binding:"omitempty,min=3,max=50"`
	Slug        string `json:"slug"`
	Description string `json:"description" binding:"omitempty,max=500"`
	LogoURL     string `json:"logoUrl"`
	IsActive    *bool  `json:"isActive"` // Pinned to a pointer to allow explicit false updates
}

// reserved for super admin
func GetShops(c *gin.Context) {}

func CreateShop(c *gin.Context) {
	// 1. Authenticate and extract user from context middleware
	user, ok := c.Get("user")
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to retrieve authenticated session user",
		})
		return
	}

	userData, ok := user.(models.User)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Invalid session user structure context",
		})
		return
	}

	// 2. Bind and validate the JSON input payload
	var body CreateShopInput
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Validation failed for request parameters",
			"error":   err.Error(),
		})
		return
	}

	// 3. Clean and normalize the Slug identifier
	// URL slugs must look like: "my-shop-name" (alphanumeric and hyphens only)
	processedSlug := strings.ToLower(strings.TrimSpace(body.Slug))
	reg, _ := regexp.Compile("[^a-z0-9-]+")
	processedSlug = reg.ReplaceAllString(processedSlug, "-")
	processedSlug = strings.Trim(processedSlug, "-")

	if len(processedSlug) < 3 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "The slug must be at least 3 alphanumeric characters long after normalization",
		})
		return
	}

	// 4. Instantiate the Shop within a transactional boundary
	var shop models.Shop

	err := initializers.DB.Transaction(func(tx *gorm.DB) error {
		// Double check if slug is uniquely available
		var count int64
		if err := tx.Model(&models.Shop{}).Where("slug = ?", processedSlug).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return gorm.ErrDuplicatedKey // Custom trigger to exit transaction block if slug is taken
		}

		// Build out the Shop record mapped directly to the Owner ID
		shop = models.Shop{
			Name:        strings.TrimSpace(body.Name),
			Slug:        processedSlug,
			Description: strings.TrimSpace(body.Description),
			OwnerID:     userData.ID, // Links the user who made the request as the Shop owner
			LogoURL:     strings.TrimSpace(body.LogoURL),
			IsActive:    true,  // Enabled immediately upon creation
			IsVerified:  false, // Verification requires a platform admin manually overriding flags later
		}

		if err := tx.Create(&shop).Error; err != nil {
			return err
		}

		// 2. Create the Membership link declaring this user as the 'Owner'
		membership := models.ShopMember{
			ShopID: shop.ID,
			UserID: userData.ID,
			Role:   "Owner",
		}
		if err := tx.Create(&membership).Error; err != nil {
			return err
		}

		return nil
	})

	// 5. Handle transactional failures
	if err != nil {
		if err == gorm.ErrDuplicatedKey {
			c.JSON(http.StatusConflict, gin.H{
				"success": false,
				"message": "This storefront URL slug is already taken by another merchant",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "An unexpected error occurred while instantiating the workspace",
			"error":   err.Error(),
		})
		return
	}

	// 6. Return successful creation status code back to client application
	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "Shop workspace initialized successfully",
		"data":    shop,
	})
}

// TODO : Add image upload in the case of an updated image
func UpdateShop(c *gin.Context) {
	// 1. Authenticate the session user
	user, ok := c.Get("user")
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to retrieve authenticated session user",
		})
		return
	}

	userData, ok := user.(models.User)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Invalid session user structure context",
		})
		return
	}

	// 2. Extract and validate Shop ID from path parameters
	shopIDStr := c.Param("shopId")
	shopID, err := uuid.Parse(shopIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid Shop ID parameter format",
		})
		return
	}

	// 3. Bind the incoming request body
	var input UpdateShopInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Validation failed for request parameters",
			"error":   err.Error(),
		})
		return
	}

	// 4. Fetch the existing shop and verify operational ownership boundaries
	var shop models.Shop
	if err := initializers.DB.First(&shop, "id = ?", shopID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "Shop workspace not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Database tracking error",
		})
		return
	}

	// Security Check: Enforce that only the assigned OwnerID can modify this profile
	if shop.OwnerID != userData.ID {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "You do not have administrative ownership over this shop workspace",
		})
		return
	}

	// 5. Build safe dynamic update map data structures
	updateData := make(map[string]interface{})

	if input.Name != "" {
		updateData["name"] = strings.TrimSpace(input.Name)
	}

	if input.Description != "" {
		updateData["description"] = strings.TrimSpace(input.Description)
	}

	if input.LogoURL != "" {
		updateData["logo_url"] = strings.TrimSpace(input.LogoURL)
	}

	if input.IsActive != nil {
		updateData["is_active"] = *input.IsActive
	}

	// 6. Handle optional storefront Slug mutation rules securely
	if input.Slug != "" {
		processedSlug := strings.ToLower(strings.TrimSpace(input.Slug))
		reg, _ := regexp.Compile("[^a-z0-9-]+")
		processedSlug = reg.ReplaceAllString(processedSlug, "-")
		processedSlug = strings.Trim(processedSlug, "-")

		if len(processedSlug) < 3 {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "The slug must be at least 3 alphanumeric characters long after normalization",
			})
			return
		}

		// Only process uniqueness execution checks if the slug value has explicitly changed
		if processedSlug != shop.Slug {
			var count int64
			err := initializers.DB.Model(&models.Shop{}).
				Where("slug = ? AND id != ?", processedSlug, shop.ID).
				Count(&count).Error

			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Database tracking error"})
				return
			}
			if count > 0 {
				c.JSON(http.StatusConflict, gin.H{
					"success": false,
					"message": "This storefront URL slug is already taken by another merchant",
				})
				return
			}
			updateData["slug"] = processedSlug
		}
	}

	// 7. Execute structural updates within database engine
	if len(updateData) > 0 {
		if err := initializers.DB.Model(&shop).Updates(updateData).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Failed updating shop workspace parameters downstream",
				"error":   err.Error(),
			})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Shop workspace parameters updated successfully",
		"data":    shop,
	})
}

func DeleteShop(c *gin.Context) {}
