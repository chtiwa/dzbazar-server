package middleware

import (
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

func RequireShopAccess(requiredRole string) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, _ := c.Get("user")                 // Populated by your base Auth middleware
		targetShopID := c.GetHeader("X-Shop-ID") // Passed by frontend admin panel

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
				c.Set("userShopRole", "Owner")
				c.Next()
				return
			}

			c.JSON(403, gin.H{"error": "You do not have permission to access this store workspace"})
			c.Abort()
			return
		}

		// Optional: Enforce authorization roles (e.g. tracking logistics agents)
		if requiredRole == "Owner" && membership.Role != "Owner" {
			c.JSON(403, gin.H{"error": "Action requires Owner privileges"})
			c.Abort()
			return
		}

		// Set the active scope variables for your handlers down the execution chain
		c.Set("activeShopID", targetShopID)
		c.Set("userShopRole", membership.Role)
		c.Next()
	}
}
