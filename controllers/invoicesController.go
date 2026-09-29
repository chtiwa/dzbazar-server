package controllers

import (
	"context"
	"fmt"
	"io"
	"math"
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

// ErrDowngradeNotSupported is returned by billedPlanAmount when the target
// plan is cheaper than the shop's current active plan — downgrades aren't
// supported through the invoice flow.
var ErrDowngradeNotSupported = gorm.ErrInvalidData

// billedPlanAmount computes what a shop must pay (before the transfer fee)
// to move onto targetPlan, given its current subscription state. Pure
// function, no DB/time.Now() access, so it's table-testable:
//   - no current subscription, or the current one is expired -> full
//     targetPlan.Price (blocker 8: an expired sub used to be treated as
//     still "current", which let a lapsed paying merchant renew for free).
//   - same plan (renewal) -> full targetPlan.Price (used to bill 0, since
//     the old code only ever billed a diff and Price - Price = 0).
//   - true upgrade within an active period -> prorated
//     (targetPlan.Price - currentPlan.Price) * remainingDays / 30, rounded
//     to the cent, never negative.
//   - cheaper target plan while the current one is still active ->
//     ErrDowngradeNotSupported (unchanged from the pre-existing behavior).
//
// remainingDays is days left until currentPlan's period ends (0 if already
// expired or unknown); it is the caller's job to compute it from
// sub.ExpiresAt and clamp it at 0.
func billedPlanAmount(targetPlan models.Plan, currentPlan models.Plan, hasActiveSub bool, remainingDays int) (float64, error) {
	if !hasActiveSub || currentPlan.Price == 0 {
		return targetPlan.Price, nil
	}
	if targetPlan.ID == currentPlan.ID {
		return targetPlan.Price, nil
	}
	if targetPlan.Price < currentPlan.Price {
		return 0, ErrDowngradeNotSupported
	}

	diff := targetPlan.Price - currentPlan.Price
	prorated := diff * float64(remainingDays) / 30
	if prorated < 0 {
		prorated = 0
	}
	return roundToCents(prorated), nil
}

func roundToCents(amount float64) float64 {
	return math.Round(amount*100) / 100
}

// CreateInvoice files a pending invoice for one of the paid plans. Amount is
// taken from the plan's own price — never trust a client-supplied amount for
// a money field. See billedPlanAmount for the exact billing rules (same-plan
// renewal and expired subscriptions bill full price; a true upgrade within
// an active period bills the prorated difference). A 0.2% transfer fee is
// added on top either way. Downgrades aren't supported through this flow.
func CreateInvoice(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	var body CreateInvoiceInput
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondError(c, http.StatusBadRequest, "Validation failed", err)
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
		RespondError(c, http.StatusInternalServerError, "Database error", err)
		return
	}

	var existingPending models.Invoice
	err = initializers.DB.Where("shop_id = ? AND status = 'pending'", shopID).First(&existingPending).Error
	if err == nil {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "An invoice is already pending approval"})
		return
	}
	if err != gorm.ErrRecordNotFound {
		RespondError(c, http.StatusInternalServerError, "Database error", err)
		return
	}

	var sub models.ShopSubscription
	subErr := initializers.DB.Preload("Plan").Where("shop_id = ?", shopID).First(&sub).Error
	if subErr != nil && subErr != gorm.ErrRecordNotFound {
		RespondError(c, http.StatusInternalServerError, "Database error", subErr)
		return
	}

	hasActiveSub := subErr == nil && (sub.ExpiresAt == nil || sub.ExpiresAt.After(time.Now()))
	remainingDays := 0
	if hasActiveSub && sub.ExpiresAt != nil {
		remainingDays = int(math.Ceil(time.Until(*sub.ExpiresAt).Hours() / 24))
		if remainingDays < 0 {
			remainingDays = 0
		}
	}

	billedAmount, err := billedPlanAmount(plan, sub.Plan, hasActiveSub, remainingDays)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Downgrading plans isn't supported here — contact support"})
		return
	}
	billedAmount += billedAmount * transferFeeRate

	invoice := models.Invoice{ShopID: shopID, PlanID: planID, Amount: billedAmount, PaymentMethod: "redot", Status: "pending"}
	if err := initializers.DB.Create(&invoice).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to create invoice", err)
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
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	invoiceID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid invoice ID"})
		return
	}

	var invoice models.Invoice
	if err := initializers.DB.Where("id = ? AND shop_id = ?", invoiceID, shopID).First(&invoice).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Invoice not found"})
			return
		}
		RespondError(c, http.StatusInternalServerError, "Database error", err)
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
		RespondError(c, http.StatusBadRequest, "Failed to open uploaded screenshot", err)
		return
	}
	defer src.Close()

	buffer := make([]byte, 512)
	n, readErr := src.Read(buffer)
	if readErr != nil && readErr != io.EOF {
		RespondError(c, http.StatusBadRequest, "Failed to read uploaded screenshot", readErr)
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
		RespondError(c, http.StatusInternalServerError, "Failed to process uploaded screenshot", seekErr)
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
		RespondError(c, http.StatusInternalServerError, "Failed to upload payment screenshot", putErr)
		return
	}

	var proofURL string
	if b2PublicBaseURL != "" {
		proofURL = fmt.Sprintf("%s/%s", strings.TrimRight(b2PublicBaseURL, "/"), key)
	} else {
		proofURL = fmt.Sprintf("https://%s.s3.%s.backblazeb2.com/%s", bucketName, b2Region, key)
	}

	if err := initializers.DB.Model(&invoice).Update("proof_screenshot_url", proofURL).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to save screenshot reference", err)
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
		RespondError(c, http.StatusInternalServerError, "Failed to fetch invoices", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": invoices})
}
