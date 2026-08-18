package superadmin

import (
	"net/http"
	"strings"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/services"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type OrderWithShop struct {
	models.Order
	ShopName string `json:"shopName"`
	ShopSlug string `json:"shopSlug"`
}

// ListOrders is a read-only cross-tenant order overview, filterable by shop
// and status. Never mutates another tenant's orders.
func ListOrders(c *gin.Context) {
	shopID := strings.TrimSpace(c.Query("shopId"))
	status := strings.TrimSpace(c.Query("status"))
	search := strings.TrimSpace(c.Query("search"))
	page, perPage := parsePageParams(c)

	db := initializers.DB.Model(&models.Order{})

	if shopID != "" {
		db = db.Where("orders.shop_id = ?", shopID)
	}
	if status != "" {
		db = db.Where("orders.status = ?", status)
	}
	// Search by order id (prefix shown in the table), client name, or phone —
	// needs a join since none of those but id live on orders itself. Every
	// column below is table-qualified (not just the joined ones) because an
	// unqualified "created_at"/"id" would be ambiguous once clients is joined
	// in — both tables carry those via BaseModel.
	if search != "" {
		like := "%" + strings.ToLower(search) + "%"
		db = db.Joins("JOIN clients ON clients.id = orders.client_id").
			Where(
				"LOWER(orders.id::text) LIKE ? OR LOWER(clients.full_name) LIKE ? OR clients.phone_number LIKE ? OR clients.phone_number2 LIKE ?",
				like, like, "%"+search+"%", "%"+search+"%",
			)
	}

	order := resolveSort(c, map[string]string{
		"id":         "orders.id",
		"totalPrice": "orders.total_price",
		"status":     "orders.status",
		"created_at": "orders.created_at",
	}, "orders.created_at DESC")

	var totalRows int64
	db.Count(&totalRows)

	var orders []models.Order
	if err := db.Order(order).
		Offset((page - 1) * perPage).Limit(perPage).
		Preload("Client").
		Find(&orders).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to fetch orders", err)
		return
	}

	shopIDs := make([]string, 0, len(orders))
	seen := map[string]bool{}
	for _, o := range orders {
		idStr := o.ShopID.String()
		if !seen[idStr] {
			seen[idStr] = true
			shopIDs = append(shopIDs, idStr)
		}
	}

	shopByID := map[string]models.Shop{}
	if len(shopIDs) > 0 {
		var shops []models.Shop
		initializers.DB.Select("id", "name", "slug").Where("id IN ?", shopIDs).Find(&shops)
		for _, s := range shops {
			shopByID[s.ID.String()] = s
		}
	}

	result := make([]OrderWithShop, 0, len(orders))
	for _, o := range orders {
		shop := shopByID[o.ShopID.String()]
		result = append(result, OrderWithShop{Order: o, ShopName: shop.Name, ShopSlug: shop.Slug})
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"data":       result,
		"pagination": paginationMeta(page, perPage, totalRows),
	})
}

// GetOrder returns full order detail (items, client, shipping) for the
// order-detail view a support agent opens from the orders list. Read-only,
// same cross-tenant scope as ListOrders.
func GetOrder(c *gin.Context) {
	orderID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid order ID"})
		return
	}

	order, err := services.GetOrderDetail(initializers.DB, orderID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Order not found"})
			return
		}
		RespondError(c, http.StatusInternalServerError, "Database error", err)
		return
	}

	var shop models.Shop
	initializers.DB.Select("id", "name", "slug").First(&shop, "id = ?", order.ShopID)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    OrderWithShop{Order: order, ShopName: shop.Name, ShopSlug: shop.Slug},
	})
}
