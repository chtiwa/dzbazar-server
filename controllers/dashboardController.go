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
	MaturedShipped   int64        `json:"maturedShipped"`
	MaturedDelivered int64        `json:"maturedDelivered"`
	MaturingOrders   int64        `json:"maturingOrders"`
	ConfirmedOrders  int64        `json:"confirmedOrders"`
	ConfirmationRate float64      `json:"confirmationRate"`
	// Resolved = left "En attente" limbo (same definition as confirmatrices.go).
	// Narrows the denominator to orders that were actually worked, so it isn't
	// dragged down by orders nobody has called yet.
	ResolvedOrders          int64   `json:"resolvedOrders"`
	ResolvedConfirmationRate float64 `json:"resolvedConfirmationRate"`
	// ResolvedShipped/ResolvedDelivered = shipments that reached a terminal
	// state (Livré/Retour), no maturity wait — an order is resolved the
	// instant it lands there, unlike deliveryRate which needs time to judge
	// carriers still in flight. StuckInTransit still uses the maturity
	// buffer: matured (past buffer) but still sitting in a non-terminal
	// status — a failing/slow carrier, distinct from MaturingOrders (too
	// recent to judge yet).
	ResolvedShipped      int64   `json:"resolvedShipped"`
	ResolvedDelivered    int64   `json:"resolvedDeliveredOrders"`
	StuckInTransit       int64   `json:"stuckInTransit"`
	ResolvedDeliveryRate float64 `json:"resolvedDeliveryRate"`
}

// deliveryRateMaturityBuffer: a shipment needs time to actually arrive before
// counting it against the carrier. 3 days covers Alger (1-2d); slower wilayas
// (e.g. Tamanrasset) just stay in "still in transit" a bit longer instead of
// wrongly tanking the rate. See dashboardController deliveryRate comment.
const deliveryRateMaturityBuffer = "3 days"

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

	var deliveryCompanyID uuid.UUID
	if v := c.Query("deliveryCompanyId"); v != "" {
		deliveryCompanyID, err = uuid.Parse(v)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid delivery company ID"})
			return
		}
	}
	hasDeliveryCompany := deliveryCompanyID != uuid.Nil

	cacheKey := dashboardCacheKey(shopID)

	// Cache hit — only for unfiltered, unscoped (all-time, all-products, all-carriers) requests
	if !hasDateFilter && !hasProduct && !hasDeliveryCompany {
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
	if hasDeliveryCompany {
		dailyQ = dailyQ.Where("shipped_via_id = ?", deliveryCompanyID)
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
	if hasDeliveryCompany {
		weeklyQ = weeklyQ.Where("shipped_via_id = ?", deliveryCompanyID)
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
	if hasDeliveryCompany {
		monthlyQ = monthlyQ.Where("shipped_via_id = ?", deliveryCompanyID)
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
	if hasDeliveryCompany {
		totalQ = totalQ.Where("shipped_via_id = ?", deliveryCompanyID)
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
		MaturedShipped      int64
		MaturedDelivered    int64
		MaturedResolved     int64
		ResolvedShipped     int64
		ResolvedDelivered   int64
		ConfirmedOrders     int64
		ResolvedOrders      int64
	}
	revQ := db.Table("orders").Where("shop_id = ? AND deleted_at IS NULL AND is_hidden = false AND status <> 'Abandonné'", shopID)
	if hasDateFilter {
		revQ = revQ.Where("created_at >= ? AND created_at < ?", fromTime, toTime)
	}
	if hasProduct {
		revQ = revQ.Where(orderContainsProductSQL, productID)
	}
	if hasDeliveryCompany {
		revQ = revQ.Where("shipped_via_id = ?", deliveryCompanyID)
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
	// status IN (...) catches carrier-shipped orders: Osen/ZR/Leopard write
	// status = 'Expedié' directly (bypass LogAudit) when shipping, and a
	// shipped/delivered/returned order was necessarily confirmed first even
	// with no audit row. EXISTS alone missed all of those.
	const wasEverConfirmed = `(
			orders.status IN ('Confirmé', 'Expedié', 'Livré', 'Retour') OR EXISTS (
				SELECT 1 FROM audit_logs al
				WHERE al.target_type = 'Order' AND al.target_id = orders.id
					AND al.action = 'order.status_changed' AND al.metadata::json->>'to' = 'Confirmé'
			)
		)`

	revSelect := fmt.Sprintf(`
			COALESCE(SUM(CASE WHEN status = 'Livré' THEN %s END), 0) AS delivered_revenue,
			COALESCE(SUM(CASE WHEN status = 'Livré' THEN %s END), 0) AS delivered_net_revenue,
			COALESCE(SUM(CASE WHEN status NOT IN ('Livré', 'Annulé', 'Retour', 'Abandonné') THEN %s END), 0) AS pending_revenue,
			COUNT(*) FILTER (WHERE status = 'Livré') AS delivered_orders,
			COUNT(*) FILTER (WHERE is_shipped = true) AS shipped_orders,
			COUNT(*) FILTER (WHERE is_shipped = true AND shipped_at <= now() - interval '%s') AS matured_shipped,
			COUNT(*) FILTER (WHERE is_shipped = true AND shipped_at <= now() - interval '%s' AND status = 'Livré') AS matured_delivered,
			COUNT(*) FILTER (WHERE is_shipped = true AND shipped_at <= now() - interval '%s' AND status IN ('Livré', 'Retour')) AS matured_resolved,
			COUNT(*) FILTER (WHERE is_shipped = true AND status IN ('Livré', 'Retour')) AS resolved_shipped,
			COUNT(*) FILTER (WHERE is_shipped = true AND status = 'Livré') AS resolved_delivered,
			COUNT(*) FILTER (WHERE %s) AS confirmed_orders,
			COUNT(*) FILTER (WHERE status <> 'En attente') AS resolved_orders
		`, deliveredExpr, netExpr, deliveredExpr, deliveryRateMaturityBuffer, deliveryRateMaturityBuffer, deliveryRateMaturityBuffer, wasEverConfirmed)

	if err := revQ.Select(revSelect).Scan(&rev).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Error fetching revenue stats", "error": err.Error()})
		return
	}

	confirmationRate := 0.0
	if totalOrders > 0 {
		confirmationRate = float64(rev.ConfirmedOrders) * 100.0 / float64(totalOrders)
	}
	resolvedConfirmationRate := 0.0
	if rev.ResolvedOrders > 0 {
		resolvedConfirmationRate = float64(rev.ConfirmedOrders) * 100.0 / float64(rev.ResolvedOrders)
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
	//
	// Denominator is further restricted to shipments old enough to have
	// plausibly resolved (deliveryRateMaturityBuffer) — a same-day shipment
	// can't be Livré yet, so counting it against the rate before it's had
	// time to arrive makes any "today"/"last 7 days" view look artificially
	// bad. Orders that are still in transit *past* the buffer stay counted
	// (against the rate) rather than excluded — a stuck/failing carrier
	// should show up, not silently vanish from the denominator.
	deliveryRate := 0.0
	if rev.MaturedShipped > 0 {
		deliveryRate = float64(rev.MaturedDelivered) * 100.0 / float64(rev.MaturedShipped)
	}
	maturingOrders := rev.ShippedOrders - rev.MaturedShipped

	// Unlike deliveryRate, no maturity gate here: Livré/Retour is already a
	// terminal outcome the moment it happens, no reason to wait 3 days to
	// count an order that's already resolved. The buffer only matters for
	// judging shipments still in flight (stuckInTransit below).
	resolvedDeliveryRate := 0.0
	if rev.ResolvedShipped > 0 {
		resolvedDeliveryRate = float64(rev.ResolvedDelivered) * 100.0 / float64(rev.ResolvedShipped)
	}
	stuckInTransit := rev.MaturedShipped - rev.MaturedResolved

	statusStats := []StatusStat{}
	statusQ := db.Table("orders").Where("shop_id = ? AND deleted_at IS NULL AND is_hidden = false AND status <> 'Abandonné'", shopID)
	if hasDateFilter {
		statusQ = statusQ.Where("created_at >= ? AND created_at < ?", fromTime, toTime)
	}
	if hasProduct {
		statusQ = statusQ.Where(orderContainsProductSQL, productID)
	}
	if hasDeliveryCompany {
		statusQ = statusQ.Where("shipped_via_id = ?", deliveryCompanyID)
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
	if hasDeliveryCompany {
		wilayaQ = wilayaQ.Where("orders.shipped_via_id = ?", deliveryCompanyID)
	}
	if err := wilayaQ.Select("c.state AS wilaya, COUNT(*) AS count").
		Group("c.state").Order("count DESC").Scan(&wilayaStats).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Error fetching wilaya stats", "error": err.Error()})
		return
	}

	data := DashboardData{
		Daily:                daily,
		Weekly:               weekly,
		Monthly:              monthly,
		StatusStats:          statusStats,
		TotalOrders:          totalOrders,
		WilayaStats:          wilayaStats,
		DeliveredRevenue:     rev.DeliveredRevenue,
		PendingRevenue:       rev.PendingRevenue,
		DeliveredOrders:      rev.DeliveredOrders,
		AvgOrderValue:        avgOrderValue,
		ShippedOrders:        rev.ShippedOrders,
		DeliveryRate:         deliveryRate,
		MaturedShipped:       rev.MaturedShipped,
		MaturedDelivered:     rev.MaturedDelivered,
		MaturingOrders:       maturingOrders,
		ConfirmedOrders:      rev.ConfirmedOrders,
		ConfirmationRate:     confirmationRate,
		ResolvedOrders:           rev.ResolvedOrders,
		ResolvedConfirmationRate: resolvedConfirmationRate,
		ResolvedShipped:      rev.ResolvedShipped,
		ResolvedDelivered:    rev.ResolvedDelivered,
		StuckInTransit:       stuckInTransit,
		ResolvedDeliveryRate: resolvedDeliveryRate,
	}

	// Store in cache — failure is non-fatal; skip for date-filtered or product/carrier-scoped requests
	if !hasDateFilter && !hasProduct && !hasDeliveryCompany {
		if b, err := json.Marshal(data); err == nil {
			initializers.RClient.Set(initializers.Ctx, cacheKey, b, dashboardCacheTTL)
		}
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "cached": false, "data": data})
}

// GetPagePerformance returns views/orders/conversionRate for a single product or landing page,
// optionally scoped to a date range — same views/orders/conversionRate math as the all-time
// figures on the Products and LandingPages list endpoints (see productsController.go), but for
// one entity so it can be date-filtered without touching those cached list responses.
func GetPagePerformance(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}
	pageType := c.Query("type")
	if pageType != "product" && pageType != "landing_page" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "type must be 'product' or 'landing_page'"})
		return
	}

	entityID, err := uuid.Parse(c.Query("entityId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid entity ID"})
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

	viewsQ := initializers.DB.Table("page_visits").
		Joins(entityShopJoin(pageType)).
		Where("page_visits.page_type = ? AND page_visits.entity_id = ? AND entity.shop_id = ?", pageType, entityID, shopID)
	if hasDateFilter {
		viewsQ = viewsQ.Where("page_visits.day >= ? AND page_visits.day < ?", fromTime, toTime)
	}
	var views int64
	if err := viewsQ.Select("COUNT(DISTINCT page_visits.visitor_id)").Row().Scan(&views); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to count views", "error": err.Error()})
		return
	}

	var orders int64
	if pageType == "product" {
		ordersQ := initializers.DB.Table("order_items").
			Joins("JOIN orders ON orders.id = order_items.order_id").
			Joins("JOIN products ON products.id = order_items.product_id").
			Where("order_items.product_id = ? AND products.shop_id = ? AND orders.deleted_at IS NULL AND orders.landing_page_id IS NULL", entityID, shopID)
		if hasDateFilter {
			ordersQ = ordersQ.Where("orders.created_at >= ? AND orders.created_at < ?", fromTime, toTime)
		}
		if err := ordersQ.Select("COUNT(DISTINCT order_items.order_id)").Row().Scan(&orders); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to count orders", "error": err.Error()})
			return
		}
	} else {
		ordersQ := initializers.DB.Table("orders").
			Joins("JOIN landing_pages ON landing_pages.id = orders.landing_page_id").
			Where("orders.landing_page_id = ? AND landing_pages.shop_id = ? AND orders.deleted_at IS NULL", entityID, shopID)
		if hasDateFilter {
			ordersQ = ordersQ.Where("orders.created_at >= ? AND orders.created_at < ?", fromTime, toTime)
		}
		if err := ordersQ.Select("COUNT(*)").Row().Scan(&orders); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to count orders", "error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"views":          views,
			"orders":         orders,
			"conversionRate": conversionRate(orders, views),
		},
	})
}

// entityShopJoin scopes page_visits to the requesting shop via the underlying product/landing_page
// row, since page_visits itself has no shop_id column.
func entityShopJoin(pageType string) string {
	if pageType == "product" {
		return "JOIN products entity ON entity.id = page_visits.entity_id"
	}
	return "JOIN landing_pages entity ON entity.id = page_visits.entity_id"
}
