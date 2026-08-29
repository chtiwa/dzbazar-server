package controllers

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/services"
	"github.com/chtiwa/dzbazar-server/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// uploadGeneratedImage stores one AI-generated image in B2 so it survives
// past the response — the source for the "AI Generated" gallery, which lists
// every generation whether or not the merchant ends up using it on a page.
// Best-effort: a failed upload just means that image is missing from the
// gallery, never a failed generation response.
func uploadGeneratedImage(shopID uuid.UUID, contentType string, data []byte) string {
	bucketName := os.Getenv("B2_BUCKET_NAME")
	b2Region := os.Getenv("B2_REGION")
	b2PublicBaseURL := strings.TrimSpace(os.Getenv("B2_PUBLIC_BASE_URL"))

	key := fmt.Sprintf("uploads/landing-pages/ai-generated/%s/%d.webp", shopID.String(), time.Now().UnixNano())
	_, err := initializers.S3Client.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket:      aws.String(bucketName),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ACL:         types.ObjectCannedACLPublicRead,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		fmt.Printf("failed to upload AI-generated image for shop %s: %v\n", shopID, err)
		return ""
	}

	if b2PublicBaseURL != "" {
		return fmt.Sprintf("%s/%s", strings.TrimRight(b2PublicBaseURL, "/"), key)
	}
	return fmt.Sprintf("https://%s.s3.%s.backblazeb2.com/%s", bucketName, b2Region, key)
}

const maxLandingPageSectionRefs = 5

// GenerateLandingPageImageSet takes up to 5 uploaded product photos plus a
// short product context and auto-generates the 5 standard landing-page
// section images (hero, problem, solution, trust, close) using the photos
// as the model's visual reference. Each result is re-encoded to WebP before
// being returned, since these go straight onto a customer-facing page where
// load time matters.
func GenerateLandingPageImageSet(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid shop ID"})
		return
	}

	productName := strings.TrimSpace(c.PostForm("productName"))
	category := strings.TrimSpace(c.PostForm("category"))
	benefit := strings.TrimSpace(c.PostForm("primaryBenefit"))
	painPoint := strings.TrimSpace(c.PostForm("painPoint"))
	audience := strings.TrimSpace(c.PostForm("audience"))
	offerDetails := strings.TrimSpace(c.PostForm("offerDetails"))
	if productName == "" || category == "" || benefit == "" || painPoint == "" || audience == "" || offerDetails == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "productName, category, primaryBenefit, painPoint, audience and offerDetails are required"})
		return
	}

	imageCount, err := strconv.Atoi(c.PostForm("imageCount"))
	if err != nil || imageCount < services.MinLandingPageImageCount || imageCount > services.MaxLandingPageImageCount {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": fmt.Sprintf("imageCount must be between %d and %d", services.MinLandingPageImageCount, services.MaxLandingPageImageCount)})
		return
	}

	form, err := c.MultipartForm()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid multipart form data"})
		return
	}
	files := form.File["images"]
	if len(files) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "at least one product photo is required"})
		return
	}
	if len(files) > maxLandingPageSectionRefs {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": fmt.Sprintf("at most %d reference photos are allowed", maxLandingPageSectionRefs)})
		return
	}

	refs := make([]services.ReferenceImage, len(files))
	for i, fh := range files {
		contentType := fh.Header.Get("Content-Type")
		if !strings.HasPrefix(contentType, "image/") {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "only image files are allowed"})
			return
		}
		src, err := fh.Open()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "failed to read uploaded image"})
			return
		}
		data, err := io.ReadAll(src)
		src.Close()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "failed to read uploaded image"})
			return
		}
		refs[i] = services.ReferenceImage{Bytes: data, MimeType: contentType}
	}

	if err := services.CheckLandingPageImageGenBudget(shopID, imageCount); err != nil {
		if errors.Is(err, services.ErrPlanLimitReached) {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "AI image generation limit reached for your plan. Upgrade to generate more.",
				"code":    "PLAN_LIMIT_REACHED",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to verify plan limits", "error": err.Error()})
		return
	}

	images, err := services.GenerateLandingPageImageSet(refs, productName, category, benefit, painPoint, audience, offerDetails, imageCount)
	if err != nil {
		if errors.Is(err, services.ErrAIUnconfigured) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "Image generation is not configured"})
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": "Failed to generate image set", "error": err.Error()})
		return
	}

	var userID *uuid.UUID
	if u, ok := c.Get("user"); ok {
		if userData, ok := u.(models.User); ok {
			userID = &userData.ID
		}
	}

	webpImages := make([]string, len(images))
	usageRows := make([]models.LandingPageImageGenUsage, len(images))
	for i, img := range images {
		webp, err := services.ToWebP(img)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to compress generated image", "error": err.Error()})
			return
		}
		webpImages[i] = "data:image/webp;base64," + base64.StdEncoding.EncodeToString(webp)
		url := uploadGeneratedImage(shopID, "image/webp", webp)
		usageRows[i] = models.LandingPageImageGenUsage{ShopID: shopID, UserID: userID, URL: url}
	}

	if err := initializers.DB.Create(&usageRows).Error; err != nil {
		fmt.Printf("failed to record AI image gen usage for shop %s: %v\n", shopID, err)
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Landing page image set generated successfully", "data": webpImages})
}

// ListGeneratedImages returns every AI-generated landing-page image for the
// shop, newest first — whether or not the merchant went on to add it to a
// landing page. Rows without a URL (upload to B2 failed at generation time)
// are excluded since there is nothing to show.
func ListGeneratedImages(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid shop ID"})
		return
	}

	page := 1
	if pageString := c.Query("page"); pageString != "" {
		if parsed, err := strconv.Atoi(pageString); err == nil && parsed > 0 {
			page = parsed
		}
	}
	const perPage = 24

	baseQuery := initializers.DB.Model(&models.LandingPageImageGenUsage{}).
		Where("shop_id = ? AND url <> ''", shopID)

	var totalRows int64
	if err := baseQuery.Count(&totalRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to count generated images", "error": err.Error()})
		return
	}

	totalPages := int(math.Ceil(float64(totalRows) / float64(perPage)))
	if totalPages == 0 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}

	var rows []models.LandingPageImageGenUsage
	if err := baseQuery.
		Order("created_at DESC").
		Limit(perPage).
		Offset((page - 1) * perPage).
		Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to retrieve generated images", "error": err.Error()})
		return
	}

	pagination := utils.GetPaginationData(page, totalPages, fmt.Sprintf("/shops/%s/landing-pages/generated-images", shopID))
	pagination.TotalRows = totalRows

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"message":    "Generated images retrieved successfully",
		"data":       rows,
		"pagination": pagination,
	})
}
