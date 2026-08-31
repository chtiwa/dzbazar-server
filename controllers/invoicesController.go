package controllers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// CreateInvoiceInput ties the invoice to a real Plan row — same shape as
// plansController.SubscribeInput — so approval can upgrade the shop's
// ShopSubscription the same way ApprovePlanSwitchRequest does.
type CreateInvoiceInput struct {
	PlanID string `json:"planId" binding:"required"`
}

// transferFeeRate is Redot's cut on every manual payment, added on top of
// the plan (or upgrade-diff) amount. ponytail: const, add a settings column
// if this ever needs to change without a redeploy.
const transferFeeRate = 0.002

// CreateInvoice files a pending invoice for one of the paid plans. Amount is
// taken from the plan's own price — never trust a client-supplied amount for
// a money field. If the shop already has an active paid subscription and is
// jumping to a higher-priced plan, only the difference is billed (e.g.
// Starter -> Growth bills Growth.Price - Starter.Price, not Growth.Price in
// full). A 0.2% transfer fee is added on top either way. Downgrades aren't
// supported through this flow.
func CreateInvoice(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	var body CreateInvoiceInput
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Validation failed", "error": err.Error()})
		return
	}

	planID, err := uuid.Parse(body.PlanID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid plan ID"})
		return
	}

	var plan models.Plan
	if err := initializers.DB.First(&plan, "id = ? AND is_active = true", planID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Plan not found or inactive"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Database error", "error": err.Error()})
		return
	}

	var existingPending models.Invoice
	err = initializers.DB.Where("shop_id = ? AND status = 'pending'", shopID).First(&existingPending).Error
	if err == nil {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "An invoice is already pending approval"})
		return
	}
	if err != gorm.ErrRecordNotFound {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Database error", "error": err.Error()})
		return
	}

	billedAmount := plan.Price
	var sub models.ShopSubscription
	subErr := initializers.DB.Preload("Plan").Where("shop_id = ?", shopID).First(&sub).Error
	if subErr != nil && subErr != gorm.ErrRecordNotFound {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Database error", "error": subErr.Error()})
		return
	}
	if subErr == nil && sub.Plan.Price > 0 {
		if plan.Price < sub.Plan.Price {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Downgrading plans isn't supported here — contact support"})
			return
		}
		billedAmount = plan.Price - sub.Plan.Price
	}
	billedAmount += billedAmount * transferFeeRate

	invoice := models.Invoice{ShopID: shopID, PlanID: planID, Amount: billedAmount, PaymentMethod: "redot", Status: "pending"}
	if err := initializers.DB.Create(&invoice).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to create invoice", "error": err.Error()})
		return
	}

	utils.LogAudit(c, "invoice.create", "Invoice", &invoice.ID, gin.H{"shopId": shopID, "planId": planID, "amount": invoice.Amount})

	initializers.DB.Preload("Plan").First(&invoice, "id = ?", invoice.ID)
	c.JSON(http.StatusCreated, gin.H{"success": true, "message": "Invoice created — upload your Redot payment screenshot next", "data": invoice})
}

// UploadInvoiceProof attaches the merchant's payment screenshot to a pending
// invoice. Same content-type sniff / seek-reset / PutObject sequence used by
// CreateShop's logo upload — this codebase deliberately keeps that inline
// per-controller rather than sharing a helper (see server/CLAUDE.md).
func UploadInvoiceProof(c *gin.Context) {
	invoiceID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid invoice ID"})
		return
	}

	var invoice models.Invoice
	if err := initializers.DB.First(&invoice, "id = ?", invoiceID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Invoice not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Database error", "error": err.Error()})
		return
	}
	if invoice.Status != "pending" {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "Invoice already reviewed"})
		return
	}

	file, err := c.FormFile("screenshot")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Payment screenshot is required"})
		return
	}

	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Failed to open uploaded screenshot", "error": err.Error()})
		return
	}
	defer src.Close()

	buffer := make([]byte, 512)
	n, readErr := src.Read(buffer)
	if readErr != nil && readErr != io.EOF {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Failed to read uploaded screenshot", "error": readErr.Error()})
		return
	}

	contentType := http.DetectContentType(buffer[:n])
	if !strings.HasPrefix(contentType, "image/") {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Only image files are allowed for the payment screenshot"})
		return
	}

	seeker, ok := src.(io.Seeker)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to process uploaded screenshot stream"})
		return
	}
	if _, seekErr := seeker.Seek(0, io.SeekStart); seekErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to process uploaded screenshot", "error": seekErr.Error()})
		return
	}

	cleanFileName := filepath.Base(file.Filename)
	key := fmt.Sprintf("uploads/invoices/%s/%d_%s", invoice.ShopID, time.Now().UnixNano(), cleanFileName)

	bucketName := os.Getenv("B2_BUCKET_NAME")
	b2Region := os.Getenv("B2_REGION")
	b2PublicBaseURL := strings.TrimSpace(os.Getenv("B2_PUBLIC_BASE_URL"))

	_, putErr := initializers.S3Client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket:        aws.String(bucketName),
		Key:           aws.String(key),
		Body:          src,
		ContentType:   aws.String(contentType),
		ContentLength: aws.Int64(file.Size),
	})
	if putErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to upload payment screenshot", "error": putErr.Error()})
		return
	}

	var proofURL string
	if b2PublicBaseURL != "" {
		proofURL = fmt.Sprintf("%s/%s", strings.TrimRight(b2PublicBaseURL, "/"), key)
	} else {
		proofURL = fmt.Sprintf("https://%s.s3.%s.backblazeb2.com/%s", bucketName, b2Region, key)
	}

	if err := initializers.DB.Model(&invoice).Update("proof_screenshot_url", proofURL).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to save screenshot reference", "error": err.Error()})
		return
	}

	utils.LogAudit(c, "invoice.upload_proof", "Invoice", &invoice.ID, gin.H{"shopId": invoice.ShopID})

	initializers.DB.Preload("Plan").First(&invoice, "id = ?", invoiceID)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Payment screenshot uploaded", "data": invoice})
}

// ListMyInvoices returns a shop's invoice history, newest first.
func ListMyInvoices(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	var invoices []models.Invoice
	if err := initializers.DB.Preload("Plan").Where("shop_id = ?", shopID).Order("created_at DESC").Find(&invoices).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to fetch invoices", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": invoices})
}
