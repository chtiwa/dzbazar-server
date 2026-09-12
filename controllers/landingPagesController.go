package controllers

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/chtiwa/dzbazar-server/services"
	"github.com/chtiwa/dzbazar-server/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const maxLandingPageImages = 10
const maxLandingPageImageSize = 10 * 1024 * 1024 // 10 MB

// landingPageCacheKeyByID, landingPagesCacheKeyByShop, invalidateLandingPageCaches
// live in services (LandingPageCacheKeyByID / LandingPagesCacheKeyByShop /
// InvalidateLandingPageCaches) so services/experiments.go can invalidate them too.

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
		Preload("Product.Combinations", "retired = ?", false).
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

		if file.Size > maxLandingPageImageSize {
			return nil, uploadedKeys, fmt.Errorf("image %q exceeds the 10 MB size limit", file.Filename)
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

	if err := services.CheckLandingPageLimit(shopID); err != nil {
		if errors.Is(err, services.ErrPlanLimitReached) {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "Landing page limit reached for your plan. Upgrade to create more.",
				"code":    "PLAN_LIMIT_REACHED",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to verify plan limits", "error": err.Error()})
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

	services.InvalidateLandingPageCaches(shopID, landingPage.ID)

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

	search := strings.TrimSpace(c.Query("search"))

	var cacheKey string
	if search == "" {
		cacheKey = services.LandingPagesCacheKeyByShop(shopID)
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
	}

	db := initializers.DB.
		Where("shop_id = ?", shopID).
		Preload("Images", func(db *gorm.DB) *gorm.DB {
			return db.Order("order_index ASC")
		}).
		Preload("Product").
		Order("created_at DESC")

	if search != "" {
		db = db.Where("title ILIKE ?", "%"+search+"%")
	}

	var landingPages []models.LandingPage
	if err := db.Find(&landingPages).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to retrieve landing pages",
			"error":   err.Error(),
		})
		return
	}

	productIDs := make([]uuid.UUID, len(landingPages))
	for i, lp := range landingPages {
		productIDs[i] = lp.ProductID
	}
	orderCounts, err := countOrdersByProductIDs(productIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to count orders per landing page",
			"error":   err.Error(),
		})
		return
	}

	landingPageIDs := make([]uuid.UUID, len(landingPages))
	for i, lp := range landingPages {
		landingPageIDs[i] = lp.ID
	}
	views, err := viewsByEntityIDs("landing_page", landingPageIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to count views per landing page",
			"error":   err.Error(),
		})
		return
	}
	// Numerator is orders attributed to this specific landing page (orders.landing_page_id),
	// not orders.Orders (all orders for the underlying product) — a product can have several
	// landing pages, so conversion rate must stay scoped to the page that drove the sale.
	landingPageOrders, err := countOrdersByLandingPageIDs(landingPageIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to count attributed orders per landing page",
			"error":   err.Error(),
		})
		return
	}
	for i := range landingPages {
		landingPages[i].Orders = orderCounts[landingPages[i].ProductID]
		landingPages[i].Views = views[landingPages[i].ID]
		landingPages[i].ConversionRate = conversionRate(landingPageOrders[landingPages[i].ID], landingPages[i].Views)
	}

	if search == "" {
		if jsonData, err := json.Marshal(landingPages); err == nil {
			_ = initializers.RClient.Set(initializers.Ctx, cacheKey, jsonData, 10*time.Minute).Err()
		}
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

	cacheKey := services.LandingPageCacheKeyByID(landingPageID)
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

	cacheKey := services.LandingPageCacheKeyByID(landingPageID)
	val, err := initializers.RClient.Get(initializers.Ctx, cacheKey).Result()
	if err == nil {
		var cachedResponse dto.PublicLandingPageResponse
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
		Preload("Shop").
		Preload("Shop.LogoImage").
		Preload("Images", func(db *gorm.DB) *gorm.DB {
			return db.Order("order_index ASC")
		}).
		Preload("Product").
		Preload("Product.Images", func(db *gorm.DB) *gorm.DB {
			return db.Order("order_index ASC")
		}).
		Preload("Product.Variants").
		Preload("Product.Variants.VariantItems").
		Preload("Product.Combinations", "retired = ?", false).
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

	// A super-admin force-hidden product (see Product.HiddenByPlatformAt)
	// must disappear from every customer-facing view, not just the direct
	// product-listing/search/detail endpoints — a landing page embeds its
	// product regardless of that flag, so it needs its own check here.
	// Reported the same as a missing landing page: the moderation state
	// itself shouldn't leak to the public. Runs against the fetched model,
	// independent of the DTO projection below.
	if landingPage.Product.HiddenByPlatformAt != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Landing page not found",
		})
		return
	}

	response := toPublicLandingPageResponse(landingPage)

	if jsonData, err := json.Marshal(response); err == nil {
		_ = initializers.RClient.Set(initializers.Ctx, cacheKey, jsonData, 10*time.Minute).Err()
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Landing page retrieved successfully",
		"data":    response,
	})
}

// toPublicLandingPageResponse projects a models.LandingPage down to the
// public DTO for IndexLandingPage — see dto.PublicLandingPageResponse for
// exactly what's dropped. Only used on that one public unauthenticated path;
// GetLandingPageByShop/GetLandingPagesByShop (merchant-dashboard, authenticated)
// keep returning the full model.
func toPublicLandingPageResponse(lp models.LandingPage) dto.PublicLandingPageResponse {
	shop := dto.PublicLandingPageShop{
		ID:   lp.Shop.ID.String(),
		Slug: lp.Shop.Slug,
		Name: lp.Shop.Name,
	}
	if lp.Shop.LogoImage != nil {
		shop.LogoImage = &dto.ProductImageResponse{ID: lp.Shop.LogoImage.ID.String(), URL: lp.Shop.LogoImage.URL}
	}

	images := make([]dto.ProductImageResponse, 0, len(lp.Images))
	for _, img := range lp.Images {
		images = append(images, dto.ProductImageResponse{ID: img.ID.String(), URL: img.URL})
	}

	productImages := make([]dto.ProductImageResponse, 0, len(lp.Product.Images))
	for _, img := range lp.Product.Images {
		productImages = append(productImages, dto.ProductImageResponse{ID: img.ID.String(), URL: img.URL})
	}

	variants := make([]dto.PublicLandingPageVariant, 0, len(lp.Product.Variants))
	for _, v := range lp.Product.Variants {
		items := make([]dto.PublicLandingPageVariantItem, 0, len(v.VariantItems))
		for _, item := range v.VariantItems {
			items = append(items, dto.PublicLandingPageVariantItem{
				ID:       item.ID.String(),
				Value:    item.Value,
				ImageURL: item.ImageURL,
			})
		}
		variants = append(variants, dto.PublicLandingPageVariant{
			ID:           v.ID.String(),
			Title:        v.Title,
			VariantItems: items,
		})
	}

	toVariantItemPtr := func(item *models.VariantItem) *dto.PublicLandingPageVariantItem {
		if item == nil {
			return nil
		}
		return &dto.PublicLandingPageVariantItem{
			ID:       item.ID.String(),
			Value:    item.Value,
			ImageURL: item.ImageURL,
		}
	}

	combinations := make([]dto.PublicLandingPageCombination, 0, len(lp.Product.Combinations))
	for _, combo := range lp.Product.Combinations {
		combinations = append(combinations, dto.PublicLandingPageCombination{
			ID:                combo.ID.String(),
			ProductID:         combo.ProductID.String(),
			Price:             combo.Price,
			Quantity:          combo.Quantity,
			CombinationString: combo.CombinationString,
			Option1:           toVariantItemPtr(combo.Option1),
			Option2:           toVariantItemPtr(combo.Option2),
			Option3:           toVariantItemPtr(combo.Option3),
		})
	}

	product := dto.PublicLandingPageProduct{
		ID:           lp.Product.ID.String(),
		Title:        lp.Product.Title,
		Description:  lp.Product.Description,
		Price:        lp.Product.Price,
		OldPrice:     lp.Product.OldPrice,
		Images:       productImages,
		Variants:     variants,
		Combinations: combinations,
	}

	var experimentID *string
	if lp.ExperimentID != nil {
		s := lp.ExperimentID.String()
		experimentID = &s
	}

	return dto.PublicLandingPageResponse{
		ID:           lp.ID.String(),
		ShopID:       lp.ShopID.String(),
		Shop:         shop,
		ProductID:    lp.ProductID.String(),
		Product:      product,
		Title:        lp.Title,
		Images:       images,
		Active:       lp.Active,
		ExperimentID: experimentID,
	}
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
	productIDValue := strings.TrimSpace(c.PostForm("productId"))

	var updates = map[string]interface{}{}

	if title != "" {
		updates["title"] = title
	}

	if activeValue != "" {
		active := activeValue == "true"
		updates["active"] = active
	}

	if productIDValue != "" {
		productID, err := uuid.Parse(productIDValue)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "Invalid product ID",
				"error":   err.Error(),
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

		updates["product_id"] = productID
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

	// Rewrite order_index for retained existing images to reflect the frontend's ordering
	for i, img := range existingImages {
		if err := tx.Model(&models.LandingPageImage{}).
			Where("id = ? AND landing_page_id = ?", img.ID, landingPageID).
			UpdateColumn("order_index", i).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Failed to reorder existing images",
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

	services.InvalidateLandingPageCaches(shopID, landingPageID)

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

	services.InvalidateLandingPageCaches(shopID, landingPageID)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Landing page deleted successfully",
	})
}
