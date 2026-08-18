package controllers

import (
	"net/http"

	"github.com/chtiwa/dzbazar-server/services"
	"github.com/gin-gonic/gin"
)

// GetFeatureFlagPublic exposes a single flag's on/off state — just the bit a
// frontend needs to grey out a "create" action, no label/description, and no
// auth required. Same default-enabled behavior as services.IsFeatureEnabled
// for an unknown key, so this can never leak which keys exist.
func GetFeatureFlagPublic(c *gin.Context) {
	key := c.Param("key")
	enabled := services.IsFeatureEnabled(key)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"key": key, "isEnabled": enabled}})
}
