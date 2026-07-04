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

type ProductConfirmationRate struct {
	ProductID        string  `json:"productId"`
	ProductName      string  `json:"productName"`
	TotalOrders      int64   `json:"totalOrders"`
	ConfirmedOrders  int64   `json:"confirmedOrders"`
	ConfirmationRate float64 `json:"confirmationRate"`
}

type WilayaStat struct {
	Wilaya string `json:"wilaya"`
	Count  int64  `json:"count"`
}

type DashboardData struct {
	Daily             []TimeCount              `json:"daily"`
	Weekly            []TimeCount              `json:"weekly"`
	Monthly           []TimeCount              `json:"monthly"`
	StatusStats       []StatusStat             `json:"statusStats"`
	TotalOrders       int64                    `json:"totalOrders"`
	ConfirmationRates []ProductConfirmationRate `json:"confirmationRates"`
	WilayaStats       []WilayaStat             `json:"wilayaStats"`
	DeliveredRevenue  float64                  `json:"deliveredRevenue"`
	PendingRevenue    float64                  `json:"pendingRevenue"`
	DeliveredOrders   int64                    `json:"deliveredOrders"`
	AvgOrderValue     float64                  `json:"avgOrderValue"`
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

	cacheKey := dashboardCacheKey(shopID)

	// Cache hit
	if cached, err := initializers.RClient.Get(initializers.Ctx, cacheKey).Bytes(); err == nil {
		var data DashboardData
		if json.Unmarshal(cached, &data) == nil {
			c.JSON(http.StatusOK, gin.H{"success": true, "cached": true, "data": data})
			return
		}
	}

	db := initializers.DB
	now := time.Now()

	daily := []TimeCount{}
	if err := db.
		Table("orders").
		Where("shop_id = ? AND deleted_at IS NULL AND created_at >= ?", shopID, now.AddDate(0, 0, -30)).
		Select("DATE(created_at)::text AS label, COUNT(*) AS count").
		Group("DATE(created_at)").
		Order("DATE(created_at) ASC").
		Scan(&daily).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Error fetching daily stats", "error": err.Error()})
		return
	}

	weekly := []TimeCount{}
	if err := db.
		Table("orders").
		Where("shop_id = ? AND deleted_at IS NULL AND created_at >= ?", shopID, now.AddDate(0, 0, -7*12)).
		Select("TO_CHAR(DATE_TRUNC('week', created_at), 'IYYY-IW') AS label, COUNT(*) AS count").
		Group("DATE_TRUNC('week', created_at)").
		Order("DATE_TRUNC('week', created_at) ASC").
		Scan(&weekly).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Error fetching weekly stats", "error": err.Error()})
		return
	}

	monthly := []TimeCount{}
	if err := db.
		Table("orders").
		Where("shop_id = ? AND deleted_at IS NULL AND created_at >= ?", shopID, now.AddDate(-1, 0, 0)).
		Select("TO_CHAR(DATE_TRUNC('month', created_at), 'YYYY-MM') AS label, COUNT(*) AS count").
		Group("DATE_TRUNC('month', created_at)").
		Order("DATE_TRUNC('month', created_at) ASC").
		Scan(&monthly).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Error fetching monthly stats", "error": err.Error()})
		return
	}

	var totalOrders int64
	if err := db.Table("orders").
		Where("shop_id = ? AND deleted_at IS NULL", shopID).
		Count(&totalOrders).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Error counting total orders", "error": err.Error()})
		return
	}

	// Livré = cash actually collected (COD). Pending = still in play, not yet
	// collected and not dead (Annulé/Abandonné).
	var rev struct {
		DeliveredRevenue float64
		PendingRevenue   float64
		DeliveredOrders  int64
	}
	if err := db.Table("orders").
		Where("shop_id = ? AND deleted_at IS NULL", shopID).
		Select(`
			COALESCE(SUM(CASE WHEN status = 'Livré' THEN total_price END), 0) AS delivered_revenue,
			COALESCE(SUM(CASE WHEN status NOT IN ('Livré', 'Annulé', 'Abandonné') THEN total_price END), 0) AS pending_revenue,
			COUNT(*) FILTER (WHERE status = 'Livré') AS delivered_orders
		`).
		Scan(&rev).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Error fetching revenue stats", "error": err.Error()})
		return
	}
	avgOrderValue := 0.0
	if rev.DeliveredOrders > 0 {
		avgOrderValue = rev.DeliveredRevenue / float64(rev.DeliveredOrders)
	}

	statusStats := []StatusStat{}
	if err := db.
		Table("orders").
		Where("shop_id = ? AND deleted_at IS NULL", shopID).
		Select("status, COUNT(*) AS count").
		Group("status").
		Scan(&statusStats).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Error fetching status stats", "error": err.Error()})
		return
	}
	for i := range statusStats {
		if totalOrders > 0 {
			statusStats[i].Percentage = float64(statusStats[i].Count) * 100.0 / float64(totalOrders)
		}
	}

	// Pool = En attente, Ne répond pas 1/2/3, Reporté, Annulé, Confirmé.
	// Excluded: Abandonné, Expédié, Livré, Retour.
	confirmationRates := []ProductConfirmationRate{}
	if err := db.
		Table("order_items oi").
		Joins("JOIN orders o ON o.id = oi.order_id").
		Joins("JOIN products p ON p.id = oi.product_id").
		Where("o.shop_id = ? AND o.deleted_at IS NULL AND p.deleted_at IS NULL", shopID).
		Select(`
			p.id::text AS product_id,
			p.title AS product_name,
			COUNT(DISTINCT CASE WHEN o.status NOT IN ('Abandonné', 'Expédié', 'Livré', 'Retour') THEN oi.order_id END) AS total_orders,
			COUNT(DISTINCT CASE WHEN o.status = 'Confirmé' THEN oi.order_id END) AS confirmed_orders,
			ROUND(
				COUNT(DISTINCT CASE WHEN o.status = 'Confirmé' THEN oi.order_id END) * 100.0
				/ NULLIF(COUNT(DISTINCT CASE WHEN o.status NOT IN ('Abandonné', 'Expédié', 'Livré', 'Retour') THEN oi.order_id END), 0),
			2) AS confirmation_rate
		`).
		Group("p.id, p.title").
		Order("total_orders DESC").
		Scan(&confirmationRates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Error fetching confirmation rates", "error": err.Error()})
		return
	}

	wilayaStats := []WilayaStat{}
	if err := db.
		Table("orders o").
		Joins("JOIN clients c ON c.id = o.client_id").
		Where("o.shop_id = ? AND o.deleted_at IS NULL AND c.state != ''", shopID).
		Select("c.state AS wilaya, COUNT(*) AS count").
		Group("c.state").
		Order("count DESC").
		Scan(&wilayaStats).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Error fetching wilaya stats", "error": err.Error()})
		return
	}

	data := DashboardData{
		Daily:             daily,
		Weekly:            weekly,
		Monthly:           monthly,
		StatusStats:       statusStats,
		TotalOrders:       totalOrders,
		ConfirmationRates: confirmationRates,
		WilayaStats:       wilayaStats,
		DeliveredRevenue:  rev.DeliveredRevenue,
		PendingRevenue:    rev.PendingRevenue,
		DeliveredOrders:   rev.DeliveredOrders,
		AvgOrderValue:     avgOrderValue,
	}

	// Store in cache — failure is non-fatal
	if b, err := json.Marshal(data); err == nil {
		initializers.RClient.Set(initializers.Ctx, cacheKey, b, dashboardCacheTTL)
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "cached": false, "data": data})
}
