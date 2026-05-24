package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/chtiwa/dzbazar-server/dto"
	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const maxLandingPageImages = 10

func landingPageCacheKeyByID(id uuid.UUID) string {
	return fmt.Sprintf("landing-page:id=%s", id.String())
}

func landingPagesCacheKeyByShop(shopID uuid.UUID) string {
	return fmt.Sprintf("landing-pages:shop=%s", shopID.String())
}

func invalidateLandingPageCaches(shopID uuid.UUID, landingPageID uuid.UUID) {
	keys := []string{
		landingPageCacheKeyByID(landingPageID),
		landingPagesCacheKeyByShop(shopID),
	}
	if err := initializers.RClient.Del(initializers.Ctx, keys...).Err(); err != nil {
		fmt.Println("Failed to delete landing page cache keys:", err)
	}
}

func loadLandingPageByShop(tx *gorm.DB, shopID, landingPageID uuid.UUID, landingPage *models.LandingPage) error {
	return tx.
		Where("id = ? AND shop_id = ?", landingPageID, shopID).
		Preload("Images", func(db *gorm.DB) *gorm.DB {
			return db.Order("order_index ASC")
		}).
		Preload("Product").
		Preload("Product.Images", func(db *gorm.DB) *gorm.DB {
			return db.Order("order_index ASC")
		}).
		Preload("Product.Variants").
		Preload("Product.Variants.VariantItems").
		Preload("Product.Combinations").
		Preload("Product.Combinations.Option1").
		Preload("Product.Combinations.Option2").
		Preload("Product.Combinations.Option3").
		First(landingPage).Error
}

func uploadLandingPageFiles(shopID uuid.UUID, files []*multipart.FileHeader) ([]models.LandingPageImage, []string, error) {
	bucketName := os.Getenv("B2_BUCKET_NAME")
	region := os.Getenv("B2_REGION")

	var uploadedKeys []string
	var images []models.LandingPageImage

	for index, file := range files {
		if !strings.HasPrefix(file.Header.Get("Content-Type"), "image/") {
			return nil, uploadedKeys, fmt.Errorf("only image files are allowed")
		}

		src, err := file.Open()
		if err != nil {
			return nil, uploadedKeys, err
		}

		key := fmt.Sprintf(
			"uploads/landing-pages/%s/%d_%s",
			shopID.String(),
			time.Now().UnixNano(),
			filepath.Base(file.Filename),
		)

		_, err = initializers.S3Client.PutObject(context.Background(), &s3.PutObjectInput{
			Bucket:      aws.String(bucketName),
			Key:         aws.String(key),
			Body:        src,
			ACL:         types.ObjectCannedACLPublicRead,
			ContentType: aws.String(file.Header.Get("Content-Type")),
		})
		src.Close()

		if err != nil {
			return nil, uploadedKeys, err
		}

		uploadedKeys = append(uploadedKeys, key)

		url := fmt.Sprintf("https://%s.s3.%s.backblazeb2.com/%s", bucketName, region, key)
		images = append(images, models.LandingPageImage{
			URL:        url,
			OrderIndex: index,
		})
	}

	return images, uploadedKeys, nil
}

func cleanupUploadedKeys(keys []string) {
	if len(keys) == 0 {
		return
	}

	bucketName := os.Getenv("B2_BUCKET_NAME")
	for _, key := range keys {
		_, _ = initializers.S3Client.DeleteObject(context.Background(), &s3.DeleteObjectInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String(key),
		})
	}
}

func CreateLandingPageByShop(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid shop ID",
			"error":   err.Error(),
		})
		return
	}

	productID, err := uuid.Parse(c.PostForm("productId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid product ID",
			"error":   err.Error(),
		})
		return
	}

	title := strings.TrimSpace(c.PostForm("title"))
	if title == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Title is required",
		})
		return
	}

	var product models.Product
	if err := initializers.DB.
		Where("id = ? AND shop_id = ?", productID, shopID).
		First(&product).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Product not found",
			"error":   err.Error(),
		})
		return
	}

	form, err := c.MultipartForm()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid multipart form data",
			"error":   err.Error(),
		})
		return
	}

	files := form.File["images"]
	if len(files) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "At least one image is required",
		})
		return
	}

	if len(files) > maxLandingPageImages {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": fmt.Sprintf("The maximum number of images is %d", maxLandingPageImages),
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

	defer func() {
		if r := recover(); r != nil {
			debug.PrintStack()
			tx.Rollback()
			panic(r)
		}
	}()

	landingPage := models.LandingPage{
		ShopID:    shopID,
		ProductID: productID,
		Title:     title,
		Active:    true,
	}

	if err := tx.Create(&landingPage).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to create landing page",
			"error":   err.Error(),
		})
		return
	}

	uploadedImages, uploadedKeys, err := uploadLandingPageFiles(shopID, files)
	if err != nil {
		tx.Rollback()
		cleanupUploadedKeys(uploadedKeys)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to upload landing page images",
			"error":   err.Error(),
		})
		return
	}

	for i := range uploadedImages {
		uploadedImages[i].LandingPageID = landingPage.ID
		uploadedImages[i].OrderIndex = i
	}

	if len(uploadedImages) > 0 {
		if err := tx.Create(&uploadedImages).Error; err != nil {
			tx.Rollback()
			cleanupUploadedKeys(uploadedKeys)
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Failed to save landing page images",
				"error":   err.Error(),
			})
			return
		}
	}

	var createdLandingPage models.LandingPage
	if err := loadLandingPageByShop(tx, shopID, landingPage.ID, &createdLandingPage); err != nil {
		tx.Rollback()
		cleanupUploadedKeys(uploadedKeys)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to reload landing page",
			"error":   err.Error(),
		})
		return
	}

	if err := tx.Commit().Error; err != nil {
		cleanupUploadedKeys(uploadedKeys)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to commit transaction",
			"error":   err.Error(),
		})
		return
	}

	invalidateLandingPageCaches(shopID, landingPage.ID)

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "Landing page created successfully",
		"data":    createdLandingPage,
	})
}

func GetLandingPagesByShop(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid shop ID",
			"error":   err.Error(),
		})
		return
	}

	cacheKey := landingPagesCacheKeyByShop(shopID)
	val, err := initializers.RClient.Get(initializers.Ctx, cacheKey).Result()
	if err == nil {
		var cachedResponse []models.LandingPage
		if unmarshalErr := json.Unmarshal([]byte(val), &cachedResponse); unmarshalErr == nil {
			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"message": "Landing pages retrieved successfully (from cache)",
				"data":    cachedResponse,
			})
			return
		}
	}

	var landingPages []models.LandingPage
	if err := initializers.DB.
		Where("shop_id = ?", shopID).
		Preload("Images", func(db *gorm.DB) *gorm.DB {
			return db.Order("order_index ASC")
		}).
		Preload("Product").
		Order("created_at DESC").
		Find(&landingPages).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to retrieve landing pages",
			"error":   err.Error(),
		})
		return
	}

	if jsonData, err := json.Marshal(landingPages); err == nil {
		_ = initializers.RClient.Set(initializers.Ctx, cacheKey, jsonData, 10*time.Minute).Err()
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Landing pages retrieved successfully",
		"data":    landingPages,
	})
}

func GetLandingPageByShop(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid shop ID",
			"error":   err.Error(),
		})
		return
	}

	landingPageID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid landing page ID",
			"error":   err.Error(),
		})
		return
	}

	cacheKey := landingPageCacheKeyByID(landingPageID)
	val, err := initializers.RClient.Get(initializers.Ctx, cacheKey).Result()
	if err == nil {
		var cachedResponse models.LandingPage
		if unmarshalErr := json.Unmarshal([]byte(val), &cachedResponse); unmarshalErr == nil {
			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"message": "Landing page retrieved successfully (from cache)",
				"data":    cachedResponse,
			})
			return
		}
	}

	var landingPage models.LandingPage
	if err := loadLandingPageByShop(initializers.DB, shopID, landingPageID, &landingPage); err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Landing page not found",
			"error":   err.Error(),
		})
		return
	}

	if jsonData, err := json.Marshal(landingPage); err == nil {
		_ = initializers.RClient.Set(initializers.Ctx, cacheKey, jsonData, 10*time.Minute).Err()
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Landing page retrieved successfully",
		"data":    landingPage,
	})
}

func IndexLandingPage(c *gin.Context) {
	landingPageID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid landing page ID",
			"error":   err.Error(),
		})
		return
	}

	cacheKey := landingPageCacheKeyByID(landingPageID)
	val, err := initializers.RClient.Get(initializers.Ctx, cacheKey).Result()
	if err == nil {
		var cachedResponse models.LandingPage
		if unmarshalErr := json.Unmarshal([]byte(val), &cachedResponse); unmarshalErr == nil {
			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"message": "Landing page retrieved successfully (from cache)",
				"data":    cachedResponse,
			})
			return
		}
	}

	var landingPage models.LandingPage
	if err := initializers.DB.
		Where("id = ? AND active = ?", landingPageID, true).
		Preload("Images", func(db *gorm.DB) *gorm.DB {
			return db.Order("order_index ASC")
		}).
		Preload("Product").
		Preload("Product.Images", func(db *gorm.DB) *gorm.DB {
			return db.Order("order_index ASC")
		}).
		Preload("Product.Variants").
		Preload("Product.Variants.VariantItems").
		Preload("Product.Combinations").
		Preload("Product.Combinations.Option1").
		Preload("Product.Combinations.Option2").
		Preload("Product.Combinations.Option3").
		First(&landingPage).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Landing page not found",
			"error":   err.Error(),
		})
		return
	}

	if jsonData, err := json.Marshal(landingPage); err == nil {
		_ = initializers.RClient.Set(initializers.Ctx, cacheKey, jsonData, 10*time.Minute).Err()
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Landing page retrieved successfully",
		"data":    landingPage,
	})
}

func UpdateLandingPageByShop(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid shop ID",
			"error":   err.Error(),
		})
		return
	}

	landingPageID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid landing page ID",
			"error":   err.Error(),
		})
		return
	}

	var landingPage models.LandingPage
	if err := initializers.DB.
		Where("id = ? AND shop_id = ?", landingPageID, shopID).
		First(&landingPage).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Landing page not found",
			"error":   err.Error(),
		})
		return
	}

	title := strings.TrimSpace(c.PostForm("title"))
	activeValue := strings.TrimSpace(c.PostForm("active"))

	var updates = map[string]interface{}{}

	if title != "" {
		updates["title"] = title
	}

	if activeValue != "" {
		active := activeValue == "true"
		updates["active"] = active
	}

	var currentImages []models.LandingPageImage
	if err := initializers.DB.
		Where("landing_page_id = ?", landingPageID).
		Order("order_index ASC, created_at ASC").
		Find(&currentImages).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to retrieve landing page images",
			"error":   err.Error(),
		})
		return
	}

	existingImagesJSON := c.PostForm("existingImages")
	var existingImages []dto.UpdateLandingPageImageInput
	if existingImagesJSON != "" {
		if err := json.Unmarshal([]byte(existingImagesJSON), &existingImages); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "Invalid existingImages JSON",
				"error":   err.Error(),
			})
			return
		}
	}

	keptImageIDs := make(map[string]struct{}, len(existingImages))
	for _, img := range existingImages {
		if img.ID == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "Existing image ID cannot be empty",
			})
			return
		}
		keptImageIDs[img.ID] = struct{}{}
	}

	currentImageIDs := make(map[string]models.LandingPageImage, len(currentImages))
	for _, img := range currentImages {
		currentImageIDs[img.ID.String()] = img
	}

	for keptID := range keptImageIDs {
		if _, exists := currentImageIDs[keptID]; !exists {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "One or more existing image IDs do not belong to this landing page",
			})
			return
		}
	}

	var imagesToDelete []models.LandingPageImage
	for _, dbImage := range currentImages {
		if _, keep := keptImageIDs[dbImage.ID.String()]; !keep {
			imagesToDelete = append(imagesToDelete, dbImage)
		}
	}

	form, err := c.MultipartForm()
	if err != nil && err != http.ErrNotMultipart {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid multipart form data",
			"error":   err.Error(),
		})
		return
	}

	var files []*multipart.FileHeader
	if form != nil && form.File != nil {
		files = form.File["images"]
	}

	if len(existingImages)+len(files) > maxLandingPageImages {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": fmt.Sprintf("The maximum number of images is %d", maxLandingPageImages),
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

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	if len(updates) > 0 {
		if err := tx.Model(&landingPage).Updates(updates).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Failed to update landing page",
				"error":   err.Error(),
			})
			return
		}
	}

	for _, img := range imagesToDelete {
		key := utils.ExtractB2KeyFromURL(img.URL)
		if key != "" {
			bucketName := os.Getenv("B2_BUCKET_NAME")
			_, err := initializers.S3Client.DeleteObject(context.Background(), &s3.DeleteObjectInput{
				Bucket: aws.String(bucketName),
				Key:    aws.String(key),
			})
			if err != nil {
				fmt.Printf("Warning: failed to delete old file %s from B2: %v\n", key, err)
			}
		}

		if err := tx.Where("id = ? AND landing_page_id = ?", img.ID, landingPageID).
			Delete(&models.LandingPageImage{}).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Failed to delete removed images",
				"error":   err.Error(),
			})
			return
		}
	}

	uploadedImages, uploadedKeys, err := uploadLandingPageFiles(shopID, files)
	if err != nil {
		tx.Rollback()
		cleanupUploadedKeys(uploadedKeys)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to upload new images",
			"error":   err.Error(),
		})
		return
	}

	for i := range uploadedImages {
		uploadedImages[i].LandingPageID = landingPageID
		uploadedImages[i].OrderIndex = len(existingImages) + i
	}

	if len(uploadedImages) > 0 {
		if err := tx.Create(&uploadedImages).Error; err != nil {
			tx.Rollback()
			cleanupUploadedKeys(uploadedKeys)
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Failed to save new images",
				"error":   err.Error(),
			})
			return
		}
	}

	var updatedLandingPage models.LandingPage
	if err := loadLandingPageByShop(tx, shopID, landingPageID, &updatedLandingPage); err != nil {
		tx.Rollback()
		cleanupUploadedKeys(uploadedKeys)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to reload updated landing page",
			"error":   err.Error(),
		})
		return
	}

	if err := tx.Commit().Error; err != nil {
		cleanupUploadedKeys(uploadedKeys)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to commit transaction",
			"error":   err.Error(),
		})
		return
	}

	invalidateLandingPageCaches(shopID, landingPageID)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Landing page updated successfully",
		"data":    updatedLandingPage,
	})
}

func DeleteLandingPageByShop(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid shop ID",
			"error":   err.Error(),
		})
		return
	}

	landingPageID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid landing page ID",
			"error":   err.Error(),
		})
		return
	}

	var landingPage models.LandingPage
	if err := initializers.DB.
		Where("id = ? AND shop_id = ?", landingPageID, shopID).
		First(&landingPage).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Landing page not found",
			"error":   err.Error(),
		})
		return
	}

	var images []models.LandingPageImage
	if err := initializers.DB.
		Where("landing_page_id = ?", landingPageID).
		Find(&images).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to load landing page images",
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

	if err := tx.Delete(&models.LandingPage{}, "id = ? AND shop_id = ?", landingPageID, shopID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to delete landing page",
			"error":   err.Error(),
		})
		return
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to commit transaction",
			"error":   err.Error(),
		})
		return
	}

	bucketName := os.Getenv("B2_BUCKET_NAME")
	for _, image := range images {
		key := utils.ExtractB2KeyFromURL(image.URL)
		if key != "" {
			_, _ = initializers.S3Client.DeleteObject(context.Background(), &s3.DeleteObjectInput{
				Bucket: aws.String(bucketName),
				Key:    aws.String(key),
			})
		}
	}

	invalidateLandingPageCaches(shopID, landingPageID)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Landing page deleted successfully",
	})
}
