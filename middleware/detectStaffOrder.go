package middleware

import (
	"github.com/chtiwa/dzbazar-server/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const staffOrderContextKey = "isStaffOrder"

// DetectStaffOrder marks the request as staff-originated when the AccessToken
// cookie belongs to an actual member of the :shopId shop. Order creation is a
// public, unauthenticated endpoint (anonymous checkout must keep working), so
// this never aborts — it only flags the request so the anti-spam guards below
// (IP/phone rate limit, per-browser cooldown) can exempt logged-in staff
// instead of blocking them the same way they block anonymous spam.
func DetectStaffOrder(c *gin.Context) {
	shopID := c.Param("shopId")
	accessTokenString, err := c.Cookie("AccessToken")
	if err == nil && shopID != "" {
		if _, claims, err := utils.ParseJWT(accessTokenString); err == nil {
			if sub, ok := claims["sub"].(string); ok {
				if userID, err := uuid.Parse(sub); err == nil {
					if _, ok := GetShopMembership(userID, shopID); ok {
						c.Set(staffOrderContextKey, true)
					}
				}
			}
		}
	}
	c.Next()
}

func IsStaffOrder(c *gin.Context) bool {
	v, _ := c.Get(staffOrderContextKey)
	b, _ := v.(bool)
	return b
}
