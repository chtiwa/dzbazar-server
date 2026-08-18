package superadmin

import (
	"net/http"
	"strings"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/services"
	"github.com/chtiwa/dzbazar-server/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ClientWithShop struct {
	models.Client
	ShopName   string `json:"shopName"`
	ShopSlug   string `json:"shopSlug"`
	OrderCount int64  `json:"orderCount"`
}

// ListClients is a read-only cross-tenant client directory, filterable by
// shop and searchable by name/phone. Never mutates another tenant's clients —
// same reasoning as ListOrders.
func ListClients(c *gin.Context) {
	shopID := strings.TrimSpace(c.Query("shopId"))
	search := strings.TrimSpace(c.Query("search"))
	page, perPage := parsePageParams(c)

	db := initializers.DB.Model(&models.Client{})

	if shopID != "" {
		db = db.Where("clients.shop_id = ?", shopID)
	}
	if search != "" {
		like := "%" + strings.ToLower(search) + "%"
		db = db.Where(
			"LOWER(clients.full_name) LIKE ? OR clients.phone_number LIKE ? OR clients.phone_number2 LIKE ?",
			like, "%"+search+"%", "%"+search+"%",
		)
	}

	order := resolveSort(c, map[string]string{
		"fullName":    "clients.full_name",
		"phoneNumber": "clients.phone_number",
		"city":        "clients.city",
		"banned":      "clients.banned",
		"created_at":  "clients.created_at",
	}, "clients.created_at DESC")

	var totalRows int64
	db.Count(&totalRows)

	var clients []models.Client
	if err := db.Order(order).
		Offset((page - 1) * perPage).Limit(perPage).
		Find(&clients).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to fetch clients", err)
		return
	}

	clientIDs := make([]string, 0, len(clients))
	shopIDs := make([]string, 0, len(clients))
	seenShop := map[string]bool{}
	for _, cl := range clients {
		clientIDs = append(clientIDs, cl.ID.String())
		idStr := cl.ShopID.String()
		if !seenShop[idStr] {
			seenShop[idStr] = true
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

	// Order count per client, one grouped query for the whole page instead of
	// N+1 — same batching principle as the shop lookup above.
	orderCountByClient := map[string]int64{}
	if len(clientIDs) > 0 {
		var rows []struct {
			ClientID string
			Count    int64
		}
		// client_id cast to text explicitly — scanning a uuid column straight
		// into a Go string is driver-dependent, the cast makes it unambiguous.
		initializers.DB.Model(&models.Order{}).
			Select("client_id::text as client_id, COUNT(*) as count").
			Where("client_id IN ?", clientIDs).
			Group("client_id").
			Scan(&rows)
		for _, r := range rows {
			orderCountByClient[r.ClientID] = r.Count
		}
	}

	result := make([]ClientWithShop, 0, len(clients))
	for _, cl := range clients {
		shop := shopByID[cl.ShopID.String()]
		result = append(result, ClientWithShop{
			Client:     cl,
			ShopName:   shop.Name,
			ShopSlug:   shop.Slug,
			OrderCount: orderCountByClient[cl.ID.String()],
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"data":       result,
		"pagination": paginationMeta(page, perPage, totalRows),
	})
}

// GetClient returns full client detail plus a lightweight order history for
// the client-detail view a support agent opens from the clients list.
// Read-only, same cross-tenant scope as GetOrder.
func GetClient(c *gin.Context) {
	clientID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid client ID"})
		return
	}

	client, err := services.GetClientDetail(initializers.DB, clientID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Client not found"})
			return
		}
		RespondError(c, http.StatusInternalServerError, "Database error", err)
		return
	}

	var shop models.Shop
	initializers.DB.Select("id", "name", "slug").First(&shop, "id = ?", client.ShopID)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": ClientWithShop{
			Client:   client,
			ShopName: shop.Name,
			ShopSlug: shop.Slug,
		},
	})
}

type ToggleClientBanInput struct {
	Banned bool `json:"banned"`
}

// ToggleClientBan flips a client's phone-based ban flag. Reversible (unlike
// a delete), so the frontend gates it behind a plain Yes/No confirmation
// rather than a typed confirmPhrase — same reasoning as UpdateUserStatus.
func ToggleClientBan(c *gin.Context) {
	clientID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid client ID"})
		return
	}

	var body ToggleClientBanInput
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondError(c, http.StatusBadRequest, "Validation failed", err)
		return
	}

	client, err := services.SetClientBanned(initializers.DB, clientID, body.Banned)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Client not found"})
			return
		}
		RespondError(c, http.StatusInternalServerError, "Failed to update client ban status", err)
		return
	}

	action := "client.ban"
	if !body.Banned {
		action = "client.unban"
	}
	utils.LogAudit(c, action, "Client", &client.ID, gin.H{"phoneNumber": client.PhoneNumber})

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Client ban status updated", "data": client})
}
