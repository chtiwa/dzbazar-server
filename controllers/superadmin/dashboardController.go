package superadmin

import (
	"net/http"
	"time"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/gin-gonic/gin"
)

// GetKPIs returns platform-wide counters for the Super Admin dashboard.
// revenueThisMonth is an estimate (sum of active subscriptions' plan price),
// not a real collected-payments figure — there is no payment gateway wired up yet.
func GetKPIs(c *gin.Context) {
	var totalShops, activeShops, totalUsers, activeSubscriptions, ordersToday, openTickets int64
	var estimatedMonthlyRevenue float64

	initializers.DB.Model(&models.Shop{}).Count(&totalShops)
	initializers.DB.Model(&models.Shop{}).Where("is_active = true").Count(&activeShops)
	initializers.DB.Model(&models.User{}).Count(&totalUsers)

	now := time.Now()
	initializers.DB.Model(&models.ShopSubscription{}).
		Where("expires_at IS NULL OR expires_at > ?", now).
		Count(&activeSubscriptions)

	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	initializers.DB.Model(&models.Order{}).Where("created_at >= ?", startOfDay).Count(&ordersToday)

	initializers.DB.Model(&models.SupportTicket{}).
		Where("status IN ?", []string{"open", "pending"}).
		Count(&openTickets)

	initializers.DB.Model(&models.ShopSubscription{}).
		Joins("JOIN plans ON plans.id = shop_subscriptions.plan_id").
		Where("shop_subscriptions.expires_at IS NULL OR shop_subscriptions.expires_at > ?", now).
		Select("COALESCE(SUM(plans.price), 0)").
		Scan(&estimatedMonthlyRevenue)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"totalShops":              totalShops,
			"activeShops":             activeShops,
			"totalUsers":              totalUsers,
			"activeSubscriptions":     activeSubscriptions,
			"ordersToday":             ordersToday,
			"openTickets":             openTickets,
			"estimatedMonthlyRevenue": estimatedMonthlyRevenue,
		},
	})
}
