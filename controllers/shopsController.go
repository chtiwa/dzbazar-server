package controllers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type CreateShopInput struct {
	Name        string `form:"name" binding:"required"`
	Slug        string `form:"slug" binding:"required"`
	Description string `form:"description"`
}

type UpdateShopInput struct {
	Name        *string `form:"name"`
	Slug        *string `form:"slug"`
	Description *string `form:"description"`
	IsActive    *bool   `form:"isActive"`
}

type MyShopResponse struct {
	Role string      `json:"role"`
	Shop models.Shop `json:"shop"`
}

// reserved for super admin
func GetShops(c *gin.Context) {}

func GetShopByID(c *gin.Context) {
	shopIDStr := c.Param("shopId")
	shopID, err := uuid.Parse(shopIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid Shop ID parameter format",
		})
		return
	}

	var shop models.Shop
	if err := initializers.DB.Preload("LogoImage").First(&shop, "id = ?", shopID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "Shop not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to retrieve shop",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Shop retrieved successfully",
		"data":    shop,
	})
}

func GetMyShops(c *gin.Context) {
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

	var memberships []models.ShopMember
	if err := initializers.DB.
		Preload("Shop").
		Preload("Shop.LogoImage").
		Where("user_id = ?", userData.ID).
		Find(&memberships).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to fetch user shop memberships",
			"error":   err.Error(),
		})
		return
	}

	response := make([]MyShopResponse, 0, len(memberships))
	for _, membership := range memberships {
		response = append(response, MyShopResponse{
			Role: membership.Role,
			Shop: membership.Shop,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "User shops fetched successfully",
		"data":    response,
	})
}

func IndexShopBySlug(c *gin.Context) {
	slug := strings.ToLower(strings.TrimSpace(c.Param("slug")))
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Shop slug is required",
		})
		return
	}

	var shop models.Shop
	err := initializers.DB.
		Preload("LogoImage").
		Where("slug = ?", slug).
		First(&shop).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "Shop not found",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to retrieve shop by slug",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Shop retrieved successfully",
		"data":    shop,
	})
}

func CreateShop(c *gin.Context) {
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

	var body CreateShopInput
	if err := c.ShouldBindWith(&body, binding.FormMultipart); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Validation failed for request parameters",
			"error":   err.Error(),
		})
		return
	}

	processedSlug := strings.ToLower(strings.TrimSpace(body.Slug))
	reg := regexp.MustCompile(`[^a-z0-9-]+`)
	processedSlug = reg.ReplaceAllString(processedSlug, "-")
	processedSlug = strings.Trim(processedSlug, "-")

	if len(processedSlug) < 3 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "The slug must be at least 3 alphanumeric characters long after normalization",
		})
		return
	}

	description := strings.TrimSpace(body.Description)
	if len(description) > 200 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Description cannot exceed 200 characters",
		})
		return
	}

	wilayas, err := initializers.GetStaticWilayas()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to load static wilayas configuration",
			"error":   err.Error(),
		})
		return
	}

	var uploadedLogoURL string

	file, err := c.FormFile("logoImage")
	if err == nil && file != nil {
		src, openErr := file.Open()
		if openErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "Failed to open uploaded logo image",
				"error":   openErr.Error(),
			})
			return
		}
		defer src.Close()

		buffer := make([]byte, 512)
		n, readErr := src.Read(buffer)
		if readErr != nil && readErr != io.EOF {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "Failed to read uploaded logo image",
				"error":   readErr.Error(),
			})
			return
		}

		contentType := http.DetectContentType(buffer[:n])
		if !strings.HasPrefix(contentType, "image/") {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "Only image files are allowed for the shop logo",
			})
			return
		}

		seeker, ok := src.(io.Seeker)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Failed to process uploaded logo image stream",
			})
			return
		}

		if _, seekErr := seeker.Seek(0, io.SeekStart); seekErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Failed to process uploaded logo image",
				"error":   seekErr.Error(),
			})
			return
		}

		cleanFileName := filepath.Base(file.Filename)
		key := fmt.Sprintf("uploads/shops/%s/%d_%s", processedSlug, time.Now().UnixNano(), cleanFileName)

		bucketName := os.Getenv("B2_BUCKET_NAME")
		b2Region := os.Getenv("B2_REGION")
		b2PublicBaseURL := strings.TrimSpace(os.Getenv("B2_PUBLIC_BASE_URL"))

		_, putErr := initializers.S3Client.PutObject(context.TODO(), &s3.PutObjectInput{
			Bucket:        aws.String(bucketName),
			Key:           aws.String(key),
			Body:          src,
			ContentType:   aws.String(contentType),
			ContentLength: aws.Int64(file.Size),
		})
		if putErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Failed to upload shop logo image",
				"error":   putErr.Error(),
			})
			return
		}

		if b2PublicBaseURL != "" {
			uploadedLogoURL = fmt.Sprintf("%s/%s", strings.TrimRight(b2PublicBaseURL, "/"), key)
		} else {
			uploadedLogoURL = fmt.Sprintf("https://%s.s3.%s.backblazeb2.com/%s", bucketName, b2Region, key)
		}
	}

	var shop models.Shop

	err = initializers.DB.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&models.Shop{}).Where("slug = ?", processedSlug).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return gorm.ErrDuplicatedKey
		}

		shop = models.Shop{
			Name:        strings.TrimSpace(body.Name),
			Slug:        processedSlug,
			Description: description,
			OwnerID:     userData.ID,
			IsActive:    true,
			IsVerified:  false,
		}

		if err := tx.Omit("Owner", "Members", "Products", "Orders", "Clients", "LogoImage").Create(&shop).Error; err != nil {
			return err
		}

		if uploadedLogoURL != "" {
			logoImage := models.ShopLogoImage{
				ShopID: shop.ID,
				URL:    uploadedLogoURL,
			}

			if err := tx.Create(&logoImage).Error; err != nil {
				return err
			}

			shop.LogoImage = &logoImage
		}

		membership := models.ShopMember{
			ShopID: shop.ID,
			UserID: userData.ID,
			Role:   "owner",
		}
		if err := tx.Create(&membership).Error; err != nil {
			return err
		}

		rates := make([]models.DeliveryRate, 0, len(wilayas))
		for _, wilaya := range wilayas {
			rates = append(rates, models.DeliveryRate{
				ShopID:       shop.ID,
				WilayaID:     wilaya.ID,
				WilayaName:   wilaya.Name,
				IsActive:     wilaya.IsActive,
				HasDoorstep:  wilaya.HasDoorstep,
				DoorstepRate: wilaya.DoorstepRate,
				HasStopdesk:  wilaya.HasStopdesk,
				StopdeskRate: wilaya.StopdeskRate,
			})
		}

		if len(rates) > 0 {
			if err := tx.Create(&rates).Error; err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
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

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "Shop workspace initialized successfully",
		"data":    shop,
	})
}

func UpdateShop(c *gin.Context) {
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

	shopIDStr := c.Param("shopId")
	shopID, err := uuid.Parse(shopIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid Shop ID parameter format",
		})
		return
	}

	var input UpdateShopInput
	if err := c.ShouldBind(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Validation failed for request parameters",
			"error":   err.Error(),
		})
		return
	}

	var shop models.Shop
	if err := initializers.DB.Preload("LogoImage").First(&shop, "id = ?", shopID).Error; err != nil {
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

	if shop.OwnerID != userData.ID {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "You do not have administrative ownership over this shop workspace",
		})
		return
	}

	updateData := make(map[string]interface{})

	if input.Name != nil {
		trimmedName := strings.TrimSpace(*input.Name)
		if trimmedName == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "Shop name cannot be empty",
			})
			return
		}
		updateData["name"] = trimmedName
	}

	if input.Description != nil {
		trimmedDescription := strings.TrimSpace(*input.Description)
		if len(trimmedDescription) > 200 {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "Description cannot exceed 200 characters",
			})
			return
		}
		updateData["description"] = trimmedDescription
	}

	if input.IsActive != nil {
		updateData["is_active"] = *input.IsActive
	}

	if input.Slug != nil {
		processedSlug := strings.ToLower(strings.TrimSpace(*input.Slug))
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

		if processedSlug != shop.Slug {
			var count int64
			err := initializers.DB.Model(&models.Shop{}).
				Where("slug = ? AND id != ?", processedSlug, shop.ID).
				Count(&count).Error

			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"success": false,
					"message": "Database tracking error",
				})
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

	var uploadedLogoURL string
	file, err := c.FormFile("logoImage")
	if err == nil && file != nil {
		src, openErr := file.Open()
		if openErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "Failed to open uploaded logo image",
				"error":   openErr.Error(),
			})
			return
		}
		defer src.Close()

		buffer := make([]byte, 512)
		n, _ := src.Read(buffer)
		contentType := http.DetectContentType(buffer[:n])

		if !strings.HasPrefix(contentType, "image/") {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "Only image files are allowed for the shop logo",
			})
			return
		}

		if _, seekErr := src.Seek(0, io.SeekStart); seekErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Failed to process uploaded logo image",
				"error":   seekErr.Error(),
			})
			return
		}

		targetSlug := shop.Slug
		if slugVal, exists := updateData["slug"].(string); exists && slugVal != "" {
			targetSlug = slugVal
		}

		cleanFileName := filepath.Base(file.Filename)
		key := fmt.Sprintf("uploads/shops/%s/%d_%s", targetSlug, time.Now().UnixNano(), cleanFileName)

		bucketName := os.Getenv("B2_BUCKET_NAME")
		b2Region := os.Getenv("B2_REGION")
		b2PublicBaseURL := strings.TrimSpace(os.Getenv("B2_PUBLIC_BASE_URL"))

		_, putErr := initializers.S3Client.PutObject(context.TODO(), &s3.PutObjectInput{
			Bucket:        aws.String(bucketName),
			Key:           aws.String(key),
			Body:          src,
			ContentType:   aws.String(contentType),
			ContentLength: aws.Int64(file.Size),
		})
		if putErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Failed to upload shop logo image",
				"error":   putErr.Error(),
			})
			return
		}

		if b2PublicBaseURL != "" {
			uploadedLogoURL = fmt.Sprintf("%s/%s", strings.TrimRight(b2PublicBaseURL, "/"), key)
		} else {
			uploadedLogoURL = fmt.Sprintf("https://%s.s3.%s.backblazeb2.com/%s", bucketName, b2Region, key)
		}

		updateData["logo_url"] = uploadedLogoURL
	}

	err = initializers.DB.Transaction(func(tx *gorm.DB) error {
		if len(updateData) > 0 {
			if err := tx.Model(&shop).Updates(updateData).Error; err != nil {
				return err
			}
		}

		if uploadedLogoURL != "" {
			var existingLogo models.ShopLogoImage
			logoErr := tx.Where("shop_id = ?", shop.ID).First(&existingLogo).Error

			if logoErr != nil {
				if logoErr == gorm.ErrRecordNotFound {
					newLogo := models.ShopLogoImage{
						ShopID: shop.ID,
						URL:    uploadedLogoURL,
					}
					if err := tx.Create(&newLogo).Error; err != nil {
						return err
					}
				} else {
					return logoErr
				}
			} else {
				if err := tx.Model(&existingLogo).Update("url", uploadedLogoURL).Error; err != nil {
					return err
				}
			}
		}

		return nil
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed updating shop workspace parameters downstream",
			"error":   err.Error(),
		})
		return
	}

	var updatedShop models.Shop
	if err := initializers.DB.Preload("LogoImage").First(&updatedShop, "id = ?", shop.ID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Shop updated but failed to reload fresh state",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Shop workspace parameters updated successfully",
		"data":    updatedShop,
	})
}

func DeleteShop(c *gin.Context) {}
