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
	"github.com/chtiwa/lk-parfumo-server/dto"
	"github.com/chtiwa/lk-parfumo-server/initializers"
	"github.com/chtiwa/lk-parfumo-server/models"
	"github.com/chtiwa/lk-parfumo-server/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func CreateLandingPage(c *gin.Context) {
	// get the product id
	id, err := uuid.Parse(c.PostForm("productId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Something went wrong while parsing the id",
			"error":   err.Error(),
		})
		return
	}

	// check if the product exists
	var product models.Product
	result := initializers.DB.First(&product, id)
	if result.Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Error while fetching the product",
			"error":   result.Error.Error(),
		})
		return
	}

	title := c.PostForm("title")
	if title == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "error while parsing the body",
		})
		return
	}

	// init the transaction
	tx := initializers.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("PANIC:", r)
			debug.PrintStack()
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   fmt.Sprintf("%v", r),
			})
		}
	}()

	// get the multipart form data
	form, err := c.MultipartForm()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid form data",
			"error":   err.Error(),
		})
		return
	}

	// get the images
	files := form.File["images"]
	if len(files) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "No files to upload",
		})
		return
	}

	// create the landing page using the product id
	landingPage := models.LandingPage{
		ProductID: product.ID,
		Title:     title,
	}

	if err := tx.Create(&landingPage).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "failed to create the landing page",
			"error":   err.Error(),
		})
		return
	}

	// upload the landing page images
	var landingPageImages []models.LandingPageImage
	for index, file := range files {
		src, err := file.Open()
		if err != nil {
			tx.Rollback()
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "failed to open the file",
				"error":   err.Error(),
			})
			return
		}

		key := fmt.Sprintf("uploads/%d_%s", time.Now().UnixNano(), filepath.Base(file.Filename))
		bucketName := os.Getenv("B2_BUCKET_NAME")
		region := os.Getenv("B2_REGION")

		_, err = initializers.S3Client.PutObject(context.TODO(), &s3.PutObjectInput{
			Bucket:      aws.String(bucketName),
			Key:         aws.String(key),
			Body:        src,
			ACL:         types.ObjectCannedACLPublicRead,
			ContentType: aws.String(file.Header.Get("Content-Type")),
		})
		src.Close()

		if err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": fmt.Sprintf("failed to upload %s", file.Filename), "error": err.Error()})
			return
		}

		url := fmt.Sprintf("https://%s.s3.%s.backblazeb2.com/%s", bucketName, region, key)
		landingPageImages = append(landingPageImages, models.LandingPageImage{
			LandingPageID: landingPage.ID,
			URL:           url,
			OrderIndex:    index,
		})
	}

	// create the landing page images in the database
	if len(landingPageImages) > 0 {
		if err := tx.Create(&landingPageImages).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Failed to save the images",
				"error":   err.Error(),
			})
			return
		}
	}

	if err := tx.Commit().Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Transaction failed",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "The landing page was created successfully",
	})
}

func GetLandingPages(c *gin.Context) {
	// TODO : add caching / pagination
	var landingPages []models.LandingPage
	result := initializers.DB.Preload("Images").Find(&landingPages)

	if result.Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Error while fetching the landing pages",
			"error":   result.Error.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"mesasge": "The landing pages were retrieved successfully",
		"data":    landingPages,
	})
}

func IndexLandingPage(c *gin.Context) {
	// get the landing page id and preload the product along with the variants, preload the landing page images
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "something went wrong while parsing the id",
			"error":   err.Error(),
		})
		return
	}

	cacheKey := fmt.Sprintf("landing-page:id=%s", c.Param("id"))

	val, err := initializers.RClient.Get(initializers.Ctx, cacheKey).Result()
	if err == nil {
		var cachedResponse models.LandingPage
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

	var ladingPage models.LandingPage

	// get the landing page
	result := initializers.DB.Preload("Images", func(db *gorm.DB) *gorm.DB {
		return db.Order("order_index ASC")
	}).Preload("Product").Preload("Product.Variants").Preload("Product.Variants.VariantItems").First(&ladingPage, id)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "something went wrong while retrieving the landing page",
			"error":   result.Error.Error(),
		})
		return
	}

	// save the data to redis
	jsonData, err := json.Marshal(ladingPage)
	if err == nil {
		initializers.RClient.Set(initializers.Ctx, cacheKey, jsonData, 10*time.Minute)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "The landing page was retrieved successfully",
		"data":    ladingPage,
	})
}

func UpdateLandingPage(c *gin.Context) {
	// get the product images sent from the front end and check if they have been modified (e.g : sending 2 images when the db has 4)
	// if the product images exist on the database but not on the data received then delete them based on the id
	// get the new images from the formData
	// upload them to the s3 bucket
	// return a success message

	// get the landing page id
	landingPageId, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "error while parsing the id",
			"error":   err.Error(),
		})
		return
	}

	// get the landing page images from the db using the product id
	var images []models.LandingPageImage
	result := initializers.DB.Where("landing_page_id = ?", landingPageId).Find(&images)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "error while retrieving the landing page images",
			"error":   result.Error.Error(),
		})
		return
	}

	// get the images from the request and decode the data
	existingImagesJson := c.PostForm("existingImages")
	var existingImages []dto.UpdateLandingPageImageInput
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

	// map through the existing images
	var deletedImages []dto.UpdateLandingPageImageInput
	for _, dbImage := range images {
		found := false
		for _, existingImage := range existingImages {
			if dbImage.ID.String() == existingImage.ID {
				found = true
				break
			}
		}
		if !found {
			deletedImages = append(deletedImages, dto.UpdateLandingPageImageInput{ID: dbImage.ID.String(), URL: dbImage.URL})
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

		// find the images in the db first (to get the url)
		key := utils.ExtractB2KeyFromURL(deletedImage.URL)
		if key != "" {
			bucketName := os.Getenv("B2_BUCKET_NAME")
			_, err := initializers.S3Client.DeleteObject(context.TODO(), &s3.DeleteObjectInput{
				Bucket: aws.String(bucketName),
				Key:    aws.String(key),
			})

			if err != nil {
				fmt.Printf("Warning: failed to delete old file %s frm B2: %v\n", key, err)
			}
		}

		result := tx.Delete(&models.LandingPageImage{}, parsedDeletedImageId)
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

	var landingPageImages []models.LandingPageImage
	// in case there are new images to be upload
	form, err := c.MultipartForm()
	if err == nil {
		// get the files
		files, ok := form.File["images"]
		if !ok {
			files = []*multipart.FileHeader{}
		}
		// fmt.Println("files : ", files)

		// the product images max length is 5
		if len(files)+len(existingImages) > 10 {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "The maximum number of images is 10",
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

			if !strings.HasPrefix(file.Header.Get("Content-Type"), "image/") {
				tx.Rollback()
				c.JSON(http.StatusBadRequest, gin.H{
					"success": false,
					"message": "only image files allowed",
				})
				return
			}

			// TODO : save the images in a folder named after the shop name
			// e.g : lk-parfumo/
			key := fmt.Sprintf("uploads/%d_%s", time.Now().UnixNano(), filepath.Base(file.Filename))
			bucketName := os.Getenv("B2_BUCKET_NAME")
			region := os.Getenv("B2_REGION")

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

			url := fmt.Sprintf("https://%s.s3.%s.backblazeb2.com/%s", bucketName, region, key)
			// urls = append(urls, url)
			landingPageImages = append(landingPageImages, models.LandingPageImage{
				LandingPageID: landingPageId,
				URL:           url,
			})
		}

		if len(landingPageImages) > 0 {
			if err := tx.Create(&landingPageImages).Error; err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to save images", "error": err.Error()})
				return
			}
		}
	}

	if err := tx.Commit().Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "transaction failed",
			"error":   err.Error(),
		})
		return
	}

	// delete the cached keys in redis
	cachedKeys := []string{fmt.Sprintf("landing-page:id=%s", landingPageId), "landing-pages"}
	if err := initializers.RClient.Del(initializers.Ctx, cachedKeys...).Err(); err != nil {
		fmt.Println("Failed to delete the product cache key")
	}

	c.JSON(http.StatusOK, gin.H{
		"success":        true,
		"existingImages": existingImages,
		"images":         landingPageImages,
	})
}

func DeleteLandingPage(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Error while parsing the id",
			"error":   err.Error(),
		})
		return
	}

	result := initializers.DB.Delete(models.LandingPage{}, id)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Error while deleting the landing page",
			"error":   result.Error.Error(),
		})
		return
	}

	// delete the cache
	cacheKey := fmt.Sprintf("landing-page:id=%s", id)
	if err := initializers.RClient.Del(initializers.Ctx, cacheKey).Err(); err != nil {
		fmt.Println("Failed to delete the product cache key")
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "The landing page was deleted successfully",
	})
}

func DeleteLandingPageImage(c *gin.Context) {
	imageId, err := uuid.Parse(c.Param("imageId"))

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Error while parsing the id",
			"error":   err.Error(),
		})
		return
	}

	var landingPageImage models.LandingPageImage

	// fetch the landing page image
	result := initializers.DB.First(&landingPageImage, "id = ?", imageId)
	if result.Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Error while retrieving the landing page image",
			"error":   result.Error.Error(),
		})
	}

	// delete the image from storage
	key := utils.ExtractB2KeyFromURL(landingPageImage.URL)
	if key != "" {
		bucketName := os.Getenv("B2_BUCKET_NAME")
		_, err := initializers.S3Client.DeleteObject(context.TODO(), &s3.DeleteObjectInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String(key),
		})

		if err != nil {
			fmt.Printf("Warning: failed to delete old file %s frm B2: %v\n", key, err)
		}
	}

	// delete the image from db
	result = initializers.DB.Delete(models.LandingPageImage{}, "id = ?", imageId)
	if result.Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Error while deleting the landing page image",
			"error":   result.Error.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "The landing page image was deleted successfully",
	})
}
