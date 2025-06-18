package middleware

import (
	"net/http"

	"github.com/chtiwa/herbs-store-client/models"
	"github.com/gin-gonic/gin"
)

func RequireAdmin(c *gin.Context) {
	user, ok := c.Get("user")

	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	userData, ok := user.(models.User)

	if !ok || userData.Role != "Admin" {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	c.Next()
}
