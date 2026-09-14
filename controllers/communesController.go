package controllers

import (
	"net/http"
	"strconv"

	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/services"
	"github.com/gin-gonic/gin"
)

// ListCommunes returns communes, optionally filtered to one wilaya via
// ?wilayaId=. Public/no-auth: the storefront checkout form needs this with
// no session, same reasoning as the public bureaux/delivery-rates routes.
func ListCommunes(c *gin.Context) {
	communes, err := services.GetCommunes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to fetch communes"})
		return
	}

	if wilayaIDParam := c.Query("wilayaId"); wilayaIDParam != "" {
		wilayaID, err := strconv.Atoi(wilayaIDParam)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid wilayaId"})
			return
		}
		filtered := make([]models.Commune, 0)
		for _, commune := range communes {
			if commune.WilayaID == wilayaID {
				filtered = append(filtered, commune)
			}
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": filtered})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": communes})
}
