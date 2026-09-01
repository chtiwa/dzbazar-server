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
	"gorm.io/gorm"
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
const maxLandingPagePromptLen = 2000

// parseReferenceImages reads the "images" multipart files shared by both the
// create and regenerate handlers: 1-5 image files, content-type checked.
func parseReferenceImages(c *gin.Context) ([]services.ReferenceImage, error) {
	form, err := c.MultipartForm()
	if err != nil {
		return nil, fmt.Errorf("invalid multipart form data")
	}
	files := form.File["images"]
	if len(files) == 0 {
		return nil, fmt.Errorf("at least one product photo is required")
	}
	if len(files) > maxLandingPageSectionRefs {
		return nil, fmt.Errorf("at most %d reference photos are allowed", maxLandingPageSectionRefs)
	}

	refs := make([]services.ReferenceImage, len(files))
	for i, fh := range files {
		contentType := fh.Header.Get("Content-Type")
		if !strings.HasPrefix(contentType, "image/") {
			return nil, fmt.Errorf("only image files are allowed")
		}
		src, err := fh.Open()
		if err != nil {
			return nil, fmt.Errorf("failed to read uploaded image")
		}
		data, err := io.ReadAll(src)
		src.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read uploaded image")
		}
		refs[i] = services.ReferenceImage{Bytes: data, MimeType: contentType}
	}
	return refs, nil
}

// resolveImageModel maps the "pro"/"flash" form value to an AIImageModel,
// defaulting to Pro when omitted.
func resolveImageModel(raw string) (services.AIImageModel, bool) {
	switch raw {
	case "", "pro":
		return services.AIImageModelPro, true
	case "flash":
		return services.AIImageModelFlash, true
	default:
		return "", false
	}
}

func currentUserID(c *gin.Context) *uuid.UUID {
	if u, ok := c.Get("user"); ok {
		if userData, ok := u.(models.User); ok {
			return &userData.ID
		}
	}
	return nil
}

// findShopCampaign loads a campaign scoped to the given shop, writing a 404
// response and returning ok=false if it doesn't belong to that shop.
func findShopCampaign(c *gin.Context, shopID uuid.UUID, campaignID string) (models.LandingPageImageCampaign, bool) {
	var campaign models.LandingPageImageCampaign
	err := initializers.DB.Where("id = ? AND shop_id = ?", campaignID, shopID).First(&campaign).Error
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "campaign not found"})
		return campaign, false
	}
	return campaign, true
}

// GenerateCampaignImage generates one landing-page section image from a
// free-text prompt plus 1-5 reference photos, at the given slot index. Used
// for both the first generation of a slot and any later regenerate — same
// action, same endpoint, matching the pattern already used elsewhere in this
// controller.
func GenerateCampaignImage(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid shop ID"})
		return
	}

	prompt := strings.TrimSpace(c.PostForm("prompt"))
	if prompt == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "prompt is required"})
		return
	}
	if len(prompt) > maxLandingPagePromptLen {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": fmt.Sprintf("prompt must be at most %d characters", maxLandingPagePromptLen)})
		return
	}

	index, err := strconv.Atoi(c.PostForm("index"))
	if err != nil || index < 0 || index >= services.MaxLandingPageImageCount {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": fmt.Sprintf("index must be between 0 and %d", services.MaxLandingPageImageCount-1)})
		return
	}

	model, ok := resolveImageModel(c.PostForm("model"))
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "model must be \"pro\" or \"flash\""})
		return
	}

	refs, err := parseReferenceImages(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}

	if err := services.CheckCreditBudget(shopID, model.CreditCost()); err != nil {
		if errors.Is(err, services.ErrPlanLimitReached) {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "Not enough AI credits. Upgrade your plan for more.",
				"code":    "PLAN_LIMIT_REACHED",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to verify plan limits", "error": err.Error()})
		return
	}

	image, err := services.GenerateLandingPageImage(refs, prompt, model)
	if err != nil {
		if errors.Is(err, services.ErrAIUnconfigured) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "Image generation is not configured"})
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": "Failed to generate image", "error": err.Error()})
		return
	}

	webp, err := services.ToWebP(image)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to compress generated image", "error": err.Error()})
		return
	}

	userID := currentUserID(c)
	campaignIDParam := strings.TrimSpace(c.PostForm("campaignId"))

	var campaignID uuid.UUID
	err = initializers.DB.Transaction(func(tx *gorm.DB) error {
		if campaignIDParam == "" {
			campaign := models.LandingPageImageCampaign{ShopID: shopID, UserID: userID, ImageCount: index + 1}
			if err := tx.Create(&campaign).Error; err != nil {
				return err
			}
			campaignID = campaign.ID
		} else {
			parsed, err := uuid.Parse(campaignIDParam)
			if err != nil {
				return fmt.Errorf("invalid campaign ID")
			}
			var campaign models.LandingPageImageCampaign
			if err := tx.Where("id = ? AND shop_id = ?", parsed, shopID).First(&campaign).Error; err != nil {
				return fmt.Errorf("campaign not found")
			}
			if index+1 > campaign.ImageCount {
				if err := tx.Model(&campaign).Update("image_count", index+1).Error; err != nil {
					return err
				}
			}
			campaignID = campaign.ID
		}

		url := uploadGeneratedImage(shopID, "image/webp", webp)
		usage := models.LandingPageImageGenUsage{ShopID: shopID, UserID: userID, URL: url, CampaignID: &campaignID, ImageIndex: index}
		return tx.Create(&usage).Error
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to save generated image", "error": err.Error()})
		return
	}

	dataURL := "data:image/webp;base64," + base64.StdEncoding.EncodeToString(webp)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Image generated successfully", "data": dataURL, "campaignId": campaignID})
}

// DeleteCampaignImage deletes every generation ever made at a slot index
// (there may be more than one if the slot was regenerated before removal) so
// the slot can be refilled. The B2-stored file is left in place — same
// best-effort, "gallery keeps everything ever generated" behavior as the
// rest of this controller; the image still shows in ListGeneratedImages.
func DeleteCampaignImage(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid shop ID"})
		return
	}

	campaign, ok := findShopCampaign(c, shopID, c.Param("campaignId"))
	if !ok {
		return
	}

	index, err := strconv.Atoi(c.Param("index"))
	if err != nil || index < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid image index"})
		return
	}

	if err := initializers.DB.
		Where("campaign_id = ? AND image_index = ?", campaign.ID, index).
		Delete(&models.LandingPageImageGenUsage{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to delete image", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Image deleted successfully"})
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

// GetCampaignImages returns one image-gen campaign and every image ever
// generated under it (including past regenerate runs), newest first.
func GetCampaignImages(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid shop ID"})
		return
	}

	campaign, ok := findShopCampaign(c, shopID, c.Param("campaignId"))
	if !ok {
		return
	}

	var images []models.LandingPageImageGenUsage
	if err := initializers.DB.Where("campaign_id = ?", campaign.ID).
		Order("image_index ASC, created_at DESC").
		Find(&images).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to retrieve campaign images", "error": err.Error()})
		return
	}
	campaign.Images = images

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Campaign retrieved successfully", "data": campaign})
}
