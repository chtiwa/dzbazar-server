package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const dashboardCacheTTL = 5 * time.Minute

type TimeCount struct {
	Label string `json:"label"`
	Count int64  `json:"count"`
}

type StatusStat struct {
	Status     string  `json:"status"`
	Count      int64   `json:"count"`
	Percentage float64 `json:"percentage"`
}

type WilayaStat struct {
	Wilaya string `json:"wilaya"`
	Count  int64  `json:"count"`
}

type DashboardData struct {
	Daily            []TimeCount  `json:"daily"`
	Weekly           []TimeCount  `json:"weekly"`
	Monthly          []TimeCount  `json:"monthly"`
	StatusStats      []StatusStat `json:"statusStats"`
	TotalOrders      int64        `json:"totalOrders"`
	WilayaStats      []WilayaStat `json:"wilayaStats"`
	DeliveredRevenue float64      `json:"deliveredRevenue"`
	PendingRevenue   float64      `json:"pendingRevenue"`
	DeliveredOrders  int64        `json:"deliveredOrders"`
	AvgOrderValue    float64      `json:"avgOrderValue"`
	ShippedOrders    int64        `json:"shippedOrders"`
	DeliveryRate     float64      `json:"deliveryRate"`
	ConfirmedOrders  int64        `json:"confirmedOrders"`
	ConfirmationRate float64      `json:"confirmationRate"`
}

func dashboardCacheKey(shopID uuid.UUID) string {
	return fmt.Sprintf("dashboard:orders:%s", shopID)
}

func InvalidateDashboardCache(shopID uuid.UUID) {
	initializers.RClient.Del(initializers.Ctx, dashboardCacheKey(shopID))
}

func GetOrdersDashboard(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	fromStr := c.Query("from")
	toStr := c.Query("to")

	var fromTime, toTime time.Time
	hasDateFilter := fromStr != "" && toStr != ""
	if hasDateFilter {
		fromTime, err = time.Parse("2006-01-02", fromStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid from date"})
			return
		}
		toTime, err = time.Parse("2006-01-02", toStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid to date"})
			return
		}
		toTime = toTime.Add(24 * time.Hour) // make end date inclusive
	}

	var productID uuid.UUID
	if v := c.Query("productId"); v != "" {
		productID, err = uuid.Parse(v)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid product ID"})
			return
		}
	}
	hasProduct := productID != uuid.Nil

	cacheKey := dashboardCacheKey(shopID)

	// Cache hit — only for unfiltered, unscoped (all-time, all-products) requests
	if !hasDateFilter && !hasProduct {
		if cached, err := initializers.RClient.Get(initializers.Ctx, cacheKey).Bytes(); err == nil {
			var data DashboardData
			if json.Unmarshal(cached, &data) == nil {
				c.JSON(http.StatusOK, gin.H{"success": true, "cached": true, "data": data})
				return
			}
		}
	}

	db := initializers.DB
	now := time.Now()

	daily := []TimeCount{}
	dailyQ := db.Table("orders").Where("shop_id = ? AND deleted_at IS NULL AND is_hidden = false AND status <> 'Abandonné'", shopID)
	if hasDateFilter {
		dailyQ = dailyQ.Where("created_at >= ? AND created_at < ?", fromTime, toTime)
	} else {
		dailyQ = dailyQ.Where("created_at >= ?", now.AddDate(0, 0, -30))
	}
	if hasProduct {
		dailyQ = dailyQ.Where(orderContainsProductSQL, productID)
	}
	if err := dailyQ.Select("DATE(created_at)::text AS label, COUNT(*) AS count").
		Group("DATE(created_at)").Order("DATE(created_at) ASC").Scan(&daily).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Error fetching daily stats", "error": err.Error()})
		return
	}

	weekly := []TimeCount{}
	weeklyQ := db.Table("orders").Where("shop_id = ? AND deleted_at IS NULL AND is_hidden = false AND status <> 'Abandonné'", shopID)
	if hasDateFilter {
		weeklyQ = weeklyQ.Where("created_at >= ? AND created_at < ?", fromTime, toTime)
	} else {
		weeklyQ = weeklyQ.Where("created_at >= ?", now.AddDate(0, 0, -7*12))
	}
	if hasProduct {
		weeklyQ = weeklyQ.Where(orderContainsProductSQL, productID)
	}
	if err := weeklyQ.Select("TO_CHAR(DATE_TRUNC('week', created_at), 'IYYY-IW') AS label, COUNT(*) AS count").
		Group("DATE_TRUNC('week', created_at)").Order("DATE_TRUNC('week', created_at) ASC").Scan(&weekly).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Error fetching weekly stats", "error": err.Error()})
		return
	}

	monthly := []TimeCount{}
	monthlyQ := db.Table("orders").Where("shop_id = ? AND deleted_at IS NULL AND is_hidden = false AND status <> 'Abandonné'", shopID)
	if hasDateFilter {
		monthlyQ = monthlyQ.Where("created_at >= ? AND created_at < ?", fromTime, toTime)
	} else {
		monthlyQ = monthlyQ.Where("created_at >= ?", now.AddDate(-1, 0, 0))
	}
	if hasProduct {
		monthlyQ = monthlyQ.Where(orderContainsProductSQL, productID)
	}
	if err := monthlyQ.Select("TO_CHAR(DATE_TRUNC('month', created_at), 'YYYY-MM') AS label, COUNT(*) AS count").
		Group("DATE_TRUNC('month', created_at)").Order("DATE_TRUNC('month', created_at) ASC").Scan(&monthly).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Error fetching monthly stats", "error": err.Error()})
		return
	}

	var totalOrders int64
	totalQ := db.Table("orders").Where("shop_id = ? AND deleted_at IS NULL AND is_hidden = false AND status <> 'Abandonné'", shopID)
	if hasDateFilter {
		totalQ = totalQ.Where("created_at >= ? AND created_at < ?", fromTime, toTime)
	}
	if hasProduct {
		totalQ = totalQ.Where(orderContainsProductSQL, productID)
	}
	if err := totalQ.Count(&totalOrders).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Error counting total orders", "error": err.Error()})
		return
	}

	// Livré = cash actually collected (COD). Pending = still in play, not yet
	// collected and not dead (Annulé/Abandonné).
	var rev struct {
		DeliveredRevenue    float64
		DeliveredNetRevenue float64
		PendingRevenue      float64
		DeliveredOrders     int64
		ShippedOrders       int64
		ConfirmedOrders     int64
	}
	revQ := db.Table("orders").Where("shop_id = ? AND deleted_at IS NULL AND is_hidden = false AND status <> 'Abandonné'", shopID)
	if hasDateFilter {
		revQ = revQ.Where("created_at >= ? AND created_at < ?", fromTime, toTime)
	}
	if hasProduct {
		revQ = revQ.Where(orderContainsProductSQL, productID)
	}

	// Money is order-level shop-wide (COD cash at the door, shipping included), but
	// line-level when scoped to one product: a multi-item order's total_price would
	// credit this product with other products' cash. Shipping is order-level and
	// therefore not attributable to a line, so product-scoped delivered revenue is
	// net by construction (delivered == deliveredNet below).
	deliveredExpr := "total_price"
	netExpr := "total_price - COALESCE(shipping_price, 0)"
	if hasProduct {
		// productID is a parsed uuid.UUID — .String() can only ever emit hex and
		// dashes, so interpolating it as a literal cannot inject. Done as a literal
		// rather than a bind arg to avoid GORM's Select-vs-Where placeholder ordering.
		lineSum := fmt.Sprintf(
			"(SELECT COALESCE(SUM(oi.price * oi.quantity), 0) FROM order_items oi WHERE oi.order_id = orders.id AND oi.deleted_at IS NULL AND oi.product_id = '%s')",
			productID.String(),
		)
		deliveredExpr, netExpr = lineSum, lineSum
	}

	// "was ever confirmed" per the order.status_changed audit trail — same
	// definition as confirmationRatesByProductIDs in productsController.go, not
	// the current order.status (which would miss orders that moved past Confirmé).
	const wasEverConfirmed = `EXISTS (
			SELECT 1 FROM audit_logs al
			WHERE al.target_type = 'Order' AND al.target_id = orders.id
				AND al.action = 'order.status_changed' AND al.metadata::json->>'to' = 'Confirmé'
		)`

	revSelect := fmt.Sprintf(`
			COALESCE(SUM(CASE WHEN status = 'Livré' THEN %s END), 0) AS delivered_revenue,
			COALESCE(SUM(CASE WHEN status = 'Livré' THEN %s END), 0) AS delivered_net_revenue,
			COALESCE(SUM(CASE WHEN status NOT IN ('Livré', 'Annulé', 'Abandonné') THEN %s END), 0) AS pending_revenue,
			COUNT(*) FILTER (WHERE status = 'Livré') AS delivered_orders,
			COUNT(*) FILTER (WHERE is_shipped = true) AS shipped_orders,
			COUNT(*) FILTER (WHERE %s) AS confirmed_orders
		`, deliveredExpr, netExpr, deliveredExpr, wasEverConfirmed)

	if err := revQ.Select(revSelect).Scan(&rev).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Error fetching revenue stats", "error": err.Error()})
		return
	}

	confirmationRate := 0.0
	if totalOrders > 0 {
		confirmationRate = float64(rev.ConfirmedOrders) * 100.0 / float64(totalOrders)
	}
	// AOV is merchandise-only: shipping is passed through to the carrier, so
	// including it inflates the figure by the delivery tarif and makes AOV
	// track wilaya mix (Tamanrasset ships at 1600 DA, Alger at 350) instead of
	// basket size. DeliveredRevenue above deliberately still includes shipping
	// — that one is cash actually collected at the door (COD).
	avgOrderValue := 0.0
	if rev.DeliveredOrders > 0 {
		avgOrderValue = rev.DeliveredNetRevenue / float64(rev.DeliveredOrders)
	}
	// Of orders actually dispatched to a carrier (is_shipped), what fraction
	// arrived. Denominator is shipment attempts, not all orders — orders
	// still stuck pre-shipment (Confirmé, Reporté, ...) never had a delivery
	// attempt, so counting them here would blame delivery for a confirmation
	// problem. is_shipped is written in the same query as status on every
	// ship path (manual PATCH, Osen/ZR/Leopard ship, sync jobs), unlike
	// audit_logs which those carrier flows bypass entirely.
	deliveryRate := 0.0
	if rev.ShippedOrders > 0 {
		deliveryRate = float64(rev.DeliveredOrders) * 100.0 / float64(rev.ShippedOrders)
	}

	statusStats := []StatusStat{}
	statusQ := db.Table("orders").Where("shop_id = ? AND deleted_at IS NULL AND is_hidden = false AND status <> 'Abandonné'", shopID)
	if hasDateFilter {
		statusQ = statusQ.Where("created_at >= ? AND created_at < ?", fromTime, toTime)
	}
	if hasProduct {
		statusQ = statusQ.Where(orderContainsProductSQL, productID)
	}
	if err := statusQ.Select("status, COUNT(*) AS count").Group("status").Scan(&statusStats).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Error fetching status stats", "error": err.Error()})
		return
	}
	for i := range statusStats {
		if totalOrders > 0 {
			statusStats[i].Percentage = float64(statusStats[i].Count) * 100.0 / float64(totalOrders)
		}
	}

	wilayaStats := []WilayaStat{}
	wilayaQ := db.Table("orders").
		Joins("JOIN clients c ON c.id = orders.client_id").
		Where("orders.shop_id = ? AND orders.deleted_at IS NULL AND orders.is_hidden = false AND orders.status <> 'Abandonné' AND c.state != ''", shopID)
	if hasDateFilter {
		wilayaQ = wilayaQ.Where("orders.created_at >= ? AND orders.created_at < ?", fromTime, toTime)
	}
	if hasProduct {
		wilayaQ = wilayaQ.Where(orderContainsProductSQL, productID)
	}
	if err := wilayaQ.Select("c.state AS wilaya, COUNT(*) AS count").
		Group("c.state").Order("count DESC").Scan(&wilayaStats).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Error fetching wilaya stats", "error": err.Error()})
		return
	}

	data := DashboardData{
		Daily:            daily,
		Weekly:           weekly,
		Monthly:          monthly,
		StatusStats:      statusStats,
		TotalOrders:      totalOrders,
		WilayaStats:      wilayaStats,
		DeliveredRevenue: rev.DeliveredRevenue,
		PendingRevenue:   rev.PendingRevenue,
		DeliveredOrders:  rev.DeliveredOrders,
		AvgOrderValue:    avgOrderValue,
		ShippedOrders:    rev.ShippedOrders,
		DeliveryRate:     deliveryRate,
		ConfirmedOrders:  rev.ConfirmedOrders,
		ConfirmationRate: confirmationRate,
	}

	// Store in cache — failure is non-fatal; skip for date-filtered or product-scoped requests
	if !hasDateFilter && !hasProduct {
		if b, err := json.Marshal(data); err == nil {
			initializers.RClient.Set(initializers.Ctx, cacheKey, b, dashboardCacheTTL)
		}
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "cached": false, "data": data})
}
