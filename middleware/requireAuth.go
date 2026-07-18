package middleware

import (
	"net/http"
	"os"
	"time"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/utils"
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

	// Helper to determine if cookies should be secure
	isProduction := os.Getenv("APP_ENV") == "production"

	// 1. Try Access Token first
	if accessErr == nil {
		_, claims, err := utils.ParseJWT(accessTokenString)
		if err == nil {
			// SAFELY extract claims to prevent panics
			exp, expOk := claims["exp"].(float64)
			sub, subOk := claims["sub"].(string)

			if expOk && subOk && float64(time.Now().Unix()) < exp &&
				!utils.IsTokenRevoked(claims) && !utils.IsTokenBeforeRevokeAll(sub, claims) {
				id, uuidErr := uuid.Parse(sub)
				if uuidErr == nil {
					var user models.User
					// Safe GORM query
					if result := initializers.DB.Where("id = ?", id).First(&user); result.Error == nil {
						if user.IsSuspended {
							c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
								"success": false,
								"message": "This account has been suspended",
							})
							return
						}

						c.Set("user", user)
						c.Set("role", user.Role)

						// Impersonation grant: only honored if the underlying
						// user is still a super admin right now — demoting a
						// super admin immediately kills any outstanding token.
						if impersonating, _ := claims["impersonating"].(bool); impersonating && user.PlatformRole == "super_admin" {
							if shopIDStr, ok := claims["shopId"].(string); ok {
								c.Set("role", "owner")
								c.Set("isImpersonating", true)
								c.Set("impersonatedShopID", shopIDStr)
							}
						}

						c.Next()
						return
					}
				}
			}
		}
	}

	// 2. Access token missing/invalid/expired, try Refresh Token
	if refreshErr == nil {
		_, claims, err := utils.ParseJWT(refreshTokenString)
		if err == nil {
			// SAFELY extract claims
			exp, expOk := claims["exp"].(float64)
			sub, subOk := claims["sub"].(string)

			if expOk && subOk && float64(time.Now().Unix()) < exp &&
				!utils.IsTokenRevoked(claims) && !utils.IsTokenBeforeRevokeAll(sub, claims) {
				id, uuidErr := uuid.Parse(sub)
				if uuidErr == nil {
					var user models.User
					if result := initializers.DB.Where("id = ?", id).First(&user); result.Error == nil {
						if user.IsSuspended {
							c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
								"success": false,
								"message": "This account has been suspended",
							})
							return
						}

						// Generate a new access token
						accessToken := utils.GenerateToken(user.ID, 60*15, user.Role)
						newAccessTokenString, err := accessToken.SignedString([]byte(os.Getenv("JWT_SECRET")))
						if err != nil {
							c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
								"success": false,
								"message": "Failed to generate new access token",
							})
							return
						}

						// Set new access token as a cookie (Secure flag updated)
						c.SetSameSite(http.SameSiteLaxMode)
						c.SetCookie("AccessToken", newAccessTokenString, 60*15, "/", "", isProduction, true)

						c.Set("user", user)
						c.Set("role", user.Role)
						c.Next()
						return
					}
				}
			}
		}
	}

	// 3. Both failed (or user deleted from DB while token was valid)
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
		"success": false,
		"message": "Session expired, please log in again",
	})
}
