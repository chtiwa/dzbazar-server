package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
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
	"github.com/chtiwa/dzbazar-server/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type VariantItemSimple struct {
	ID    string `json:"id"`
	Value string `json:"value"`
	// Price and Quantity removed from here!
}

func CreateProductByShop(c *gin.Context) {
	// 1. CRITICAL FIX: Extract the ShopID.
	// Assuming your route is something like POST /shops/:shopId/products
	shopIDParam := c.Param("shopId")
	shopID, err := uuid.Parse(shopIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid Shop ID"})
		return
	}

	title := c.PostForm("title")
	price := c.PostForm("price")
	description := c.PostForm("description")

	if title == "" || price == "" || description == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Missing required fields"})
		return
	}

	parsedPrice, err := strconv.ParseFloat(price, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid price"})
		return
	}

	var variants []dto.VariantInput
	variantsJson := c.PostForm("variants")
	if variantsJson != "" {
		if err := json.Unmarshal([]byte(variantsJson), &variants); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid variants JSON"})
			return
		}
	}

	// Transaction start
	tx := initializers.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "internal server error"})
		}
	}()

	var tagNames []string
	var tags []models.Tag

	// --- TAGS LOGIC (Kept exactly as you wrote it, it is excellent) ---
	tagsJson := c.PostForm("tags")
	if tagsJson != "" {
		if err := json.Unmarshal([]byte(tagsJson), &tagNames); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid tags JSON"})
			return
		}

		uniqueNames := make(map[string]struct{})
		normalizedNames := []string{}
		for _, tagName := range tagNames {
			tagName = strings.ToLower(strings.TrimSpace(tagName))
			if tagName == "" {
				continue
			}
			if _, exists := uniqueNames[tagName]; !exists {
				uniqueNames[tagName] = struct{}{}
				normalizedNames = append(normalizedNames, tagName)
			}
		}

		if len(normalizedNames) > 0 {
			var existingTags []models.Tag
			if err := tx.Where("name IN ?", normalizedNames).Find(&existingTags).Error; err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to fetch tags"})
				return
			}

			existingMap := make(map[string]models.Tag)
			for _, tag := range existingTags {
				existingMap[tag.Name] = tag
			}

			var tagsToCreate []models.Tag
			for _, tagName := range normalizedNames {
				if _, exists := existingMap[tagName]; !exists {
					tagsToCreate = append(tagsToCreate, models.Tag{Name: tagName})
				}
			}

			if len(tagsToCreate) > 0 {
				if err := tx.Create(&tagsToCreate).Error; err != nil {
					tx.Rollback()
					c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to create tags"})
					return
				}
			}
			tags = append(existingTags, tagsToCreate...)
		}
	}

	// --- PRODUCT CREATION ---
	product := models.Product{
		ShopID:      shopID, // 2. CRITICAL FIX: Link to the shop
		Title:       title,
		Description: description,
		Price:       parsedPrice,
		Tags:        tags,
	}

	if err := tx.Create(&product).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to create product", "error": err.Error()})
		return
	}

	// --- IMAGE UPLOAD ---
	form, _ := c.MultipartForm()
	files := form.File["images"]

	var productImages []models.ProductImage
	for _, file := range files {
		src, err := file.Open()
		if err != nil {
			tx.Rollback()
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
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to upload image"})
			return
		}

		url := fmt.Sprintf("https://%s.s3.%s.backblazeb2.com/%s", bucketName, os.Getenv("B2_REGION"), key)
		productImages = append(productImages, models.ProductImage{
			ProductID: product.ID,
			URL:       url,
		})
	}

	if len(productImages) > 0 {
		if err := tx.Create(&productImages).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to save images"})
			return
		}
	}

	product.Images = productImages

	// --- VARIANTS & COMBINATIONS ---

	// 1. A map to remember the new UUIDs for each VariantItem string
	// e.g., "Red" -> "123e4567-e89b-12d3..."
	itemIDs := make(map[string]uuid.UUID)

	for _, v := range variants {
		variant := models.Variant{ProductID: product.ID, Title: v.Title}
		if err := tx.Create(&variant).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to create variant category"})
			return
		}

		for _, item := range v.VariantItems {
			vItem := models.VariantItem{
				VariantID: variant.ID,
				Value:     item.Value,
			}

			if err := tx.Create(&vItem).Error; err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to create variant item"})
				return
			}

			// Save the newly generated UUID into our map for later!
			itemIDs[item.Value] = vItem.ID
		}
	}

	// 2. Parse the Combinations from the form data
	var combinationsInput []dto.CombinationInput
	combinationsJson := c.PostForm("combinations")
	if combinationsJson != "" {
		if err := json.Unmarshal([]byte(combinationsJson), &combinationsInput); err != nil {
			tx.Rollback()
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid combinations JSON"})
			return
		}
	}

	// 3. Create the actual sellable SKUs (Combinations)
	var combinationsToSave []models.ProductVariantCombination

	for _, combo := range combinationsInput {
		var opt1ID, opt2ID, opt3ID *uuid.UUID
		var comboStringParts []string

		// Map Option 1
		if combo.Option1 != nil && *combo.Option1 != "" {
			if id, exists := itemIDs[*combo.Option1]; exists {
				opt1ID = &id
				comboStringParts = append(comboStringParts, *combo.Option1)
			}
		}
		// Map Option 2
		if combo.Option2 != nil && *combo.Option2 != "" {
			if id, exists := itemIDs[*combo.Option2]; exists {
				opt2ID = &id
				comboStringParts = append(comboStringParts, *combo.Option2)
			}
		}
		// Map Option 3
		if combo.Option3 != nil && *combo.Option3 != "" {
			if id, exists := itemIDs[*combo.Option3]; exists {
				opt3ID = &id
				comboStringParts = append(comboStringParts, *combo.Option3)
			}
		}

		// Create a clean readable string like "Red / 41"
		comboString := strings.Join(comboStringParts, " / ")

		combinationsToSave = append(combinationsToSave, models.ProductVariantCombination{
			ProductID:         product.ID,
			SKU:               combo.SKU,
			Price:             combo.Price,
			Quantity:          combo.Quantity,
			Option1ID:         opt1ID,
			Option2ID:         opt2ID,
			Option3ID:         opt3ID,
			CombinationString: comboString,
		})
	}

	// 4. Batch Insert all Combinations at once
	if len(combinationsToSave) > 0 {
		if err := tx.Create(&combinationsToSave).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "failed to save product combinations",
				"error":   err.Error(),
			})
			return
		}
	}

	// --- END VARIANTS & COMBINATIONS ---

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "transaction failed"})
		return
	}

	// 3. IMPROVEMENT: Run Redis invalidation in the background
	// This prevents the user from waiting for the cache to clear
	go func() {
		var cursor uint64
		for {
			var keys []string
			var err error
			// Note: ensure you are using a background context here, not gin.Context
			keys, cursor, err = initializers.RClient.Scan(context.Background(), cursor, "products:*", 100).Result()
			if err == nil && len(keys) > 0 {
				initializers.RClient.Del(context.Background(), keys...)
			}
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
func GetProductsByShop(c *gin.Context) {
	// 1. Extract the SLUG from the URL instead of the UUID
	slug := c.Param("slug")

	tag := c.Query("tag")
	pageString := c.Query("page")
	page := 1

	if pageString != "" {
		if parsedPage, err := strconv.Atoi(pageString); err == nil && parsedPage > 0 {
			page = parsedPage
		}
	}

	// 2. RESOLVE SLUG TO UUID (Protects against fake URLs)
	// We only select the "id" column to make this query lightning fast.
	var shopID uuid.UUID
	if err := initializers.DB.Model(&models.Shop{}).Select("id").Where("slug = ?", slug).First(&shopID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Shop not found",
		})
		return
	}
	userInterface, ok := c.Get("user")
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Error while fetching the user from context",
		})
		return
	}

	user, _ := userInterface.(models.User)
	if user.ShopID == &shopID {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Error while fetching the products",
		})
		return
	}

	// 3. CRITICAL FIX: Add the ShopID to the Redis cache key to prevent cross-shop data leaks
	cacheKey := fmt.Sprintf("products:shop:%s:admin:tag=%s:page=%d", shopID.String(), tag, page)

	val, err := initializers.RClient.Get(initializers.Ctx, cacheKey).Result()
	if err == nil {
		var cachedResponse map[string]interface{}
		if unmarshalErr := json.Unmarshal([]byte(val), &cachedResponse); unmarshalErr == nil {
			c.JSON(http.StatusOK, cachedResponse)
			return // crucial: avoid continuing to DB query
		}
		// if unmarshaling failed, fall through to DB fetch
	}

	var totalRows int64
	var products []models.Product

	// 3. Lock the query to this specific shop immediately
	db := initializers.DB.Model(&models.Product{}).
		Where("products.shop_id = ?", shopID). // Use "products.shop_id" to prevent column ambiguity during JOINs
		Preload("Images").
		Preload("Tags")

	if tag != "" {
		// Filter products by tag name
		db = db.Joins("JOIN product_tags ON product_tags.product_id = products.id").
			Joins("JOIN tags ON tags.id = product_tags.tag_id").
			Where("LOWER(tags.name) = ?", strings.ToLower(tag))
	}

	// count total rows after applying the filter if there's any
	if err := db.Count(&totalRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "error while counting the products",
			"error":   err.Error(),
		})
		return
	}

	perPage := 10.0
	totalPages := math.Ceil(float64(totalRows) / perPage)

	offset := (page - 1) * int(perPage)
	if page > int(totalPages) {
		offset = 0 // Optional: Or you can return an empty array if page is out of bounds
	}

	result := db.Order("products.updated_at DESC").Limit(int(perPage)).Offset(offset).Find(&products)

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "error while retrieving the products",
			"error":   result.Error.Error(),
		})
		return
	}

	pagination := utils.GetPaginationData(page, int(totalPages), "/products")

	response := gin.H{
		"success":    true,
		"data":       products,
		"pagination": pagination,
	}

	jsonData, err := json.Marshal(response)
	if err == nil {
		initializers.RClient.Set(initializers.Ctx, cacheKey, jsonData, 10*time.Minute)
		initializers.RClient.SAdd(initializers.Ctx, "cache:products", cacheKey)
	}

	c.JSON(http.StatusOK, response)
}

func GetActiveProductsBySlug(c *gin.Context) {
	// 1. Extract the SLUG from the URL instead of the UUID
	slug := c.Param("slug")

	tag := c.Query("tag")
	pageString := c.Query("page")
	page := 1

	if pageString != "" {
		if parsedPage, err := strconv.Atoi(pageString); err == nil && parsedPage > 0 {
			page = parsedPage
		}
	}

	// 2. Update the cache key to use the slug
	cacheKey := fmt.Sprintf("products:slug:%s:client:tag=%s:page=%d", slug, tag, page)
	val, err := initializers.RClient.Get(initializers.Ctx, cacheKey).Result()
	if err == nil {
		var cachedResponse map[string]interface{}
		if unmarshalErr := json.Unmarshal([]byte(val), &cachedResponse); unmarshalErr == nil {
			c.JSON(http.StatusOK, cachedResponse)
			return
		}
	}

	// 3. RESOLVE SLUG TO UUID (Protects against fake URLs)
	// We only select the "id" column to make this query lightning fast.
	var shopID uuid.UUID
	if err := initializers.DB.Model(&models.Shop{}).Select("id").Where("slug = ?", slug).First(&shopID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Shop not found",
		})
		return
	}

	var totalRows int64
	var products []models.Product

	// 4. Run the product query using the securely resolved shopID
	db := initializers.DB.Model(&models.Product{}).
		Where("products.shop_id = ?", shopID).
		Where("products.active = ?", true).
		Preload("Images").
		Preload("Tags")

	if tag != "" {
		db = db.Joins("JOIN product_tags ON product_tags.product_id = products.id").
			Joins("JOIN tags ON tags.id = product_tags.tag_id").
			Where("LOWER(tags.name) = ?", strings.ToLower(tag))
	}

	if err := db.Count(&totalRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "error while counting the products",
			"error":   err.Error(),
		})
		return
	}

	perPage := 8.0
	totalPages := math.Ceil(float64(totalRows) / perPage)

	offset := (page - 1) * int(perPage)
	if page > int(totalPages) {
		offset = int(totalPages) * int(perPage)
	}

	result := db.Order("products.updated_at DESC").Limit(int(perPage)).Offset(offset).Find(&products)

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "error while retrieving the products",
			"error":   result.Error.Error(),
		})
		return
	}

	pagination := utils.GetPaginationData(page, int(totalPages), "/products")

	response := gin.H{
		"success":    true,
		"data":       products,
		"pagination": pagination,
	}

	jsonData, err := json.Marshal(response)
	if err == nil {
		initializers.RClient.Set(initializers.Ctx, cacheKey, jsonData, 10*time.Minute)
		initializers.RClient.SAdd(initializers.Ctx, "cache:products", cacheKey)
	}

	c.JSON(http.StatusOK, response)
}

func GetProductsBySearchBySlug(c *gin.Context) {
	search := c.Query("search")
	slug := c.Param("slug")

	// 1. RESOLVE SLUG TO UUID (Fixes the Slug mismatch bug)
	var shopID uuid.UUID
	if err := initializers.DB.Model(&models.Shop{}).Select("id").Where("slug = ?", slug).First(&shopID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Shop not found",
		})
		return
	}

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

func IndexProductByShop(c *gin.Context) {
	productId, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "error while parsing the product id",
		})
		return
	}

	// redis key
	cacheKey := fmt.Sprintf("product:id=%s", productId.String())
	val, err := initializers.RClient.Get(initializers.Ctx, cacheKey).Result()
	if err == nil {
		var cachedResponse dto.ProductResponse
		if unmarshalErr := json.Unmarshal([]byte(val), &cachedResponse); unmarshalErr == nil {
			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"message": "product was retrieved successfully (from cache)",
				"data":    cachedResponse,
			})
			return
		}
	}

	var product models.Product

	// CRITICAL ADDITION: .Preload("Combinations")
	// (Ensure your models.Product struct has `Combinations []ProductVariantCombination` defined on it!)
	result := initializers.DB.
		Preload("Images").
		Preload("Tags").
		Preload("Variants").
		Preload("Variants.VariantItems").
		Preload("Combinations").   // The Shopify Trick Inventory!
		Where("active = ?", true). // ONLY return if active (assuming this is for the client storefront)
		First(&product, "id = ?", productId)

	if result.Error != nil {
		c.JSON(http.StatusNotFound, gin.H{ // Changed from 500 to 404
			"success": false,
			"message": "Product not found or inactive",
		})
		return
	}

	response := dto.ProductResponse{
		ID:           product.ID.String(),
		Title:        product.Title,
		Description:  product.Description,
		Price:        product.Price,
		OldPrice:     product.OldPrice,
		Variants:     make([]dto.VariantResponse, 0),
		Combinations: make([]dto.CombinationResponse, 0),
	}

	// Map Images
	var images []dto.ProductImageResponse
	for _, i := range product.Images {
		images = append(images, dto.ProductImageResponse{
			ID:  i.ID.String(),
			URL: i.URL,
		})
	}
	response.Images = images

	// Map Tags
	var tags []string
	for _, t := range product.Tags {
		tags = append(tags, t.Name)
	}
	response.Tags = tags

	// Map Variants & Items
	for _, v := range product.Variants {
		var vr dto.VariantResponse
		vr.ID = v.ID.String()
		vr.Title = v.Title
		vr.VariantItems = make([]dto.VariantItemSimple, 0)

		for _, item := range v.VariantItems {
			vr.VariantItems = append(vr.VariantItems, dto.VariantItemSimple{
				ID:    item.ID.String(),
				Value: item.Value,
			})
		}
		response.Variants = append(response.Variants, vr)
	}

	// CRITICAL ADDITION: Map the Sellable Inventory (Combinations)
	var combinations []dto.CombinationResponse
	for _, c := range product.Combinations {

		// Safely convert pointers to strings
		var opt1, opt2, opt3 *string
		if c.Option1ID != nil {
			str := c.Option1ID.String()
			opt1 = &str
		}
		if c.Option2ID != nil {
			str := c.Option2ID.String()
			opt2 = &str
		}
		if c.Option3ID != nil {
			str := c.Option3ID.String()
			opt3 = &str
		}

		combinations = append(combinations, dto.CombinationResponse{
			ID:                c.ID.String(),
			SKU:               c.SKU,
			Price:             c.Price,
			Quantity:          c.Quantity,
			Option1ID:         opt1,
			Option2ID:         opt2,
			Option3ID:         opt3,
			CombinationString: c.CombinationString,
		})
	}

	response.Combinations = combinations // Make sure this is in your ProductResponse struct!

	// save the data to redis
	jsonData, err := json.Marshal(response)
	if err == nil {
		initializers.RClient.Set(initializers.Ctx, cacheKey, jsonData, 10*time.Minute)
		initializers.RClient.SAdd(initializers.Ctx, "cache:product:id", cacheKey)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "product was retrieved successfully",
		"data":    response,
	})
}

// stopped here
func UpdateProduct(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "error while parsing the id",
			"error":   err.Error(),
		})
		return
	}

	var body struct {
		Title       string   `json:"title"`
		Description string   `json:"description"`
		Brand       string   `json:"brand"`
		Price       float64  `json:"price"`
		OldPrice    float64  `json:"oldPrice"`
		Active      bool     `json:"active"`
		Tags        []string `json:"tags"` // this should of type uuid
	}

	err = c.ShouldBindJSON(&body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "error while parsing the body",
			"error":   err.Error(),
		})
		return
	}

	var product models.Product
	result := initializers.DB.First(&product, id)

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "error while retrieving the product",
			"error":   result.Error.Error(),
		})
		return
	}

	if body.Title != "" {
		product.Title = body.Title
	}
	if body.Description != "" {
		product.Description = body.Description
	}

	if body.Price != 0 {
		product.Price = body.Price
		fmt.Println(product.Price, body.Price)
	}
	if body.OldPrice != 0 {
		product.OldPrice = body.OldPrice
	}
	if body.Active != product.Active {
		product.Active = body.Active
	}

	// ensures that there are no duplicates (sets)
	tagNameSet := make(map[string]struct{})
	for _, t := range body.Tags {
		cleaned := strings.ToLower(strings.TrimSpace(t))
		if cleaned != "" {
			tagNameSet[cleaned] = struct{}{}
		}
	}

	// convert the set to a slice in order to map through
	uniqueTagNames := make([]string, 0, len(tagNameSet))
	for name := range tagNameSet {
		uniqueTagNames = append(uniqueTagNames, name)
	}

	// star the transaction
	tx := initializers.DB.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to start transaction"})
		return
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	if err := tx.Save(&product).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to update product fields", "error": err.Error()})
		return
	}

	// fetch existing tags in the a single query
	var existingTags []models.Tag
	if err := tx.Where("name IN ?", uniqueTagNames).Find(&existingTags).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to fetch existing tags", "error": err.Error()})
		return
	}

	// build a map of existing tags
	existingTagMap := make(map[string]models.Tag)
	for _, tag := range existingTags {
		existingTagMap[tag.Name] = tag
	}

	// create missing tags if there are any
	var allTags []models.Tag
	for _, name := range uniqueTagNames {
		if existingTag, found := existingTagMap[name]; found {
			allTags = append(allTags, existingTag)
			continue
		}
		newTag := models.Tag{Name: name}
		if err := tx.Create(&newTag).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to create tag", "error": err.Error()})
			return
		}
		allTags = append(allTags, newTag)
	}

	// replace the product tags in one go
	if err := tx.Model(&product).Association("Tags").Replace(allTags); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to update product tags", "error": err.Error()})
		return
	}

	// commit the transaction
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to commit transaction", "error": err.Error()})
		return
	}

	go func() {
		productsKeys, _ := initializers.RClient.SMembers(initializers.Ctx, "cache:products").Result()
		for _, key := range productsKeys {
			initializers.RClient.Del(initializers.Ctx, key)
		}

		productKeys, _ := initializers.RClient.SMembers(initializers.Ctx, "cache:product:id").Result()
		for _, key := range productKeys {
			initializers.RClient.Del(initializers.Ctx, key)
		}

		// clear the set itself
		initializers.RClient.Del(initializers.Ctx, "cache:products")
		initializers.RClient.Del(initializers.Ctx, "cache:product:id")
	}()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Product was updated successfully",
		"data":    product,
	})
}

func UpdateProductImages(c *gin.Context) {
	// get the product images sent from the front end and check if they have been modified (e.g : sending 2 images when the db has 4)
	// if the product images exist on the database but not on the data received then delete them based on the id (some filtering mecanisim using ids)
	// get the new images from the formData
	// upload them to the s3 bucket
	// return a success message

	// get the product id
	productId, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "error while parsing the id",
			"error":   err.Error(),
		})
		return
	}

	// get the product images from the db using the product id
	var images []models.ProductImage
	result := initializers.DB.Where("product_id = ?", productId).Find(&images)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "error while retrieving the product images",
			"error":   result.Error.Error(),
		})
		return
	}

	// get the images from the request and decode the data
	existingImagesJson := c.PostForm("existingImages")
	var existingImages []dto.UpdateProductsImageInput
	if existingImagesJson != "" {
		err := json.Unmarshal([]byte(existingImagesJson), &existingImages)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "invalid images JSON",
				"error":   err.Error(),
			})
			return
		}
	}

	// map through the existing images1
	var deletedImages []dto.UpdateProductsImageInput
	for _, dbImage := range images {
		found := false
		for _, existingImage := range existingImages {
			if dbImage.ID.String() == existingImage.ID {
				found = true
				break
			}
		}
		if !found {
			deletedImages = append(deletedImages, dto.UpdateProductsImageInput{ID: dbImage.ID.String()})
		}
	}

	// Transaction start
	tx := initializers.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "internal server error",
			})
		}
	}()

	// map through the deleted images in the front and deleted them from the db
	for _, deletedImage := range deletedImages {
		parsedDeletedImageId, err := uuid.Parse(deletedImage.ID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "invalid images id",
				"error":   err.Error(),
			})
			return
		}
		result := tx.Delete(&models.ProductImage{}, parsedDeletedImageId)
		if result.Error != nil {
			tx.Rollback()
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "invalid images id",
				"error":   result.Error.Error(),
			})
			return
		}
	}

	var productImages []models.ProductImage
	// in case there are new images to be upload
	form, err := c.MultipartForm()
	if err == nil {
		// get the files
		files := form.File["images"]
		fmt.Println("files : ", files)

		// the product images max length is 5
		if len(files)+len(existingImages) > 5 {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "The maximum number of images is 5",
			})
			return
		}

		for _, file := range files {
			src, err := file.Open()
			if err != nil {
				tx.Rollback()
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "failed to open file", "error": err.Error()})
				return
			}

			// TODO : save the images in a folder named after the shop name
			// e.g : lk-parfumo/
			key := fmt.Sprintf("uploads/%d_%s", time.Now().UnixNano(), filepath.Base(file.Filename))
			bucketName := os.Getenv("B2_BUCKET_NAME")

			_, err = initializers.S3Client.PutObject(context.TODO(), &s3.PutObjectInput{
				Bucket:      aws.String(bucketName),
				Key:         aws.String(key),
				Body:        src,
				ContentType: aws.String(file.Header.Get("Content-Type")),
			})
			src.Close() // close immediately after upload

			if err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": fmt.Sprintf("failed to upload %s", file.Filename), "error": err.Error()})
				return
			}

			url := fmt.Sprintf("https://%s.s3.%s.backblazeb2.com/%s", bucketName, os.Getenv("B2_REGION"), key)

			// urls = append(urls, url)
			productImages = append(productImages, models.ProductImage{
				ProductID: productId,
				URL:       url,
			})
		}

		if len(productImages) > 0 {
			if err := tx.Create(&productImages).Error; err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to save images", "error": err.Error()})
				return
			}
		}
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "failed to commit transaction",
			"error":   err.Error(),
		})
		return
	}

	cacheKey := fmt.Sprintf("product:id=%s", productId)
	if err := initializers.RClient.Del(initializers.Ctx, cacheKey, "products").Err(); err != nil {
		fmt.Println("Failed to delete the product cache key")
	}

	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"exitingImages": existingImages,
		"images":        productImages,
	})
}

func UpdateVariant(c *gin.Context) {
	variantId, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid variant ID", "error": err.Error()})
		return
	}

	// Parse request body
	var body dto.UpdateVariantDTO
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid request body", "error": err.Error()})
		return
	}

	// Retrieve the variant (and preload existing items if you want to verify)
	var variant models.Variant
	if err := initializers.DB.First(&variant, "id = ?", variantId).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Variant not found", "error": err.Error()})
		return
	}

	// Update title if changed
	if body.Title != "" && body.Title != variant.Title {
		if err := initializers.DB.Model(&variant).Update("title", body.Title).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to update variant title", "error": err.Error()})
			return
		}
	}

	// Update variant items
	// take into consideration the variants that weren't created at first but were later created in the editing phase
	for _, item := range body.VariantItems {
		// You only need the ID to update directly
		updateData := map[string]interface{}{
			"value":    item.Value,
			"quantity": item.Quantity,
			"price":    item.Price,
		}

		// check if the variant item id exists, if not then create the new variant item
		// else
		// simply modify the existing variant item

		if err := initializers.DB.Model(&models.VariantItem{}).Where("id = ?", item.ID).
			Updates(updateData).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": fmt.Sprintf("Failed to update variant item ID: %s", item.ID),
				"error":   err.Error(),
			})
			return
		}
	}

	cacheKey := fmt.Sprintf("product:id=%s", variant.ProductID)
	if err := initializers.RClient.Del(initializers.Ctx, cacheKey, "products").Err(); err != nil {
		fmt.Println("Failed to delete the product cache key")
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Variant and items updated successfully"})
}

func DeleteProduct(c *gin.Context) {
	// 1) Parse product UUID
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "invalid product ID format",
			"error":   err.Error(),
		})
		return
	}

	// 2) Start transaction
	tx := initializers.DB.Begin()

	// 3) Delete product-tag associations (product_tags join table)
	if err := tx.Exec("DELETE FROM product_tags WHERE product_id = ?", id).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "failed to delete product-tag associations",
			"error":   err.Error(),
		})
		return
	}

	// 4) Delete the product itself
	if err := tx.Delete(&models.Product{}, "id = ?", id).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "failed to delete product",
			"error":   err.Error(),
		})
		return
	}

	// 5) Commit transaction
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "failed to commit transaction",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "product was deleted successfully",
	})
}

func GetTags(c *gin.Context) {
	var tags []models.Tag
	result := initializers.DB.Find(&tags)
	if result.Error != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Error while retrieving the tags",
			"error":   result.Error.Error(),
		})
		return
	}

	var formattedTags []string
	if len(tags) > 0 {
		for _, tag := range tags {
			formattedTags = append(formattedTags, tag.Name)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "tags were retrieved successfully",
		"data":    formattedTags,
	})
}

func GetAllTags(c *gin.Context) {
	var tags []models.Tag

	initializers.DB.Find(&tags)

	c.JSON(http.StatusOK, gin.H{
		"data": tags,
	})
}

func DeleteTag(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "error",
		})
		return
	}

	initializers.DB.Delete(&models.Tag{}, id)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
	})
}

func CreateTag(c *gin.Context) {

}
