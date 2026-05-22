package middleware

import (
	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/gin-gonic/gin"
)

func RequireShopAccess(requiredRole string) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, _ := c.Get("user")                 // Populated by your base Auth middleware
		targetShopID := c.GetHeader("X-Shop-ID") // Passed by frontend admin panel

		if targetShopID == "" {
			c.JSON(400, gin.H{"error": "Missing active workspace header"})
			c.Abort()
			return
		}

		var membership models.ShopMember
		err := initializers.DB.Where("shop_id = ? AND user_id = ?", targetShopID, user.(models.User).ID).First(&membership).Error

		if err != nil {
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
