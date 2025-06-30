package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/chtiwa/herbs-store-client/dto"
	"github.com/chtiwa/herbs-store-client/initializers"
	"github.com/chtiwa/herbs-store-client/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
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

	// Parse tags JSON and fetch/create tags
	var tagNames []string
	var tags []models.Tag
	tagsJson := c.PostForm("tags")
	fmt.Println(tagsJson)
	if tagsJson != "" {
		if err := json.Unmarshal([]byte(tagsJson), &tagNames); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid tags JSON", "error": err.Error()})
			return
		}

		for _, tagName := range tagNames {
			tagName = strings.ToLower(strings.TrimSpace(tagName))
			if tagName == "" {
				continue
			}

			var tag models.Tag
			err := initializers.DB.Where("name = ?", tagName).First(&tag).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				tag = models.Tag{Name: tagName}
				if err := initializers.DB.Create(&tag).Error; err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to create tag", "error": err.Error()})
					return
				}
			} else if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to fetch tag", "error": err.Error()})
				return
			}
			tags = append(tags, tag)
		}
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

	// Transaction start
	tx := initializers.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "internal server error"})
		}
	}()

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
		bucketName := os.Getenv("AWS_BUCKET_NAME")

		_, err = initializers.S3Client.PutObject(context.TODO(), &s3.PutObjectInput{
			Bucket:      aws.String(bucketName),
			Key:         aws.String(key),
			Body:        src,
			ACL:         "public-read",
			ContentType: aws.String(file.Header.Get("Content-Type")),
		})
		src.Close() // close immediately after upload

		if err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": fmt.Sprintf("failed to upload %s", file.Filename), "error": err.Error()})
			return
		}

		url := fmt.Sprintf("https://%s.s3.amazonaws.com/%s", bucketName, key)
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

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Product created successfully",
		"data":    product,
	})
}

func GetProducts(c *gin.Context) {
	tag := c.Query("tag")
	var products []models.Product
	db := initializers.DB.Preload("Images").Preload("Tags")

	if tag != "" {
		// Filter products by tag name
		// Join product_tags and tags to filter products by tag name
		db = db.Joins("JOIN product_tags ON product_tags.product_id = products.id").
			Joins("JOIN tags ON tags.id = product_tags.tag_id").
			Where("LOWER(tags.name) = ?", strings.ToLower(tag))
	} else {
		// No filter: get latest products by created_at
		db = db.Order("created_at DESC")
	}

	result := db.Find(&products)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "error while retrieving the products",
			"error":   result.Error.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    products,
	})
}

func GetProductsBySearch(c *gin.Context) {
	search := c.Query("search")

	var products []models.Product
	query := initializers.DB.Order("updated_at DESC")

	if search != "" {
		query = query.Where("title ILIKE ? OR description ILIKE ?", "%"+search+"%", "%"+search+"%")
	}

	result := query.Limit(10).Preload("Images").Find(&products)

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
	}
	if body.OldPrice != 0 {
		product.OldPrice = body.OldPrice
	}
	if body.Active != product.Active {
		product.Active = body.Active
	}

	result = initializers.DB.Save(&product)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "error while saving the product",
			"error":   result.Error.Error(),
		})
		return
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
			bucketName := os.Getenv("AWS_BUCKET_NAME")

			_, err = initializers.S3Client.PutObject(context.TODO(), &s3.PutObjectInput{
				Bucket:      aws.String(bucketName),
				Key:         aws.String(key),
				Body:        src,
				ACL:         "public-read",
				ContentType: aws.String(file.Header.Get("Content-Type")),
			})
			src.Close() // close immediately after upload

			if err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": fmt.Sprintf("failed to upload %s", file.Filename), "error": err.Error()})
				return
			}

			url := fmt.Sprintf("https://%s.s3.amazonaws.com/%s", bucketName, key)
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
