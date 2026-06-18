package superadmin

import (
	"net/http"
	"os"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const impersonationTTLSeconds = 20 * 60

// StartImpersonation grants the calling super admin Owner-equivalent access
// to one shop for 20 minutes. Always audit-logged. RequireShopAccess /
// RequireAuthentication validate the resulting grant on every subsequent request.
func StartImpersonation(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	var shop models.Shop
	if err := initializers.DB.First(&shop, "id = ?", shopID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Shop not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Database error", "error": err.Error()})
		return
	}

	actor, ok := c.Get("user")
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Missing session user"})
		return
	}
	actorUser := actor.(models.User)

	token := utils.GenerateImpersonationToken(actorUser.ID, shop.ID, impersonationTTLSeconds)
	tokenString, err := token.SignedString([]byte(os.Getenv("JWT_SECRET")))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to issue impersonation token"})
		return
	}

	isProduction := os.Getenv("APP_ENV") == "production"
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("AccessToken", tokenString, impersonationTTLSeconds, "/", "", isProduction, true)

	utils.LogAudit(c, "impersonation.start", "Shop", &shop.ID, gin.H{"name": shop.Name, "slug": shop.Slug})

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Impersonation started",
		"data": gin.H{
			"shop":      shop,
			"expiresIn": impersonationTTLSeconds,
		},
	})
}

// EndImpersonation reissues a normal (non-impersonating) access token for the
// super admin's own identity and audit-logs the exit.
func EndImpersonation(c *gin.Context) {
	actor, ok := c.Get("user")
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Missing session user"})
		return
	}
	actorUser := actor.(models.User)

	shopIDClaim, _ := c.Get("impersonatedShopID")

	token := utils.GenerateToken(actorUser.ID, 60*15, actorUser.Role)
	tokenString, err := token.SignedString([]byte(os.Getenv("JWT_SECRET")))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to end impersonation"})
		return
	}

	isProduction := os.Getenv("APP_ENV") == "production"
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("AccessToken", tokenString, 60*15, "/", "", isProduction, true)

	if shopIDStr, ok := shopIDClaim.(string); ok {
		if shopID, err := uuid.Parse(shopIDStr); err == nil {
			utils.LogAudit(c, "impersonation.end", "Shop", &shopID, nil)
		}
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Impersonation ended"})
}
