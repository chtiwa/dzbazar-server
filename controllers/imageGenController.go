package controllers

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// GenerateLandingPageImage turns a text prompt into a PNG image the admin
// frontend attaches to a landing page like any other uploaded file — this
// endpoint only returns image bytes, it does not touch storage itself.
func GenerateLandingPageImage(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid shop ID"})
		return
	}

	var body struct {
		Prompt string `json:"prompt"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.Prompt) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "prompt is required"})
		return
	}

	if err := services.CheckLandingPageImageGenLimit(shopID); err != nil {
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

	png, err := services.GenerateLandingPageImage(strings.TrimSpace(body.Prompt))
	if err != nil {
		if errors.Is(err, services.ErrAIUnconfigured) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "Image generation is not configured"})
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": "Failed to generate image", "error": err.Error()})
		return
	}

	var userID *uuid.UUID
	if u, ok := c.Get("user"); ok {
		if userData, ok := u.(models.User); ok {
			userID = &userData.ID
		}
	}
	usage := models.LandingPageImageGenUsage{ShopID: shopID, UserID: userID}
	if err := initializers.DB.Create(&usage).Error; err != nil {
		// Non-fatal: the merchant already got their image, losing the usage
		// row only means one uncounted call, not a failed request.
		fmt.Printf("failed to record AI image gen usage for shop %s: %v\n", shopID, err)
	}

	c.Data(http.StatusOK, "image/png", png)
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
	if productName == "" || category == "" || benefit == "" || painPoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "productName, category, primaryBenefit and painPoint are required"})
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

	const sectionCount = 5
	if err := services.CheckLandingPageImageGenBudget(shopID, sectionCount); err != nil {
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

	images, err := services.GenerateLandingPageImageSet(refs, productName, category, benefit, painPoint)
	if err != nil {
		if errors.Is(err, services.ErrAIUnconfigured) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "Image generation is not configured"})
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": "Failed to generate image set", "error": err.Error()})
		return
	}

	webpImages := make([]string, len(images))
	for i, img := range images {
		webp, err := services.ToWebP(img)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to compress generated image", "error": err.Error()})
			return
		}
		webpImages[i] = "data:image/webp;base64," + base64.StdEncoding.EncodeToString(webp)
	}

	var userID *uuid.UUID
	if u, ok := c.Get("user"); ok {
		if userData, ok := u.(models.User); ok {
			userID = &userData.ID
		}
	}
	usageRows := make([]models.LandingPageImageGenUsage, sectionCount)
	for i := range usageRows {
		usageRows[i] = models.LandingPageImageGenUsage{ShopID: shopID, UserID: userID}
	}
	if err := initializers.DB.Create(&usageRows).Error; err != nil {
		fmt.Printf("failed to record AI image gen usage for shop %s: %v\n", shopID, err)
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Landing page image set generated successfully", "data": webpImages})
}
