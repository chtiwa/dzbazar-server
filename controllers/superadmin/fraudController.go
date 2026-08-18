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

// ListFlaggedClients is a read-only cross-tenant view of flagged_clients
// (fbp/ttp ids auto-flagged for a cussword name, or manually banned via
// BanOrderClient), filterable by shop/platform/resolved-state. Same
// read-only reasoning as ListClients/ListOrders — support agents triage
// fraud tickets from here.
func ListFlaggedClients(c *gin.Context) {
	shopID := strings.TrimSpace(c.Query("shopId"))
	platform := strings.TrimSpace(c.Query("platform"))
	page, perPage := parsePageParams(c)

	var resolved *bool
	switch strings.TrimSpace(c.Query("resolved")) {
	case "true":
		v := true
		resolved = &v
	case "false":
		v := false
		resolved = &v
	}

	order := resolveSort(c, map[string]string{
		"platform":   "platform",
		"created_at": "created_at",
	}, "created_at DESC")

	result, totalRows, err := services.ListFlaggedClients(initializers.DB, services.ListFlaggedClientsFilter{
		ShopID:   shopID,
		Platform: platform,
		Resolved: resolved,
	}, order, page, perPage)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to fetch flagged clients", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"data":       result,
		"pagination": paginationMeta(page, perPage, totalRows),
	})
}

// ResolveFlaggedClient dismisses a flag as a false positive. Reversible in
// spirit (the row and its history stay, only resolved_at/resolved_by are
// set) so the frontend gates it behind a plain Yes/No confirmation, same
// reasoning as ToggleClientBan.
func ResolveFlaggedClient(c *gin.Context) {
	flagID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid flag ID"})
		return
	}

	actor, ok := c.Get("user")
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Missing session user"})
		return
	}
	actorUser := actor.(models.User)

	flag, err := services.ResolveFlaggedClient(initializers.DB, flagID, actorUser.ID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Flagged client not found"})
			return
		}
		RespondError(c, http.StatusInternalServerError, "Failed to resolve flag", err)
		return
	}

	utils.LogAudit(c, "flagged_client.resolve", "FlaggedClient", &flag.ID, gin.H{
		"shopId":   flag.ShopID,
		"platform": flag.Platform,
		"clientId": flag.ClientID,
	})

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Flag resolved", "data": flag})
}
