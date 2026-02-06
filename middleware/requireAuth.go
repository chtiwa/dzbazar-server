package middleware

import (
	"net/http"
	"os"
	"time"

	"github.com/chtiwa/lk-parfumo-server/initializers"
	"github.com/chtiwa/lk-parfumo-server/models"
	"github.com/chtiwa/lk-parfumo-server/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func RequireAuthentication(c *gin.Context) {
	accessTokenString, accessErr := c.Cookie("AccessToken")
	refreshTokenString, refreshErr := c.Cookie("RefreshToken")

	if accessErr != nil && refreshErr != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Authentication required",
		})
		return
	}

	if accessErr == nil {
		// Validate access token
		_, claims, err := utils.ParseJWT(accessTokenString)
		if err == nil && float64(time.Now().Unix()) < claims["exp"].(float64) {
			var user models.User
			id, _ := uuid.Parse(claims["sub"].(string))
			if result := initializers.DB.First(&user, id); result.Error == nil {
			c.Set("user", user)
				c.Set("role", user.Role)
				c.Next()
				return
			}
		}
	}

	// Access token invalid, try refresh token
	if refreshErr == nil {
		// Validate refresh token
		_, claims, err := utils.ParseJWT(refreshTokenString)
		if err == nil && float64(time.Now().Unix()) < claims["exp"].(float64) {
			var user models.User
			id, _ := uuid.Parse(claims["sub"].(string))
			if result := initializers.DB.First(&user, id); result.Error == nil {
				// Generate a new access token
				accessToken := utils.GenerateToken(user.ID, 60*15, user.Role) // 15 minutes
				accessTokenString, err := accessToken.SignedString([]byte(os.Getenv("JWT_SECRET")))
				if err != nil {
					c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "message": "Failed to generate new access token"})
					return
				}

				// Set new access token as a cookie
				c.SetSameSite(http.SameSiteLaxMode)
				c.SetCookie("AccessToken", accessTokenString, 60*15, "/", "", false, true)

				c.Set("user", user)
				c.Set("role", user.Role)
				c.Next()
				return
			}
		}
	}

	// Authentication failed
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
		"success": false,
		"message": "Session expired, please log in again",
	})
}
