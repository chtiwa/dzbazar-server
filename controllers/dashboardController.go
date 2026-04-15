package controllers

import (
	"net/http"

	"github.com/chtiwa/lk-parfumo-server/initializers"
	"github.com/gin-gonic/gin"
)

type TimeCount struct {
	Label string `json:"label"`
	Count int64  `json:"count"`
}

type StatusStat struct {
	Status     string  `json:"status"`
	Count      int64   `json:"count"`
	Percentage float64 `json:"percentage"`
}

func GetOrdersDashboard(c *gin.Context) {
	var daily []TimeCount
	var weekly []TimeCount
	var monthly []TimeCount
	var statusStats []StatusStat

	db := initializers.DB

	if err := db.
		Table("orders").
		Where("created_at >= ? AND created_at < ?", "2026-01-01", "2027-01-01").
		Select("DATE(created_at)::text as label, COUNT(*) as count").
		Group("DATE(created_at)").
		Order("DATE(created_at) DESC").
		Scan(&daily).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Error while fetching daily stats", "error": err.Error()})
		return
	}

	if err := db.
		Table("orders").
		Where("created_at >= ? AND created_at < ?", "2026-01-01", "2027-01-01").
		Select("TO_CHAR(DATE_TRUNC('week', created_at), 'IYYY-IW') as label, COUNT(*) as count").
		Group("DATE_TRUNC('week', created_at)").
		Order("DATE_TRUNC('week', created_at) DESC").
		Scan(&weekly).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Error while fetching weekly stats", "error": err.Error()})
		return
	}

	if err := db.
		Table("orders").
		Where("created_at >= ? AND created_at < ?", "2026-01-01", "2027-01-01").
		Select("TO_CHAR(DATE_TRUNC('month', created_at), 'YYYY-MM') as label, COUNT(*) as count").
		Group("DATE_TRUNC('month', created_at)").
		Order("DATE_TRUNC('month', created_at) DESC").
		Scan(&monthly).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Error while fetching monthly stats", "error": err.Error()})
		return
	}

	var totalOrders int64
	if err := db.Table("orders").Count(&totalOrders).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Error while counting total orders", "error": err.Error()})
		return
	}

	if err := db.
		Table("orders").
		Where("created_at >= ? AND created_at < ?", "2026-01-01", "2027-01-01").
		Select("status, COUNT(*) as count").
		Group("status").
		Scan(&statusStats).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Error while fetching status stats", "error": err.Error()})
		return
	}

	for i := range statusStats {
		if totalOrders > 0 {
			statusStats[i].Percentage = float64(statusStats[i].Count) * 100 / float64(totalOrders)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"daily":       daily,
			"weekly":      weekly,
			"monthly":     monthly,
			"statusStats": statusStats,
			"totalOrders": totalOrders,
		},
	})
}
