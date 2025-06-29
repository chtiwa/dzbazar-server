package controllers

import (
	"context"
	"encoding/json"
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
)

type VariantItemInput struct {
	Value    string  `json:"value"`
	Price    float64 `json:"price"`
	Quantity int     `json:"quantity"`
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
}

// check admin
func CreateProduct(c *gin.Context) {
	title := c.PostForm("title")
	price := c.PostForm("price")
	description := c.PostForm("description")
	categoryID := c.PostForm("categoryId")

	if title == "" || categoryID == "" || price == "" || description == "" {
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

	parsedCategoryID, err := uuid.Parse(categoryID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "invalid category id",
			"error":   err.Error(),
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
		CategoryID:  parsedCategoryID,
		Price:       parsedPrice,
	}

	if err := tx.Create(&product).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to create product", "error": err.Error()})
		return
	}

	// Upload images and store in DB
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

func UpdateVariant(c *gin.Context) {
	// get the variant id
	variantId, err := uuid.Parse(c.Param("id"))

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "error while parsing the body",
			"error":   err.Error(),
		})
		return
	}

	// define the struct
	var body dto.UpdateVariantDTO
	// bind the body
	err = c.ShouldBindJSON(&body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "error while parsing the body",
			"error":   err.Error(),
		})
		return
	}

	// get the variant
	var variant models.Variant
	result := initializers.DB.First(&variant, variantId)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "failed while retrieving the variant",
			"error":   result.Error.Error(),
		})
		return
	}

	// check if the title exists
	if body.Title != "" {
		variant.Title = body.Title
		result = initializers.DB.Save(&variant)
		if result.Error != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "failed while update the variant",
				"error":   result.Error.Error(),
			})
			return
		}
	}

	// map through the variant items and update the items in bulk
	if len(body.VariantItems) > 0 {
		var ids []string
		caseQuantity := "CASE id "
		caseValue := "CASE id "
		casePrice := "CASE id "

		for _, vi := range body.VariantItems {
			ids = append(ids, fmt.Sprintf("'%s'", vi.ID))
			caseQuantity += fmt.Sprintf("WHEN '%s' THEN %d", vi.ID, vi.Quantity)
			caseValue += fmt.Sprintf("WHEN '%s' THEN '%s'", vi.ID, vi.Value)
			caseValue += fmt.Sprintf("WHEN '%s' THEN '%s'", vi.ID, vi.Price)
		}

		caseQuantity += "END"
		caseValue += "END"
		casePrice += "END"

		updateQuery := fmt.Sprintf(`
			UPDATE variant_items
			SET quantity = %s
				value = %s
				price = %s
			WHERE id IN (%s)
		`, caseQuantity, caseValue, casePrice, strings.Join(ids, ","))

		result := initializers.DB.Exec(updateQuery)

		if result.Error != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "failed while updating the variant items",
				"error":   result.Error.Error(),
			})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "variant updated successfully",
	})
}

func GetProducts(c *gin.Context) {
	var products []models.Product

	result := initializers.DB.Preload("Images").Find(&products)

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

	result := query.Limit(10).Preload("Images").Preload("Category").Find(&products)

	if result.Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Error retrieving the orders",
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

	result := initializers.DB.Preload("Images").Preload("Category").Preload("Variants").Preload("Variants.VariantItems").First(&product, "id = ?", productId)

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
		Category: dto.CategoryResponse{
			ID:    product.Category.ID.String(),
			Title: product.Category.Title,
		},
		Variants: make([]dto.VariantResponse, 0),
	}

	var images []dto.ProductImageResponse
	for _, i := range product.Images {
		images = append(images, dto.ProductImageResponse{
			ID:  i.ID.String(),
			URL: i.URL,
		})
	}

	response.Images = images

	for _, v := range product.Variants {
		var vr dto.VariantResponse
		vr.ID = v.ID.String()
		vr.Title = v.Title
		vr.VariantItems = make([]dto.VariantItemSimple, 0)

		for _, item := range v.VariantItems {
			vr.VariantItems = append(vr.VariantItems, dto.VariantItemSimple{
				ID:       item.ID.String(),
				Value:    item.Value,
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

func GetProductImages(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "error while parsing the id",
			"error":   err.Error(),
		})
		return
	}

	// var imgs []models.ProductImage
	// result := initializers.DB.Where("product_id = ?", id).Find(&imgs)

	var product models.Product

	result := initializers.DB.Preload("Images").First(&product, "id = ?", id)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "error while retrieving the product images",
			"error":   result.Error.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    product,
	})
}

func UpdateProduct(c *gin.Context) {}

func DeleteProduct(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "error while parsing the id",
			"error":   err.Error(),
		})
		return
	}

	result := initializers.DB.Delete(&models.Product{}, id)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "error while parsing the id",
			"error":   result.Error.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "product was deleted successfully",
	})
}
