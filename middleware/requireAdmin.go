package middleware

import (
	"fmt"
	"net/http"

	"github.com/chtiwa/landing-page-server/models"
	"github.com/gin-gonic/gin"
)

func RequireAdmin(c *gin.Context) {
	user, ok := c.Get("user")

	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
	}

	userData, ok := user.(models.User)
	fmt.Println(userData)

	if !ok || userData.Role != "Admin" {
		c.AbortWithStatus(http.StatusUnauthorized)
	}

	c.Next()
}
