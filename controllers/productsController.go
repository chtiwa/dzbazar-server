package controllers

import (
	"net/http"

	"github.com/chtiwa/herbs-store-client/dto"
	"github.com/chtiwa/herbs-store-client/initializers"
	"github.com/chtiwa/herbs-store-client/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type VariantItemInput struct {
	Value    string `json:"value"`
	Quantity int    `json:"quantity"`
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
	var body CreateProductInput
	err := c.ShouldBindJSON(&body)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "failed to parse the body",
			"error":   err.Error(),
		})
		return
	}

	parsedCatgeoryId, err := uuid.Parse(body.CategoryID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "error while parsing the category id",
			"error":   err.Error(),
		})
		return
	}

	// form, err := c.MultipartForm()
	// if err != nil {
	// 	c.JSON(http.StatusBadRequest, gin.H{
	// 		"success": false,
	// 		"message": "invalid form data",
	// 		"error":   err.Error(),
	// 	})
	// 	return
	// }

	// files := form.File["files"]
	// if len(files) == 0 {
	// 	c.JSON(http.StatusBadRequest, gin.H{
	// 		"success": false,
	// 		"message": "no files to upload",
	// 	})
	// 	return
	// }

	// var urls []string
	// for _, file := range files {
	// 	// open the uploaded file
	// 	src, err := file.Open()
	// 	if err != nil {

	// 		c.JSON(http.StatusBadRequest, gin.H{
	// 			"success": false,
	// 			"message": "invalid form data",
	// 			"error":   err.Error(),
	// 		})
	// 		return
	// 	}
	// 	defer src.Close()

	// 	// create a unique key for the S3 object
	// 	key := fmt.Sprintf("uploads/%d_%s", time.Now().UnixNano(), filepath.Base(file.Filename))

	// 	bucketName := os.Getenv("AWS_BUCKET_NAME")

	// 	// uplaod to S3
	// 	_, err = initializers.S3Client.PutObject(context.TODO(), &s3.PutObjectInput{
	// 		Bucket:      aws.String(bucketName),
	// 		Key:         aws.String(key),
	// 		Body:        src,
	// 		ACL:         "public-read",
	// 		ContentType: aws.String(file.Header.Get("Content-Type")),
	// 	})

	// 	if err != nil {
	// 		c.JSON(http.StatusInternalServerError, gin.H{
	// 			"success": false,
	// 			"error":   fmt.Sprintf("failed to upload %s: %v", file.Filename, err),
	// 		})
	// 		return
	// 	}

	// 	// Construct the public URL
	// 	url := fmt.Sprintf("https://%s.s3.amazonaws.com/%s", bucketName, key)
	// 	urls = append(urls, url)
	// }

	product := models.Product{Title: body.Title, Description: body.Description, CategoryID: parsedCatgeoryId}

	result := initializers.DB.Create(&product)

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "error while creating the product",
			"error":   result.Error.Error(),
		})
		return
	}

	// // create product images and associate them witht the product
	// var productImages []models.ProductImage
	// for _, url := range urls {
	// 	productImages = append(productImages, models.ProductImage{
	// 		ProductID: product.ID,
	// 		URL:       url,
	// 	})
	// }

	// if len(productImages) > 0 {
	// 	result = initializers.DB.Create(&productImages)

	// 	if result.Error != nil {
	// 		c.JSON(http.StatusInternalServerError, gin.H{
	// 			"success": false,
	// 			"message": "error while uploading the images",
	// 			"error":   result.Error.Error(),
	// 		})
	// 		return
	// 	}
	// }

	// product.Images = productImages

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Product was created succssfully",
		"data":    product,
	})

}

func CreateVariant(c *gin.Context) {
	productID, err := uuid.Parse(c.Param("id"))

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": true,
			"message": "error while parsing the product id",
			"error":   err.Error(),
		})
		return
	}

	var body struct {
		Title        string `json:"title"`
		VariantItems []struct {
			Value    string `json:"value"`
			Quantity int    `json:"quantity"`
		} `json:"variantItems"`
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

	var items []models.VariantItem
	for _, vi := range body.VariantItems {
		items = append(items, models.VariantItem{
			Value:    vi.Value,
			Quantity: vi.Quantity,
		})
	}

	variant := models.Variant{
		ProductID:    productID,
		Title:        body.Title,
		VariantItems: items,
	}

	result := initializers.DB.Create(&variant)

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "error while creating the variant",
			"error":   result.Error.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "variant was created successfully",
		"data":    variant,
	})
}

func GetProducts(c *gin.Context) {
	var products []models.Product

	result := initializers.DB.Find(&products)

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

	result := initializers.DB.Preload("Images").Preload("Variants").Preload("Variants.VariantItems").Preload("Category").First(&product, "id = ?", productId)

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
		Images:      product.Images, // assuming this is []string
		Category: dto.CategoryResponse{
			ID:    product.Category.ID.String(),
			Title: product.Category.Title,
		},
		Variants: make([]dto.VariantResponse, 0),
	}

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
func UpdateProduct(c *gin.Context) {}
func DeleteProduct(c *gin.Context) {}
