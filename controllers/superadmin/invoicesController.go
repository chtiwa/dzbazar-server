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
// invoice approved.
//
// Blocker 8 fix: a same-plan renewal now extends the period from
// max(now, sub.ExpiresAt) + 30d instead of always resetting to now+30d,
// which used to discard any days the merchant had already paid for and not
// used yet. An upgrade (different, pricier plan) keeps the existing
// ExpiresAt — the merchant already paid full price up to that date via
// CreateInvoice's same-plan/expired-sub full-price billing, so the upgrade
// only buys the higher tier's caps, not extra time. A brand new
// subscription (no row yet) still gets a fresh 30-day period from now.
//
// The pending -> approved transition is now a conditional
// `UPDATE ... WHERE status = 'pending'` run inside the same transaction as
// the subscription write, so a double-click (two concurrent approvals of
// the same invoice) can't both succeed: the second UPDATE affects 0 rows
// and the whole transaction is aborted with 409 before either one touches
// the subscription.
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

	now := time.Now()
	alreadyReviewed := false

	err = initializers.DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&models.Invoice{}).
			Where("id = ? AND status = 'pending'", invoiceID).
			Updates(map[string]any{
				"status":      "approved",
				"reviewed_by": actorUser.ID,
				"reviewed_at": now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			alreadyReviewed = true
			return nil
		}

		var sub models.ShopSubscription
		existing := tx.Where("shop_id = ?", invoice.ShopID).First(&sub)
		if existing.Error != nil && existing.Error != gorm.ErrRecordNotFound {
			return existing.Error
		}

		if existing.Error == gorm.ErrRecordNotFound {
			expires := now.AddDate(0, 0, 30)
			sub = models.ShopSubscription{ShopID: invoice.ShopID, PlanID: invoice.PlanID, StartedAt: now, ExpiresAt: &expires}
			return tx.Create(&sub).Error
		}

		expires := renewedExpiry(sub.PlanID, invoice.PlanID, sub.ExpiresAt, now)
		return tx.Model(&sub).Updates(map[string]any{
			"plan_id":                 invoice.PlanID,
			"started_at":              now,
			"expires_at":              expires,
			"expiry_reminder_sent_at": nil,
		}).Error
	})

	if err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to approve invoice", err)
		return
	}
	if alreadyReviewed {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "Invoice already reviewed"})
		return
	}

	utils.LogAudit(c, "invoice.approve", "Invoice", &invoice.ID, gin.H{"shopId": invoice.ShopID, "planId": invoice.PlanID})

	initializers.DB.Preload("Plan").First(&invoice, "id = ?", invoiceID)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Invoice approved", "data": invoice})
}

// renewedExpiry is the pure piece of blocker 8's ApproveInvoice fix: a
// same-plan renewal extends from whichever is later, now or the current
// ExpiresAt, so a merchant who renews early keeps the days they already
// paid for instead of losing them to a flat now+30d reset. An upgrade to a
// different plan keeps the existing ExpiresAt untouched — see ApproveInvoice
// doc comment for why. currentExpiresAt == nil (no-expiry subscription,
// shouldn't happen for a paid plan in practice) is treated as "not later
// than now" so a renewal still produces a normal 30-day period rather than
// staying permanently nil.
func renewedExpiry(currentPlanID, targetPlanID uuid.UUID, currentExpiresAt *time.Time, now time.Time) time.Time {
	if targetPlanID != currentPlanID {
		if currentExpiresAt != nil {
			return *currentExpiresAt
		}
		return now.AddDate(0, 0, 30)
	}

	base := now
	if currentExpiresAt != nil && currentExpiresAt.After(now) {
		base = *currentExpiresAt
	}
	return base.AddDate(0, 0, 30)
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
