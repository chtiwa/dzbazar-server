package middleware

import (
	"net/http"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// GetShopMembership reports whether the user is a member of the shop, and
// returns the membership row if so. Shared by RequireShopAccess and the
// websocket handshake — both need the same tenant-isolation check.
func GetShopMembership(userID uuid.UUID, shopID string) (models.ShopMember, bool) {
	var membership models.ShopMember
	err := initializers.DB.Where("shop_id = ? AND user_id = ?", shopID, userID).First(&membership).Error
	return membership, err == nil
}

// roleCan reports whether role may perform action.
// ponytail: perms in code; add shop_roles boolean columns only if shops need custom permissions.
func roleCan(role, action string) bool {
	switch action {
	case "delete":
		return role == "owner"
	default:
		return role == "owner" || role == "moderator"
	}
}

// RequireShopPermission checks the shop role (set by RequireShopAccess) against action.
// Must run after RequireShopAccess.
func RequireShopPermission(action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, _ := c.Get("userShopRole")
		if !roleCan(role.(string), action) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "You do not have permission to perform this action"})
			return
		}
		c.Next()
	}
}

// RequireShopAccess verifies the user is a member of the shop in X-Shop-ID.
// Pass allowed roles to restrict further (empty = any shop member is fine).
func RequireShopAccess(allowedRoles ...string) gin.HandlerFunc {
	allowedMap := map[string]bool{}
	for _, r := range allowedRoles {
		allowedMap[r] = true
	}

	return func(c *gin.Context) {
		user, _ := c.Get("user")                 // Populated by RequireAuthentication
		targetShopID := c.GetHeader("X-Shop-ID") // Passed by the frontend admin panel

		if targetShopID == "" {
			c.JSON(400, gin.H{"error": "Missing active workspace header"})
			c.Abort()
			return
		}

		membership, ok := GetShopMembership(user.(models.User).ID, targetShopID)

		if !ok {
			// Fall back to an explicit, time-boxed impersonation grant minted by
			// POST /v1/super-admin/shops/:shopId/impersonate — scoped to exactly
			// one shop, never a wildcard across all shops.
			isImpersonating, _ := c.Get("isImpersonating")
			impersonatedShopID, _ := c.Get("impersonatedShopID")
			if isImpersonating == true && impersonatedShopID == targetShopID {
				c.Set("activeShopID", targetShopID)
				c.Set("userShopRole", "owner")
				c.Next()
				return
			}

			c.JSON(403, gin.H{"error": "You do not have permission to access this store workspace"})
			c.Abort()
			return
		}

		if len(allowedRoles) > 0 && !allowedMap[membership.Role] {
			c.JSON(403, gin.H{"error": "Action requires elevated privileges"})
			c.Abort()
			return
		}

		c.Set("activeShopID", targetShopID)
		c.Set("userShopRole", membership.Role)
		c.Next()
	}
}
