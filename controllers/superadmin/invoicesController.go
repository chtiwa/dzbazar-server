package superadmin

import (
	"net/http"
	"strings"
	"time"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ListInvoices is the queue a super admin works from — merchant Redot
// payment proofs land here as "pending", same shape as ListPlanSwitchRequests.
func ListInvoices(c *gin.Context) {
	status := strings.TrimSpace(c.Query("status"))
	page, perPage := parsePageParams(c)

	db := initializers.DB.Model(&models.Invoice{}).Preload("Plan")
	if status != "" {
		db = db.Where("status = ?", status)
	}

	order := resolveSort(c, map[string]string{
		"status":     "status",
		"created_at": "created_at",
	}, "created_at DESC")

	var totalRows int64
	db.Count(&totalRows)

	var invoices []models.Invoice
	if err := db.Order(order).
		Offset((page - 1) * perPage).Limit(perPage).
		Find(&invoices).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to fetch invoices", err)
		return
	}

	shopIDs := make([]string, 0, len(invoices))
	seen := map[string]bool{}
	for _, inv := range invoices {
		idStr := inv.ShopID.String()
		if !seen[idStr] {
			seen[idStr] = true
			shopIDs = append(shopIDs, idStr)
		}
	}

	shopByID := map[string]models.Shop{}
	if len(shopIDs) > 0 {
		var shops []models.Shop
		initializers.DB.Select("id", "name", "slug").Where("id IN ?", shopIDs).Find(&shops)
		for _, sh := range shops {
			shopByID[sh.ID.String()] = sh
		}
	}

	type invoiceWithShop struct {
		models.Invoice
		ShopName string `json:"shopName"`
		ShopSlug string `json:"shopSlug"`
	}

	result := make([]invoiceWithShop, 0, len(invoices))
	for _, inv := range invoices {
		shop := shopByID[inv.ShopID.String()]
		result = append(result, invoiceWithShop{Invoice: inv, ShopName: shop.Name, ShopSlug: shop.Slug})
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"data":       result,
		"pagination": paginationMeta(page, perPage, totalRows),
	})
}

// ApproveInvoice applies the invoice's plan to the shop's subscription —
// same upsert-by-shop-id logic as ApprovePlanSwitchRequest — then marks the
// invoice approved. A fresh 30-day period starts now, same rule paid plan
// switches already use.
func ApproveInvoice(c *gin.Context) {
	invoiceID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid invoice ID"})
		return
	}

	actor, ok := c.Get("user")
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Missing session user"})
		return
	}
	actorUser := actor.(models.User)

	var invoice models.Invoice
	if err := initializers.DB.First(&invoice, "id = ?", invoiceID).Error; err != nil {
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

	start := time.Now()
	expires := start.AddDate(0, 0, 30)

	err = initializers.DB.Transaction(func(tx *gorm.DB) error {
		var sub models.ShopSubscription
		existing := tx.Where("shop_id = ?", invoice.ShopID).First(&sub)
		if existing.Error != nil && existing.Error != gorm.ErrRecordNotFound {
			return existing.Error
		}

		if existing.Error == gorm.ErrRecordNotFound {
			sub = models.ShopSubscription{ShopID: invoice.ShopID, PlanID: invoice.PlanID, StartedAt: start, ExpiresAt: &expires}
			if err := tx.Create(&sub).Error; err != nil {
				return err
			}
		} else if err := tx.Model(&sub).Updates(map[string]any{
			"plan_id":                 invoice.PlanID,
			"started_at":              start,
			"expires_at":              expires,
			"expiry_reminder_sent_at": nil,
		}).Error; err != nil {
			return err
		}

		now := time.Now()
		return tx.Model(&invoice).Updates(map[string]any{
			"status":      "approved",
			"reviewed_by": actorUser.ID,
			"reviewed_at": now,
		}).Error
	})

	if err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to approve invoice", err)
		return
	}

	utils.LogAudit(c, "invoice.approve", "Invoice", &invoice.ID, gin.H{"shopId": invoice.ShopID, "planId": invoice.PlanID})

	initializers.DB.Preload("Plan").First(&invoice, "id = ?", invoiceID)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Invoice approved", "data": invoice})
}

// RejectInvoice (denied) leaves the shop's current subscription untouched.
func RejectInvoice(c *gin.Context) {
	invoiceID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid invoice ID"})
		return
	}

	actor, ok := c.Get("user")
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Missing session user"})
		return
	}
	actorUser := actor.(models.User)

	var invoice models.Invoice
	if err := initializers.DB.First(&invoice, "id = ?", invoiceID).Error; err != nil {
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

	now := time.Now()
	if err := initializers.DB.Model(&invoice).Updates(map[string]any{
		"status":      "denied",
		"reviewed_by": actorUser.ID,
		"reviewed_at": now,
	}).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to reject invoice", err)
		return
	}

	utils.LogAudit(c, "invoice.reject", "Invoice", &invoice.ID, gin.H{"shopId": invoice.ShopID, "planId": invoice.PlanID})

	initializers.DB.Preload("Plan").First(&invoice, "id = ?", invoiceID)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Invoice denied", "data": invoice})
}
