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

// Optional persona/brand fields are free text going into an LLM prompt as
// labeled data lines (never as instructions), so a generous length cap is
// enough of a trust-boundary guard here — no need for the stricter
// prompt-injection screening the AI description path uses.
const maxPersonaFieldLen = 200
const maxCustomNotesLen = 500

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

// parsePersonaFields reads the optional buyer-persona / brand-style form
// fields shared by both handlers, trimmed and length-capped.
func parsePersonaFields(c *gin.Context) (age, gender, desires, objections, colors, mood, style, notes string, err error) {
	get := func(key string, max int) (string, error) {
		v := strings.TrimSpace(c.PostForm(key))
		if len(v) > max {
			return "", fmt.Errorf("%s must be at most %d characters", key, max)
		}
		return v, nil
	}
	if age, err = get("ageRange", maxPersonaFieldLen); err != nil {
		return
	}
	if gender, err = get("gender", maxPersonaFieldLen); err != nil {
		return
	}
	if desires, err = get("desires", maxPersonaFieldLen); err != nil {
		return
	}
	if objections, err = get("objections", maxPersonaFieldLen); err != nil {
		return
	}
	if colors, err = get("brandColors", maxPersonaFieldLen); err != nil {
		return
	}
	if mood, err = get("mood", maxPersonaFieldLen); err != nil {
		return
	}
	if style, err = get("referenceStyle", maxPersonaFieldLen); err != nil {
		return
	}
	notes, err = get("customNotes", maxCustomNotesLen)
	return
}

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

	ageRange, gender, desires, objections, brandColors, mood, referenceStyle, customNotes, err := parsePersonaFields(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}

	imageCount, err := strconv.Atoi(c.PostForm("imageCount"))
	if err != nil || imageCount < services.MinLandingPageImageCount || imageCount > services.MaxLandingPageImageCount {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": fmt.Sprintf("imageCount must be between %d and %d", services.MinLandingPageImageCount, services.MaxLandingPageImageCount)})
		return
	}

	refs, err := parseReferenceImages(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}

	if err := services.CheckCreditBudget(shopID, imageCount*services.CreditCostImage); err != nil {
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

	input := services.LandingPageImageSetInput{
		ProductName: productName, Category: category, Benefit: benefit, PainPoint: painPoint, Audience: audience, OfferDetails: offerDetails,
		AgeRange: ageRange, Gender: gender, Desires: desires, Objections: objections,
		BrandColors: brandColors, Mood: mood, ReferenceStyle: referenceStyle, CustomNotes: customNotes,
		Count: imageCount,
	}

	images, err := services.GenerateLandingPageImageSet(refs, input)
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

	campaign := models.LandingPageImageCampaign{
		ShopID: shopID, UserID: userID,
		ProductName: productName, Category: category, PrimaryBenefit: benefit, PainPoint: painPoint, Audience: audience, OfferDetails: offerDetails,
		AgeRange: ageRange, Gender: gender, Desires: desires, Objections: objections,
		BrandColors: brandColors, Mood: mood, ReferenceStyle: referenceStyle, CustomNotes: customNotes,
		ImageCount: imageCount,
	}
	webpImages, campaignID := persistGeneratedImages(c, shopID, userID, images, &campaign, nil)
	if webpImages == nil {
		return // persistGeneratedImages already wrote the error response
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Landing page image set generated successfully", "data": webpImages, "campaignId": campaignID})
}

// persistGeneratedImages converts each generated image to WebP, uploads it,
// and inserts one usage row per image. If newCampaign is non-nil it's
// inserted first and every usage row is tied to it (the campaign row is only
// inserted once generation has already succeeded, so a failed generation
// never leaves an orphan campaign with no images); if existingCampaignID is
// set instead (the regenerate path), usage rows are tied to that campaign
// without inserting a new one. Returns nil data on failure, after already
// writing the JSON error response.
func persistGeneratedImages(c *gin.Context, shopID uuid.UUID, userID *uuid.UUID, images [][]byte, newCampaign *models.LandingPageImageCampaign, existingCampaignID *uuid.UUID) ([]string, *uuid.UUID) {
	webpImages := make([]string, len(images))
	webpBytes := make([][]byte, len(images))
	for i, img := range images {
		webp, err := services.ToWebP(img)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to compress generated image", "error": err.Error()})
			return nil, nil
		}
		webpBytes[i] = webp
		webpImages[i] = "data:image/webp;base64," + base64.StdEncoding.EncodeToString(webp)
	}

	campaignID := existingCampaignID
	usageRows := make([]models.LandingPageImageGenUsage, len(images))
	err := initializers.DB.Transaction(func(tx *gorm.DB) error {
		if newCampaign != nil {
			if err := tx.Create(newCampaign).Error; err != nil {
				return err
			}
			campaignID = &newCampaign.ID
		}
		for i, webp := range webpBytes {
			url := uploadGeneratedImage(shopID, "image/webp", webp)
			usageRows[i] = models.LandingPageImageGenUsage{ShopID: shopID, UserID: userID, URL: url, CampaignID: campaignID, ImageIndex: i}
		}
		return tx.Create(&usageRows).Error
	})
	if err != nil {
		// Generation already succeeded and cost real API budget — log and
		// still return the images rather than fail the response over a
		// bookkeeping write.
		fmt.Printf("failed to record AI image gen usage for shop %s: %v\n", shopID, err)
	}

	return webpImages, campaignID
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

// RegenerateLandingPageImageSet re-runs generation for an existing campaign
// using its stored product/persona/brand inputs. Reference photos are not
// persisted from the original run, so they must be re-uploaded here — the
// smallest option that avoids adding photo storage for a rarely-used path.
// New images are appended as additional usage rows under the same campaign
// (same image_index values as before) rather than overwriting or forking a
// new campaign, since the original images are already paid-for API calls and
// already live in B2.
func RegenerateLandingPageImageSet(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid shop ID"})
		return
	}

	campaign, ok := findShopCampaign(c, shopID, c.Param("campaignId"))
	if !ok {
		return
	}

	refs, err := parseReferenceImages(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}

	if err := services.CheckCreditBudget(shopID, campaign.ImageCount*services.CreditCostImage); err != nil {
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

	input := services.LandingPageImageSetInput{
		ProductName: campaign.ProductName, Category: campaign.Category, Benefit: campaign.PrimaryBenefit,
		PainPoint: campaign.PainPoint, Audience: campaign.Audience, OfferDetails: campaign.OfferDetails,
		AgeRange: campaign.AgeRange, Gender: campaign.Gender, Desires: campaign.Desires, Objections: campaign.Objections,
		BrandColors: campaign.BrandColors, Mood: campaign.Mood, ReferenceStyle: campaign.ReferenceStyle, CustomNotes: campaign.CustomNotes,
		Count: campaign.ImageCount,
	}

	images, err := services.GenerateLandingPageImageSet(refs, input)
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

	webpImages, campaignID := persistGeneratedImages(c, shopID, userID, images, nil, &campaign.ID)
	if webpImages == nil {
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Landing page image set regenerated successfully", "data": webpImages, "campaignId": campaignID})
}
