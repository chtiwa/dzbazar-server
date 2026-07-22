package controllers

import (
	"net/http"
	"time"

	"github.com/chtiwa/dzbazar-server/services"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// GetConfirmationRates returns the "taux de confirmation" leaderboard for
// every confirmatrice of this shop, optionally scoped to a [from, to) window.
func GetConfirmationRates(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid or missing Shop ID"})
		return
	}

	var from, to *time.Time
	if fromStr := c.Query("from"); fromStr != "" {
		if parsed, parseErr := time.Parse("2006-01-02", fromStr); parseErr == nil {
			from = &parsed
		}
	}
	if toStr := c.Query("to"); toStr != "" {
		if parsed, parseErr := time.Parse("2006-01-02", toStr); parseErr == nil {
			next := parsed.AddDate(0, 0, 1)
			to = &next
		}
	}

	rates, err := services.ConfirmationRates(shopID, from, to)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Error while computing confirmation rates", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Confirmation rates retrieved successfully", "data": rates})
}
