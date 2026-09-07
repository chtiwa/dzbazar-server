package controllers

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const maxRefImageBytes = 10 << 20 // 10 MB

func currentUserID(c *gin.Context) *uuid.UUID {
	if u, ok := c.Get("user"); ok {
		if userData, ok := u.(models.User); ok {
			return &userData.ID
		}
	}
	return nil
}

// GenerateAIImage generates one image from a free-text prompt and an
// optional reference photo, for both the first generation and any later
// regenerate — same action, same endpoint. The reference photo is never
// persisted server-side: it lives in memory for this one outbound AI call
// and is discarded when the request returns. The prompt is moderated (via
// services.ModerateImagePrompt) before the paid image-gen call runs, on
// every call, so a regenerate is re-checked exactly like a first generate.
func GenerateAIImage(c *gin.Context) {
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

	model := services.AIImageModel(c.PostForm("model"))
	if model == "" {
		model = services.AIImageModelPro
	}
	if !model.Valid() {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid model"})
		return
	}
	if utf8.RuneCountInString(prompt) > services.MaxImagePromptRunes {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": fmt.Sprintf("prompt must be at most %d characters", services.MaxImagePromptRunes)})
		return
	}

	var ref *services.ReferenceImage
	if fh, ferr := c.FormFile("image"); ferr == nil {
		if fh.Size > maxRefImageBytes {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "reference image must be at most 10 MB"})
			return
		}
		contentType := fh.Header.Get("Content-Type")
		if !strings.HasPrefix(contentType, "image/") {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "only image files are allowed"})
			return
		}
		src, oerr := fh.Open()
		if oerr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "failed to read uploaded image"})
			return
		}
		data, rerr := io.ReadAll(src)
		src.Close()
		if rerr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "failed to read uploaded image"})
			return
		}
		// data is held in memory for the single GenerateToolImage call below
		// and discarded after the response — never written to disk/DB/blob.
		ref = &services.ReferenceImage{Bytes: data, MimeType: contentType}
	}

	if err := services.ModerateImagePrompt(prompt); err != nil {
		var rejected services.PromptRejectedError
		if errors.As(err, &rejected) {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": rejected.Reason, "code": "PROMPT_REJECTED"})
			return
		}
		if errors.Is(err, services.ErrAIUnconfigured) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "Image generation is not configured"})
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": "Failed to verify prompt", "error": err.Error()})
		return
	}

	if err := services.CheckCreditBudget(shopID, services.AIImageModelCredits[model]); err != nil {
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

	image, err := services.GenerateToolImage(prompt, ref, model)
	if err != nil {
		if errors.Is(err, services.ErrAIUnconfigured) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "Image generation is not configured"})
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": "Failed to generate image"})
		return
	}

	webp, err := services.ToWebP(image)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to compress generated image"})
		return
	}

	// Log error but don't fail the response — the merchant already has their
	// image; a failed usage-row write shouldn't cost them the result.
	if err := services.RecordAiImageToolUsage(shopID, currentUserID(c), prompt, string(model)); err != nil {
		fmt.Printf("failed to record AI image tool usage for shop %s: %v\n", shopID, err)
	}

	dataURL := "data:image/webp;base64," + base64.StdEncoding.EncodeToString(webp)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Image generated successfully", "data": dataURL})
}
