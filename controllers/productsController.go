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
	"github.com/chtiwa/lk-parfumo-server/dto"
	"github.com/chtiwa/lk-parfumo-server/initializers"
	"github.com/chtiwa/lk-parfumo-server/models"
	"github.com/chtiwa/lk-parfumo-server/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type VariantItemInput struct {
	Value    string `json:"value"`
	Price    int    `json:"price"`
	Quantity int    `json:"quantity"`
}

type TagInput struct {
	Name string `json:"name"`
}

type VariantInput struct {
	Title        string             `json:"title"`
	VariantItems []VariantItemInput `json:"variantItems"`
}

type CreateProductInput struct {
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Price       float64        `json:"price"`
	CategoryID  string         `json:"categoryId"`
	Variants    []VariantInput `json:"variants"`
	Tags        []string       `json:"tags"`
}

// check admin
func CreateProduct(c *gin.Context) {
	title := c.PostForm("title")
	price := c.PostForm("price")
	description := c.PostForm("description")

	if title == "" || price == "" || description == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "error while parsing the body",
		})
		return
	}

	parsedPrice, err := strconv.ParseFloat(price, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "invalid price",
		})
		return
	}

	var variants []VariantInput
	variantsJson := c.PostForm("variants")
	if variantsJson != "" {
		if err := json.Unmarshal([]byte(variantsJson), &variants); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "invalid variants JSON",
				"error":   err.Error(),
			})
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

	// Step 1: Parse tags JSON from form data
	tagsJson := c.PostForm("tags")

	if tagsJson != "" {
		if err := json.Unmarshal([]byte(tagsJson), &tagNames); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid tags JSON", "error": err.Error()})
			return
		}

		// Step 2: Normalize tag names and collect unique names
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

		if len(normalizedNames) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "no valid tags provided"})
			return
		}

		// Step 3: Process tags inside the existing transaction
		// Fetch existing tags in one query
		var existingTags []models.Tag
		if err := tx.Where("name IN ?", normalizedNames).Find(&existingTags).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to fetch tags", "error": err.Error()})
			return
		}

		// Build a map of existing tags for quick lookup
		existingMap := make(map[string]models.Tag)
		for _, tag := range existingTags {
			existingMap[tag.Name] = tag
		}

		// Determine missing tags to create
		var tagsToCreate []models.Tag
		for _, tagName := range normalizedNames {
			if _, exists := existingMap[tagName]; !exists {
				tagsToCreate = append(tagsToCreate, models.Tag{Name: tagName})
			}
		}

		// Create missing tags in one batch
		if len(tagsToCreate) > 0 {
			if err := tx.Create(&tagsToCreate).Error; err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to create tags", "error": err.Error()})
				return
			}
		}

		// Merge existing + new tags
		tags = append(existingTags, tagsToCreate...)
	}

	form, err := c.MultipartForm()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "invalid form data",
			"error":   err.Error(),
		})
		return
	}

	files := form.File["images"]
	if len(files) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "no files to upload",
		})
		return
	}

	// urls := make([]string, 0, len(files))

	product := models.Product{
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

	// Upload images and store in DB should be last in case anything else fails
	var productImages []models.ProductImage
	for _, file := range files {
		src, err := file.Open()
		if err != nil {
			tx.Rollback()
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "failed to open file", "error": err.Error()})
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
		src.Close() // close immediately after upload

		if err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": fmt.Sprintf("failed to upload %s", file.Filename), "error": err.Error()})
			return
		}

		url := fmt.Sprintf("https://%s.s3.%s.backblazeb2.com/%s", bucketName, os.Getenv("B2_REGION"), key)

		// urls = append(urls, url)
		productImages = append(productImages, models.ProductImage{
			ProductID: product.ID,
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

	// Create variants + items
	for _, v := range variants {
		variant := models.Variant{ProductID: product.ID, Title: v.Title}
		if err := tx.Create(&variant).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to create variant", "error": err.Error()})
			return
		}

		var items []models.VariantItem
		for _, item := range v.VariantItems {
			items = append(items, models.VariantItem{
				VariantID: variant.ID,
				Value:     item.Value,
				Price:     item.Price,
				Quantity:  item.Quantity,
			})
		}

		if len(items) > 0 {
			if err := tx.Create(&items).Error; err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to create variant items", "error": err.Error()})
				return
			}
		}
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "transaction failed", "error": err.Error()})
		return
	}

	product.Images = productImages

	var cursor uint64
	for {
		var keys []string
		var err error
		keys, cursor, err = initializers.RClient.Scan(initializers.Ctx, cursor, "products:*", 100).Result()
		if err != nil {
			fmt.Printf("failed to scan keys")
		}
		if len(keys) > 0 {
			if err := initializers.RClient.Del(initializers.Ctx, keys...).Err(); err != nil {
				fmt.Printf("failed to delete keys")
			}
		}
		if cursor == 0 {
			break
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Product created successfully",
		"data":    product,
	})
}

func GetProducts(c *gin.Context) {
	tag := c.Query("tag")
	pageString := c.Query("page")
	page := 1

	if pageString != "" {
		if parsedPage, err := strconv.Atoi(pageString); err == nil && parsedPage > 0 {
			page = parsedPage
		}
	}

	// redis key
	cacheKey := fmt.Sprintf("products:tag=%s:page=%d", tag, page)
	val, err := initializers.RClient.Get(initializers.Ctx, cacheKey).Result()
	if err == nil {
		var cachedReponse map[string]interface{}
		if unmarshalErr := json.Unmarshal([]byte(val), &cachedReponse); unmarshalErr == nil {
			c.JSON(http.StatusOK, cachedReponse)
			return // crucial: avoid continuing to DB query
		}
		// if unmarshaling failed, fall through to DB fetch
	}

	var totalRows int64
	var products []models.Product
	// db := initializers.DB.Model(&models.Product{}).Where("products.active = ?", true).Preload("Images").Preload("Tags")
	db := initializers.DB.Model(&models.Product{}).Preload("Images").Preload("Tags")

	if tag != "" {
		// Filter products by tag name
		// Join product_tags and tags to filter products by tag name
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

	perPage := 8.0
	totalPages := math.Ceil(float64(totalRows) / perPage)

	offset := (page - 1) * int(perPage)
	if page > int(totalPages) {
		offset = 0
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
	}

	c.JSON(http.StatusOK, response)
}

func GetProductsBySearch(c *gin.Context) {
	search := c.Query("search")

	var products []models.Product
	query := initializers.DB.Order("updated_at DESC")

	if search != "" {
		query = query.Where("title ILIKE ? OR description ILIKE ?", "%"+search+"%", "%"+search+"%")
	}

	result := query.Limit(10).Preload("Images").Preload("Variants").Preload("Variants.VariantItems").Find(&products)

	if result.Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Error retrieving the products",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "products were retrieved successfully",
		"data":    products,
	})
}

func GetProduct(c *gin.Context) {
	productId, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "error while parsing the product id",
			"error":   err.Error(),
		})
		return
	}

	// redis key
	cacheKey := fmt.Sprintf("product:id=%s", c.Param("id"))

	val, err := initializers.RClient.Get(initializers.Ctx, cacheKey).Result()
	if err == nil {
		var cachedResponse dto.ProductResponse
		if unmarshalErr := json.Unmarshal([]byte(val), &cachedResponse); unmarshalErr == nil {
			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"message": "product was retrieved successfully (from cache)",
				"data":    cachedResponse,
			})
			return // crucial: avoid continuing to DB query
		}
		// if unmarshaling failed, fall through to DB fetch
	}

	var product models.Product

	result := initializers.DB.Preload("Images").Preload("Variants").Preload("Variants.VariantItems").Preload("Tags").First(&product, "id = ?", productId)

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "error while retrieving the product",
			"error":   result.Error.Error(),
		})
		return
	}

	response := dto.ProductResponse{
		ID:          product.ID.String(),
		Title:       product.Title,
		Description: product.Description,
		Price:       product.Price,
		OldPrice:    product.OldPrice,
		Variants:    make([]dto.VariantResponse, 0),
	}

	var images []dto.ProductImageResponse
	for _, i := range product.Images {
		images = append(images, dto.ProductImageResponse{
			ID:  i.ID.String(),
			URL: i.URL,
		})
	}
	response.Images = images

	var tags []string
	for _, t := range product.Tags {
		tags = append(tags, t.Name)
	}
	response.Tags = tags

	for _, v := range product.Variants {
		var vr dto.VariantResponse
		vr.ID = v.ID.String()
		vr.Title = v.Title
		vr.VariantItems = make([]dto.VariantItemSimple, 0)

		for _, item := range v.VariantItems {
			vr.VariantItems = append(vr.VariantItems, dto.VariantItemSimple{
				ID:       item.ID.String(),
				Value:    item.Value,
				Price:    item.Price,
				Quantity: item.Quantity,
			})
		}

		response.Variants = append(response.Variants, vr)
	}

	// save the data to redis
	jsonData, err := json.Marshal(response)
	if err == nil {
		initializers.RClient.Set(initializers.Ctx, cacheKey, jsonData, 10*time.Minute)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "product was retrieved successfully",
		"data":    response,
	})
}

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

	// delete the cached key in redis
	cacheKey := fmt.Sprintf("product:id=%s", id)
	if err := initializers.RClient.Del(initializers.Ctx, cacheKey).Err(); err != nil {
		fmt.Println("Failed to delete the product cache key")
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Product was updated successfully",
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
	if err := initializers.RClient.Del(initializers.Ctx, cacheKey).Err(); err != nil {
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
