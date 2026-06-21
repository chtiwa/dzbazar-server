package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/chtiwa/dzbazar-server/dto"
	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type VariantItemSimple struct {
	ID    string `json:"id"`
	Value string `json:"value"`
	// Price and Quantity removed from here!
}

func CreateProductByShop(c *gin.Context) {
	shopIDParam := c.Param("shopId")
	shopID, err := uuid.Parse(shopIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid shop ID"})
		return
	}

	title := strings.TrimSpace(c.PostForm("title"))
	price := strings.TrimSpace(c.PostForm("price"))
	oldPrice := strings.TrimSpace(c.PostForm("oldPrice"))
	description := strings.TrimSpace(c.PostForm("description"))

	if title == "" || price == "" || description == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "missing required fields"})
		return
	}

	parsedPrice, err := strconv.ParseFloat(price, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid price"})
		return
	}

	var parsedOldPrice *float64
	if oldPrice != "" {
		value, err := strconv.ParseFloat(oldPrice, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid old price"})
			return
		}
		parsedOldPrice = &value
	}

	var variants []dto.VariantInput
	variantsJSON := c.PostForm("variants")
	if variantsJSON != "" {
		if err := json.Unmarshal([]byte(variantsJSON), &variants); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid variants JSON"})
			return
		}
	}

	var combinationsInput []dto.CombinationInput
	combinationsJSON := c.PostForm("combinations")
	if combinationsJSON != "" {
		if err := json.Unmarshal([]byte(combinationsJSON), &combinationsInput); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid combinations JSON"})
			return
		}
	}

	tx := initializers.DB.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to start transaction"})
		return
	}
	defer tx.Rollback()

	product := models.Product{
		ShopID:      shopID,
		Title:       title,
		Description: description,
		Price:       parsedPrice,
		OldPrice:    parsedOldPrice,
	}

	if err := tx.Create(&product).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "failed to create product",
			"error":   err.Error(),
		})
		return
	}

	form, err := c.MultipartForm()
	if err != nil && err != http.ErrNotMultipart {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid multipart form"})
		return
	}

	var files []*multipart.FileHeader
	if form != nil {
		files = form.File["images"]
	}

	if len(files) > 5 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "maximum 5 images allowed"})
		return
	}

	var productImages []models.ProductImage
	for _, file := range files {
		src, err := file.Open()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "failed to open file"})
			return
		}

		key := fmt.Sprintf("uploads/%d_%s", time.Now().UnixNano(), filepath.Base(file.Filename))
		bucketName := os.Getenv("B2_BUCKET_NAME")

		_, err = initializers.S3Client.PutObject(context.TODO(), &s3.PutObjectInput{
			Bucket:      aws.String(bucketName),
			Key:         aws.String(key),
			Body:        src,
			ContentType: aws.String(file.Header.Get("Content-Type")),
		})
		src.Close()

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to upload image"})
			return
		}

		url := fmt.Sprintf(
			"https://%s.s3.%s.backblazeb2.com/%s",
			bucketName,
			os.Getenv("B2_REGION"),
			key,
		)

		productImages = append(productImages, models.ProductImage{
			ProductID: product.ID,
			URL:       url,
		})
	}

	if len(productImages) > 0 {
		if err := tx.Create(&productImages).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to save images"})
			return
		}
	}
	product.Images = productImages

	itemIDs := make(map[string]uuid.UUID)

	makeItemKey := func(variantTitle, itemValue string) string {
		return strings.ToLower(strings.TrimSpace(variantTitle)) + "::" + strings.ToLower(strings.TrimSpace(itemValue))
	}

	for _, v := range variants {
		variant := models.Variant{
			ProductID: product.ID,
			Title:     strings.TrimSpace(v.Title),
		}

		if err := tx.Create(&variant).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to create variant category"})
			return
		}

		for _, item := range v.VariantItems {
			value := strings.TrimSpace(item.Value)
			if value == "" {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "variant item value cannot be empty"})
				return
			}

			vItem := models.VariantItem{
				VariantID: variant.ID,
				Value:     value,
			}

			if err := tx.Create(&vItem).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to create variant item"})
				return
			}

			itemIDs[makeItemKey(v.Title, value)] = vItem.ID
		}
	}

	var variantTitles []string
	for _, v := range variants {
		variantTitles = append(variantTitles, strings.TrimSpace(v.Title))
	}

	var combinationsToSave []models.ProductVariantCombination

	for _, combo := range combinationsInput {
		var opt1ID, opt2ID, opt3ID *uuid.UUID
		var comboStringParts []string

		sku := strings.TrimSpace(combo.SKU)
		if sku == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "combination SKU cannot be empty",
			})
			return
		}

		if combo.Option1 != nil && strings.TrimSpace(*combo.Option1) != "" {
			if len(variantTitles) < 1 {
				c.JSON(http.StatusBadRequest, gin.H{
					"success": false,
					"message": "invalid option1 mapping",
				})
				return
			}

			value := strings.TrimSpace(*combo.Option1)
			id, exists := itemIDs[makeItemKey(variantTitles[0], value)]
			if !exists {
				c.JSON(http.StatusBadRequest, gin.H{
					"success": false,
					"message": fmt.Sprintf("invalid option1 value: %s", value),
				})
				return
			}
			opt1ID = &id
			comboStringParts = append(comboStringParts, value)
		}

		if combo.Option2 != nil && strings.TrimSpace(*combo.Option2) != "" {
			if len(variantTitles) < 2 {
				c.JSON(http.StatusBadRequest, gin.H{
					"success": false,
					"message": "invalid option2 mapping",
				})
				return
			}

			value := strings.TrimSpace(*combo.Option2)
			id, exists := itemIDs[makeItemKey(variantTitles[1], value)]
			if !exists {
				c.JSON(http.StatusBadRequest, gin.H{
					"success": false,
					"message": fmt.Sprintf("invalid option2 value: %s", value),
				})
				return
			}
			opt2ID = &id
			comboStringParts = append(comboStringParts, value)
		}

		if combo.Option3 != nil && strings.TrimSpace(*combo.Option3) != "" {
			if len(variantTitles) < 3 {
				c.JSON(http.StatusBadRequest, gin.H{
					"success": false,
					"message": "invalid option3 mapping",
				})
				return
			}

			value := strings.TrimSpace(*combo.Option3)
			id, exists := itemIDs[makeItemKey(variantTitles[2], value)]
			if !exists {
				c.JSON(http.StatusBadRequest, gin.H{
					"success": false,
					"message": fmt.Sprintf("invalid option3 value: %s", value),
				})
				return
			}
			opt3ID = &id
			comboStringParts = append(comboStringParts, value)
		}

		comboString := strings.Join(comboStringParts, " / ")

		combinationsToSave = append(combinationsToSave, models.ProductVariantCombination{
			ProductID:         product.ID,
			SKU:               sku,
			Price:             combo.Price,
			Quantity:          combo.Quantity,
			Option1ID:         opt1ID,
			Option2ID:         opt2ID,
			Option3ID:         opt3ID,
			CombinationString: comboString,
		})
	}

	if len(combinationsToSave) > 0 {
		if err := tx.Create(&combinationsToSave).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "failed to save product combinations",
				"error":   err.Error(),
			})
			return
		}
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "transaction failed"})
		return
	}

	go func() {
		var cursor uint64
		for {
			keys, nextCursor, err := initializers.RClient.Scan(context.Background(), cursor, "products:*", 100).Result()
			if err == nil && len(keys) > 0 {
				initializers.RClient.Del(context.Background(), keys...)
			}
			cursor = nextCursor
			if cursor == 0 {
				break
			}
		}
	}()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Product created successfully",
		"data":    product,
	})
}

// get products in the admin panel
func GetProductsByShopAdmin(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid shop ID",
		})
		return
	}

	tag := strings.TrimSpace(c.Query("tag"))
	search := strings.TrimSpace(c.Query("search"))
	page := 1
	perPage := 10

	if v := c.Query("page"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			page = parsed
		}
	}
	if v := c.Query("perPage"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 && parsed <= 100 {
			perPage = parsed
		}
	}

	cacheKey := fmt.Sprintf(
		"products:shop:%s:admin:tag=%s:search=%s:page=%d:perPage=%d",
		shopID.String(),
		strings.ToLower(tag),
		strings.ToLower(search),
		page,
		perPage,
	)

	val, err := initializers.RClient.Get(initializers.Ctx, cacheKey).Result()
	if err == nil {
		var cachedResponse map[string]interface{}
		if unmarshalErr := json.Unmarshal([]byte(val), &cachedResponse); unmarshalErr == nil {
			c.JSON(http.StatusOK, cachedResponse)
			return
		}
	}

	var totalRows int64
	var products []models.Product

	db := initializers.DB.Model(&models.Product{}).
		Where("products.shop_id = ?", shopID).
		Preload("Images").
		Preload("Variants").
		Preload("Variants.VariantItems").
		Preload("Combinations")

	if search != "" {
		words := strings.Fields(search)
		for _, w := range words {
			like := "%" + w + "%"
			db = db.Where("(products.title ILIKE ? OR products.description ILIKE ?)", like, like)
		}
	}

	if tag != "" {
		db = db.Joins("JOIN product_tags ON product_tags.product_id = products.id").
			Joins("JOIN tags ON tags.id = product_tags.tag_id").
			Where("LOWER(tags.name) = ?", strings.ToLower(tag))
	}

	if err := db.Count(&totalRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "error while counting products",
			"error":   err.Error(),
		})
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

	if err := db.Order("products.updated_at DESC").
		Limit(perPage).
		Offset(offset).
		Find(&products).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "error while retrieving products",
			"error":   err.Error(),
		})
		return
	}

	response := gin.H{
		"success": true,
		"data":    products,
		"pagination": gin.H{
			"page":       page,
			"perPage":    perPage,
			"totalRows":  totalRows,
			"totalPages": totalPages,
			"hasNext":    page < totalPages,
			"hasPrev":    page > 1,
		},
	}

	if jsonData, err := json.Marshal(response); err == nil {
		initializers.RClient.Set(initializers.Ctx, cacheKey, jsonData, 10*time.Minute)
		initializers.RClient.SAdd(initializers.Ctx, "cache:products", cacheKey)
	}

	c.JSON(http.StatusOK, response)
}

func GetProductByIDAdmin(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	productID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid product ID"})
		return
	}

	var product models.Product
	err = initializers.DB.
		Where("id = ? AND shop_id = ?", productID, shopID).
		Preload("Images").
		Preload("Variants").
		Preload("Variants.VariantItems").
		Preload("Combinations").
		Preload("Combinations.Option1").
		Preload("Combinations.Option2").
		Preload("Combinations.Option3").
		First(&product).Error

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Product not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Product retrieved successfully",
		"data":    product,
	})
}

func GetActiveProductsBySlug(c *gin.Context) {
	slug := strings.ToLower(strings.TrimSpace(c.Param("slug")))
	pageString := c.Query("page")

	page := 1
	if pageString != "" {
		if parsedPage, err := strconv.Atoi(pageString); err == nil && parsedPage > 0 {
			page = parsedPage
		}
	}

	cacheKey := fmt.Sprintf("products:slug:%s:client:page:%d", slug, page)
	val, err := initializers.RClient.Get(initializers.Ctx, cacheKey).Result()
	if err == nil {
		var cachedResponse map[string]interface{}
		if unmarshalErr := json.Unmarshal([]byte(val), &cachedResponse); unmarshalErr == nil {
			c.JSON(http.StatusOK, cachedResponse)
			return
		}
	}

	var shop models.Shop
	if err := initializers.DB.
		Select("id", "slug", "is_active").
		Where("slug = ?", slug).
		First(&shop).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Shop not found",
		})
		return
	}

	if !shop.IsActive {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Shop not found",
		})
		return
	}

	var totalRows int64
	var products []models.Product

	db := initializers.DB.Model(&models.Product{}).
		Where("products.shop_id = ?", shop.ID).
		Where("products.active = ?", true).
		Preload("Images")

	if err := db.Count(&totalRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "error while counting the products",
			"error":   err.Error(),
		})
		return
	}

	perPage := 8
	totalPages := int(math.Ceil(float64(totalRows) / float64(perPage)))
	if totalPages == 0 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}

	offset := (page - 1) * perPage

	if err := db.
		Order("products.updated_at DESC").
		Limit(perPage).
		Offset(offset).
		Find(&products).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "error while retrieving the products",
			"error":   err.Error(),
		})
		return
	}

	response := gin.H{
		"success": true,
		"data":    products,
		"pagination": gin.H{
			"page":       page,
			"totalPages": totalPages,
			"totalRows":  totalRows,
			"hasNext":    page < totalPages,
			"hasPrev":    page > 1,
		},
	}

	if jsonData, err := json.Marshal(response); err == nil {
		initializers.RClient.Set(initializers.Ctx, cacheKey, jsonData, 10*time.Minute)
		initializers.RClient.SAdd(initializers.Ctx, "cache:products", cacheKey)
	}

	c.JSON(http.StatusOK, response)
}

func GetProductsBySearchBySlug(c *gin.Context) {
	search := c.Query("search")
	slug := c.Param("slug")

	// 1. RESOLVE SLUG TO UUID (Fixes the Slug mismatch bug)
	var shop models.Shop
	if err := initializers.DB.Select("id").Where("slug = ?", slug).First(&shop).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Shop not found",
		})
		return
	}
	shopID := shop.ID

	var products []models.Product

	// 2. THE BASE SECURITY QUERY (Fixes the active=true trap)
	query := initializers.DB.Model(&models.Product{}).
		Where("shop_id = ?", shopID).
		Where("active = ?", true).
		Order("updated_at DESC")

	// 3. THE SEARCH LOGIC (Fixes the Phantom Brand bug)
	if search != "" {
		words := strings.Fields(search)
		for _, w := range words {
			like := "%" + w + "%"
			query = query.Where("title ILIKE ? OR description ILIKE ?", like, like)
		}
	}

	// 4. EXECUTE & PRELOAD
	result := query.
		Limit(10).
		Preload("Images").
		Find(&products)

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Error retrieving the products",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    products,
	})
}

func IndexProductBySlug(c *gin.Context) {
	productID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid product ID"})
		return
	}

	slug := c.Param("slug")

	var shop models.Shop
	if err := initializers.DB.Select("id").Where("slug = ?", slug).First(&shop).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "shop not found"})
		return
	}

	cacheKey := fmt.Sprintf("product:shop:%s:id=%s", shop.ID.String(), productID.String())

	val, err := initializers.RClient.Get(initializers.Ctx, cacheKey).Result()
	if err == nil {
		var cachedResponse dto.ProductResponse
		if unmarshalErr := json.Unmarshal([]byte(val), &cachedResponse); unmarshalErr == nil {
			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"message": "product was retrieved successfully",
				"data":    cachedResponse,
			})
			return
		}
	}

	var product models.Product
	err = initializers.DB.
		Where("id = ? AND shop_id = ? AND active = ?", productID, shop.ID, true).
		Preload("Images").
		Preload("Variants").
		Preload("Variants.VariantItems").
		Preload("Combinations").
		First(&product).Error

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "product not found or inactive"})
		return
	}

	itemValueByID := make(map[uuid.UUID]string)
	for _, v := range product.Variants {
		for _, item := range v.VariantItems {
			itemValueByID[item.ID] = item.Value
		}
	}

	response := dto.ProductResponse{
		ID:           product.ID.String(),
		Title:        product.Title,
		Description:  product.Description,
		Price:        product.Price,
		OldPrice:     *product.OldPrice,
		Images:       []dto.ProductImageResponse{},
		Variants:     []dto.VariantResponse{},
		Combinations: []dto.CombinationResponse{},
	}

	for _, i := range product.Images {
		response.Images = append(response.Images, dto.ProductImageResponse{
			ID:  i.ID.String(),
			URL: i.URL,
		})
	}

	for _, v := range product.Variants {
		variantResponse := dto.VariantResponse{
			ID:           v.ID.String(),
			Title:        v.Title,
			VariantItems: []dto.VariantItemSimple{},
		}

		for _, item := range v.VariantItems {
			variantResponse.VariantItems = append(variantResponse.VariantItems, dto.VariantItemSimple{
				ID:    item.ID.String(),
				Value: item.Value,
			})
		}

		response.Variants = append(response.Variants, variantResponse)
	}

	for _, combo := range product.Combinations {
		var opt1ID, opt2ID, opt3ID *string
		var opt1Value, opt2Value, opt3Value *string

		if combo.Option1ID != nil {
			s := combo.Option1ID.String()
			opt1ID = &s
		}
		if combo.Option2ID != nil {
			s := combo.Option2ID.String()
			opt2ID = &s
		}
		if combo.Option3ID != nil {
			s := combo.Option3ID.String()
			opt3ID = &s
		}

		if combo.Option1 != nil {
			s := combo.Option1.Value
			opt1Value = &s
		}
		if combo.Option2 != nil {
			s := combo.Option2.Value
			opt2Value = &s
		}
		if combo.Option3 != nil {
			s := combo.Option3.Value
			opt3Value = &s
		}

		response.Combinations = append(response.Combinations, dto.CombinationResponse{
			ID:                combo.ID.String(),
			SKU:               combo.SKU,
			Price:             combo.Price,
			Quantity:          combo.Quantity,
			Option1ID:         opt1ID,
			Option2ID:         opt2ID,
			Option3ID:         opt3ID,
			Option1Value:      opt1Value,
			Option2Value:      opt2Value,
			Option3Value:      opt3Value,
			CombinationString: combo.CombinationString,
		})
	}

	if jsonData, err := json.Marshal(response); err == nil {
		_ = initializers.RClient.Set(initializers.Ctx, cacheKey, jsonData, 10*time.Minute).Err()
		_ = initializers.RClient.SAdd(initializers.Ctx, "cache:product:id", cacheKey).Err()
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "product was retrieved successfully",
		"data":    response,
	})
}

func UpdateProductByShop(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid shop ID",
			"error":   err.Error(),
		})
		return
	}

	productID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid product ID",
			"error":   err.Error(),
		})
		return
	}

	type VariantItemInput struct {
		Value string `json:"value"`
	}

	type VariantInput struct {
		Title        string             `json:"title"`
		VariantItems []VariantItemInput `json:"variantItems"`
	}

	type CombinationInput struct {
		SKU      string  `json:"sku"`
		Price    float64 `json:"price"`
		Quantity int     `json:"quantity"`
		Option1  *string `json:"option1"`
		Option2  *string `json:"option2"`
		Option3  *string `json:"option3"`
	}

	type UpdateProductBody struct {
		Title        *string            `json:"title"`
		Description  *string            `json:"description"`
		Price        *float64           `json:"price"`
		OldPrice     *float64           `json:"oldPrice"`
		Active       *bool              `json:"active"`
		Variants     []VariantInput     `json:"variants"`
		Combinations []CombinationInput `json:"combinations"`
	}

	var body UpdateProductBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request body",
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

	updates := map[string]interface{}{}

	if body.Title != nil {
		title := strings.TrimSpace(*body.Title)
		if title == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "Title cannot be empty",
			})
			return
		}
		updates["title"] = title
	}

	if body.Description != nil {
		description := strings.TrimSpace(*body.Description)
		if description == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "Description cannot be empty",
			})
			return
		}
		updates["description"] = description
	}

	if body.Price != nil {
		if *body.Price < 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "Price cannot be negative",
			})
			return
		}
		updates["price"] = *body.Price
	}

	if body.OldPrice != nil {
		if *body.OldPrice < 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "Old price cannot be negative",
			})
			return
		}
		updates["old_price"] = *body.OldPrice
	}

	if body.Active != nil {
		updates["active"] = *body.Active
	}

	if len(body.Variants) > 3 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "A product can have at most 3 variants",
		})
		return
	}

	for _, v := range body.Variants {
		title := strings.TrimSpace(v.Title)
		if title == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "Variant title cannot be empty",
			})
			return
		}

		if len(v.VariantItems) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "Each variant must contain at least one item",
			})
			return
		}

		itemSet := make(map[string]struct{})
		for _, item := range v.VariantItems {
			value := strings.TrimSpace(item.Value)
			if value == "" {
				c.JSON(http.StatusBadRequest, gin.H{
					"success": false,
					"message": "Variant item value cannot be empty",
				})
				return
			}

			normalized := strings.ToLower(value)
			if _, exists := itemSet[normalized]; exists {
				c.JSON(http.StatusBadRequest, gin.H{
					"success": false,
					"message": fmt.Sprintf("Duplicate item value '%s' inside variant '%s'", value, title),
				})
				return
			}
			itemSet[normalized] = struct{}{}
		}
	}

	if len(body.Variants) == 0 && len(body.Combinations) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Combinations cannot be provided without variants",
		})
		return
	}

	skuSet := make(map[string]struct{})
	for _, combo := range body.Combinations {
		if strings.TrimSpace(combo.SKU) == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "Combination SKU cannot be empty",
			})
			return
		}
		if combo.Price < 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "Combination price cannot be negative",
			})
			return
		}
		if combo.Quantity < 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "Combination quantity cannot be negative",
			})
			return
		}

		normalizedSKU := strings.ToLower(strings.TrimSpace(combo.SKU))
		if _, exists := skuSet[normalizedSKU]; exists {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": fmt.Sprintf("Duplicate SKU '%s' in request", combo.SKU),
			})
			return
		}
		skuSet[normalizedSKU] = struct{}{}
	}

	if len(updates) == 0 && body.Variants == nil && body.Combinations == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "No fields provided for update",
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
		if err := tx.Model(&product).Updates(updates).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Failed to update product",
				"error":   err.Error(),
			})
			return
		}
	}

	if body.Variants != nil || body.Combinations != nil {
		// Null out option FK columns on combinations before deleting variant_items,
		// otherwise the FK constraint (option1_id → variant_items) blocks the delete.
		if err := tx.Model(&models.ProductVariantCombination{}).
			Where("product_id = ?", productID).
			Updates(map[string]any{
				"option1_id": nil,
				"option2_id": nil,
				"option3_id": nil,
			}).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Failed to detach variant items from combinations",
				"error":   err.Error(),
			})
			return
		}

		// Now safe to delete variants (cascades to variant_items)
		if err := tx.Where("product_id = ?", productID).Delete(&models.Variant{}).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Failed to clear existing variants",
				"error":   err.Error(),
			})
			return
		}

		itemIDs := make(map[string]uuid.UUID)

		for _, v := range body.Variants {
			variant := models.Variant{
				ProductID: productID,
				Title:     strings.TrimSpace(v.Title),
			}

			if err := tx.Create(&variant).Error; err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{
					"success": false,
					"message": "Failed to create variant",
					"error":   err.Error(),
				})
				return
			}

			for _, item := range v.VariantItems {
				value := strings.TrimSpace(item.Value)

				variantItem := models.VariantItem{
					VariantID: variant.ID,
					Value:     value,
				}

				if err := tx.Create(&variantItem).Error; err != nil {
					tx.Rollback()
					c.JSON(http.StatusInternalServerError, gin.H{
						"success": false,
						"message": "Failed to create variant item",
						"error":   err.Error(),
					})
					return
				}

				itemIDs[strings.ToLower(value)] = variantItem.ID
			}
		}

		// Build combinations from the new payload
		type resolvedCombo struct {
			sku               string
			price             float64
			quantity          int
			opt1ID            *uuid.UUID
			opt2ID            *uuid.UUID
			opt3ID            *uuid.UUID
			combinationString string
		}

		newSKUs := make(map[string]resolvedCombo)

		for _, combo := range body.Combinations {
			var opt1ID, opt2ID, opt3ID *uuid.UUID
			var comboStringParts []string

			if combo.Option1 != nil && strings.TrimSpace(*combo.Option1) != "" {
				value := strings.ToLower(strings.TrimSpace(*combo.Option1))
				id, exists := itemIDs[value]
				if !exists {
					tx.Rollback()
					c.JSON(http.StatusBadRequest, gin.H{
						"success": false,
						"message": fmt.Sprintf("Option1 value '%s' does not exist", *combo.Option1),
					})
					return
				}
				opt1ID = &id
				comboStringParts = append(comboStringParts, strings.TrimSpace(*combo.Option1))
			}

			if combo.Option2 != nil && strings.TrimSpace(*combo.Option2) != "" {
				value := strings.ToLower(strings.TrimSpace(*combo.Option2))
				id, exists := itemIDs[value]
				if !exists {
					tx.Rollback()
					c.JSON(http.StatusBadRequest, gin.H{
						"success": false,
						"message": fmt.Sprintf("Option2 value '%s' does not exist", *combo.Option2),
					})
					return
				}
				opt2ID = &id
				comboStringParts = append(comboStringParts, strings.TrimSpace(*combo.Option2))
			}

			if combo.Option3 != nil && strings.TrimSpace(*combo.Option3) != "" {
				value := strings.ToLower(strings.TrimSpace(*combo.Option3))
				id, exists := itemIDs[value]
				if !exists {
					tx.Rollback()
					c.JSON(http.StatusBadRequest, gin.H{
						"success": false,
						"message": fmt.Sprintf("Option3 value '%s' does not exist", *combo.Option3),
					})
					return
				}
				opt3ID = &id
				comboStringParts = append(comboStringParts, strings.TrimSpace(*combo.Option3))
			}

			normalizedSKU := strings.ToLower(strings.TrimSpace(combo.SKU))
			newSKUs[normalizedSKU] = resolvedCombo{
				sku:               strings.TrimSpace(combo.SKU),
				price:             combo.Price,
				quantity:          combo.Quantity,
				opt1ID:            opt1ID,
				opt2ID:            opt2ID,
				opt3ID:            opt3ID,
				combinationString: strings.Join(comboStringParts, " / "),
			}
		}

		// Load existing combinations to decide upsert vs retire
		var existingCombos []models.ProductVariantCombination
		if err := tx.Where("product_id = ?", productID).Find(&existingCombos).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Failed to load existing combinations",
				"error":   err.Error(),
			})
			return
		}

		existingBySKU := make(map[string]models.ProductVariantCombination)
		for _, ec := range existingCombos {
			existingBySKU[strings.ToLower(ec.SKU)] = ec
		}

		// Upsert: update existing SKUs, create new ones
		for normalizedSKU, rc := range newSKUs {
			if existing, found := existingBySKU[normalizedSKU]; found {
				if err := tx.Model(&existing).Updates(map[string]any{
					"price":              rc.price,
					"quantity":           rc.quantity,
					"option1_id":         rc.opt1ID,
					"option2_id":         rc.opt2ID,
					"option3_id":         rc.opt3ID,
					"combination_string": rc.combinationString,
				}).Error; err != nil {
					tx.Rollback()
					c.JSON(http.StatusInternalServerError, gin.H{
						"success": false,
						"message": fmt.Sprintf("Failed to update combination with SKU '%s'", rc.sku),
						"error":   err.Error(),
					})
					return
				}
			} else {
				newCombo := models.ProductVariantCombination{
					ProductID:         productID,
					SKU:               rc.sku,
					Price:             rc.price,
					Quantity:          rc.quantity,
					Option1ID:         rc.opt1ID,
					Option2ID:         rc.opt2ID,
					Option3ID:         rc.opt3ID,
					CombinationString: rc.combinationString,
				}
				if err := tx.Create(&newCombo).Error; err != nil {
					tx.Rollback()
					c.JSON(http.StatusInternalServerError, gin.H{
						"success": false,
						"message": fmt.Sprintf("Failed to create combination with SKU '%s'", rc.sku),
						"error":   err.Error(),
					})
					return
				}
			}
		}

		// Remove or retire combinations no longer in the new payload
		for normalizedSKU, existing := range existingBySKU {
			if _, kept := newSKUs[normalizedSKU]; kept {
				continue
			}
			var refCount int64
			tx.Model(&models.OrderItem{}).Where("product_variant_combination_id = ?", existing.ID).Count(&refCount)
			if refCount == 0 {
				if err := tx.Delete(&existing).Error; err != nil {
					tx.Rollback()
					c.JSON(http.StatusInternalServerError, gin.H{
						"success": false,
						"message": fmt.Sprintf("Failed to delete combination with SKU '%s'", existing.SKU),
						"error":   err.Error(),
					})
					return
				}
			} else {
				// Has order references — retire it instead of deleting
				if err := tx.Model(&existing).Update("quantity", 0).Error; err != nil {
					tx.Rollback()
					c.JSON(http.StatusInternalServerError, gin.H{
						"success": false,
						"message": fmt.Sprintf("Failed to retire combination with SKU '%s'", existing.SKU),
						"error":   err.Error(),
					})
					return
				}
			}
		}

	}

	if err := tx.
		Preload("Images").
		Preload("Variants").
		Preload("Variants.VariantItems").
		Preload("Combinations").
		Where("id = ? AND shop_id = ?", productID, shopID).
		First(&product).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to reload updated product",
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

	invalidateProductCaches(productID, shopID)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Product was updated successfully",
		"data":    product,
	})
}

func UpdateProductImagesByShop(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid shop ID",
			"error":   err.Error(),
		})
		return
	}

	productID, err := uuid.Parse(c.Param("id"))
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

	var currentImages []models.ProductImage
	if err := initializers.DB.
		Where("product_id = ?", productID).
		Order("order_index ASC, created_at ASC").
		Find(&currentImages).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to retrieve product images",
			"error":   err.Error(),
		})
		return
	}

	existingImagesJSON := c.PostForm("existingImages")
	var existingImages []dto.UpdateProductsImageInput
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

	currentImageIDs := make(map[string]models.ProductImage, len(currentImages))
	for _, img := range currentImages {
		currentImageIDs[img.ID.String()] = img
	}

	for keptID := range keptImageIDs {
		if _, exists := currentImageIDs[keptID]; !exists {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "One or more existing image IDs do not belong to this product",
			})
			return
		}
	}

	var imagesToDelete []models.ProductImage
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

	if len(existingImages)+len(files) > 5 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "The maximum number of images is 5",
		})
		return
	}

	var uploadedKeys []string
	var newProductImages []models.ProductImage

	tx := initializers.DB.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to start transaction",
		})
		return
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	for _, img := range imagesToDelete {
		if err := tx.Where("id = ? AND product_id = ?", img.ID, productID).Delete(&models.ProductImage{}).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Failed to delete removed images",
				"error":   err.Error(),
			})
			return
		}
	}

	bucketName := os.Getenv("B2_BUCKET_NAME")
	region := os.Getenv("B2_REGION")

	for _, file := range files {
		src, err := file.Open()
		if err != nil {
			tx.Rollback()
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "Failed to open uploaded file",
				"error":   err.Error(),
			})
			return
		}

		key := fmt.Sprintf("uploads/products/%s/%d_%s", shopID.String(), time.Now().UnixNano(), filepath.Base(file.Filename))

		_, err = initializers.S3Client.PutObject(context.Background(), &s3.PutObjectInput{
			Bucket:      aws.String(bucketName),
			Key:         aws.String(key),
			Body:        src,
			ContentType: aws.String(file.Header.Get("Content-Type")),
		})
		src.Close()

		if err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": fmt.Sprintf("Failed to upload %s", file.Filename),
				"error":   err.Error(),
			})
			return
		}

		uploadedKeys = append(uploadedKeys, key)

		url := fmt.Sprintf("https://%s.s3.%s.backblazeb2.com/%s", bucketName, region, key)
		newProductImages = append(newProductImages, models.ProductImage{
			ProductID: productID,
			URL:       url,
		})
	}

	if len(newProductImages) > 0 {
		if err := tx.Create(&newProductImages).Error; err != nil {
			tx.Rollback()

			for _, key := range uploadedKeys {
				_, _ = initializers.S3Client.DeleteObject(context.Background(), &s3.DeleteObjectInput{
					Bucket: aws.String(bucketName),
					Key:    aws.String(key),
				})
			}

			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Failed to save new product images",
				"error":   err.Error(),
			})
			return
		}
	}

	var finalImages []models.ProductImage
	if err := tx.
		Where("product_id = ?", productID).
		Order("order_index ASC, created_at ASC").
		Find(&finalImages).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to reload product images",
			"error":   err.Error(),
		})
		return
	}

	if err := tx.Commit().Error; err != nil {
		for _, key := range uploadedKeys {
			_, _ = initializers.S3Client.DeleteObject(context.Background(), &s3.DeleteObjectInput{
				Bucket: aws.String(bucketName),
				Key:    aws.String(key),
			})
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to commit transaction",
			"error":   err.Error(),
		})
		return
	}

	invalidateProductCaches(productID, shopID)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Product images updated successfully",
		"data": gin.H{
			"productId": productID,
			"images":    finalImages,
		},
	})
}

func DeleteProductByShop(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid shop ID",
			"error":   err.Error(),
		})
		return
	}

	productID, err := uuid.Parse(c.Param("id"))
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

	result := tx.Where("id = ? AND shop_id = ?", productID, shopID).Delete(&models.Product{})
	if result.Error != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to delete product",
			"error":   result.Error.Error(),
		})
		return
	}

	if result.RowsAffected == 0 {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Product not found",
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

	invalidateProductCaches(productID, shopID)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Product deleted successfully",
	})
}

// Variants
func UpdateProductVariantsByShop(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid shop ID",
			"error":   err.Error(),
		})
		return
	}

	productID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid product ID",
			"error":   err.Error(),
		})
		return
	}

	type VariantItemInput struct {
		Value string `json:"value"`
	}

	type VariantInput struct {
		Title string             `json:"title"`
		Items []VariantItemInput `json:"items"`
	}

	type CombinationInput struct {
		SKU          string  `json:"sku"`
		Price        float64 `json:"price"`
		Quantity     int     `json:"quantity"`
		Option1Value *string `json:"option1Value"`
		Option2Value *string `json:"option2Value"`
		Option3Value *string `json:"option3Value"`
	}

	type UpdateProductVariantsBody struct {
		Variants     []VariantInput     `json:"variants"`
		Combinations []CombinationInput `json:"combinations"`
	}

	var body UpdateProductVariantsBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request body",
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

	if len(body.Variants) > 3 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "A product can have at most 3 variants",
		})
		return
	}

	for _, v := range body.Variants {
		if strings.TrimSpace(v.Title) == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "Variant title cannot be empty",
			})
			return
		}

		if len(v.Items) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "Each variant must contain at least one item",
			})
			return
		}

		itemSet := make(map[string]struct{})
		for _, item := range v.Items {
			value := strings.TrimSpace(item.Value)
			if value == "" {
				c.JSON(http.StatusBadRequest, gin.H{
					"success": false,
					"message": "Variant item value cannot be empty",
				})
				return
			}

			normalized := strings.ToLower(value)
			if _, exists := itemSet[normalized]; exists {
				c.JSON(http.StatusBadRequest, gin.H{
					"success": false,
					"message": fmt.Sprintf("Duplicate item value '%s' inside variant '%s'", value, v.Title),
				})
				return
			}
			itemSet[normalized] = struct{}{}
		}
	}

	skuSet := make(map[string]struct{})
	for _, combo := range body.Combinations {
		if strings.TrimSpace(combo.SKU) == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "Combination SKU cannot be empty",
			})
			return
		}
		if combo.Price < 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "Combination price cannot be negative",
			})
			return
		}
		if combo.Quantity < 0 {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "Combination quantity cannot be negative",
			})
			return
		}

		normalizedSKU := strings.ToLower(strings.TrimSpace(combo.SKU))
		if _, exists := skuSet[normalizedSKU]; exists {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": fmt.Sprintf("Duplicate SKU '%s' in request", combo.SKU),
			})
			return
		}
		skuSet[normalizedSKU] = struct{}{}
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

	if err := tx.Where("product_id = ?", productID).Delete(&models.ProductVariantCombination{}).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to clear existing combinations",
			"error":   err.Error(),
		})
		return
	}

	if err := tx.Where("product_id = ?", productID).Delete(&models.Variant{}).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to clear existing variants",
			"error":   err.Error(),
		})
		return
	}

	itemIDs := make(map[string]uuid.UUID)

	for _, v := range body.Variants {
		variant := models.Variant{
			ProductID: productID,
			Title:     strings.TrimSpace(v.Title),
		}

		if err := tx.Create(&variant).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Failed to create variant",
				"error":   err.Error(),
			})
			return
		}

		for _, item := range v.Items {
			value := strings.TrimSpace(item.Value)

			variantItem := models.VariantItem{
				VariantID: variant.ID,
				Value:     value,
			}

			if err := tx.Create(&variantItem).Error; err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{
					"success": false,
					"message": "Failed to create variant item",
					"error":   err.Error(),
				})
				return
			}

			itemIDs[strings.ToLower(value)] = variantItem.ID
		}
	}

	var combinationsToSave []models.ProductVariantCombination

	for _, combo := range body.Combinations {
		var opt1ID, opt2ID, opt3ID *uuid.UUID
		var comboStringParts []string

		if combo.Option1Value != nil && strings.TrimSpace(*combo.Option1Value) != "" {
			value := strings.ToLower(strings.TrimSpace(*combo.Option1Value))
			id, exists := itemIDs[value]
			if !exists {
				tx.Rollback()
				c.JSON(http.StatusBadRequest, gin.H{
					"success": false,
					"message": fmt.Sprintf("Option1 value '%s' does not exist", *combo.Option1Value),
				})
				return
			}
			opt1ID = &id
			comboStringParts = append(comboStringParts, strings.TrimSpace(*combo.Option1Value))
		}

		if combo.Option2Value != nil && strings.TrimSpace(*combo.Option2Value) != "" {
			value := strings.ToLower(strings.TrimSpace(*combo.Option2Value))
			id, exists := itemIDs[value]
			if !exists {
				tx.Rollback()
				c.JSON(http.StatusBadRequest, gin.H{
					"success": false,
					"message": fmt.Sprintf("Option2 value '%s' does not exist", *combo.Option2Value),
				})
				return
			}
			opt2ID = &id
			comboStringParts = append(comboStringParts, strings.TrimSpace(*combo.Option2Value))
		}

		if combo.Option3Value != nil && strings.TrimSpace(*combo.Option3Value) != "" {
			value := strings.ToLower(strings.TrimSpace(*combo.Option3Value))
			id, exists := itemIDs[value]
			if !exists {
				tx.Rollback()
				c.JSON(http.StatusBadRequest, gin.H{
					"success": false,
					"message": fmt.Sprintf("Option3 value '%s' does not exist", *combo.Option3Value),
				})
				return
			}
			opt3ID = &id
			comboStringParts = append(comboStringParts, strings.TrimSpace(*combo.Option3Value))
		}

		combinationsToSave = append(combinationsToSave, models.ProductVariantCombination{
			ProductID:         productID,
			SKU:               strings.TrimSpace(combo.SKU),
			Price:             combo.Price,
			Quantity:          combo.Quantity,
			Option1ID:         opt1ID,
			Option2ID:         opt2ID,
			Option3ID:         opt3ID,
			CombinationString: strings.Join(comboStringParts, " / "),
		})
	}

	if len(combinationsToSave) > 0 {
		if err := tx.Create(&combinationsToSave).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Failed to save combinations",
				"error":   err.Error(),
			})
			return
		}
	}

	var updatedProduct models.Product
	if err := tx.
		Preload("Variants").
		Preload("Variants.VariantItems").
		Preload("Combinations").
		Where("id = ? AND shop_id = ?", productID, shopID).
		First(&updatedProduct).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to reload updated product",
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

	invalidateProductCaches(productID, shopID)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Product variants updated successfully",
		"data":    updatedProduct,
	})
}

func invalidateProductCaches(productID uuid.UUID, shopID uuid.UUID) {
	go func() {
		ctx := context.Background()

		productKeys, _ := initializers.RClient.SMembers(ctx, "cache:product:id").Result()
		for _, key := range productKeys {
			if strings.Contains(key, productID.String()) || strings.Contains(key, shopID.String()) {
				initializers.RClient.Del(ctx, key)
			}
		}

		productListKeys, _ := initializers.RClient.SMembers(ctx, "cache:products").Result()
		for _, key := range productListKeys {
			if strings.Contains(key, shopID.String()) {
				initializers.RClient.Del(ctx, key)
			}
		}
	}()
}
