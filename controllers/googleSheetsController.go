package controllers

import (
	"log"
	"net/http"
	"time"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/services"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// sheetsParams validates :shopId and :kind (kind optional for the list route).
func sheetsParams(c *gin.Context, needKind bool) (uuid.UUID, string, bool) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return uuid.Nil, "", false
	}
	kind := c.Param("kind")
	if needKind && services.SheetCatalog(kind) == nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid sheet kind"})
		return uuid.Nil, "", false
	}
	return shopID, kind, true
}

func GetGoogleSheets(c *gin.Context) {
	shopID, _, ok := sheetsParams(c, false)
	if !ok {
		return
	}
	var rows []models.GoogleSheetsIntegration
	if err := initializers.DB.Where("shop_id = ?", shopID).Find(&rows).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to load Google Sheets", err)
		return
	}
	integrations := map[string]*models.GoogleSheetsIntegration{services.SheetKindOrders: nil, services.SheetKindAbandoned: nil}
	emails := map[string]string{}
	for i := range rows {
		integrations[rows[i].Kind] = &rows[i]
	}
	for kind, integ := range integrations {
		emails[kind] = services.SheetsEmailFor(integ)
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "OK", "data": gin.H{
		"emails": emails,
		"catalog": gin.H{
			services.SheetKindOrders:    services.SheetCatalog(services.SheetKindOrders),
			services.SheetKindAbandoned: services.SheetCatalog(services.SheetKindAbandoned),
		},
		"integrations": integrations,
	}})
}

func SaveGoogleSheet(c *gin.Context) {
	shopID, kind, ok := sheetsParams(c, true)
	if !ok {
		return
	}
	var body services.SheetIntegrationInput
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondError(c, http.StatusBadRequest, "Invalid request body", err)
		return
	}
	integ, err := services.SaveSheetIntegration(shopID, kind, body)
	if err != nil {
		log.Printf("%s %s: save google sheet failed: %v", c.Request.Method, c.Request.URL.Path, err)
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Could not save Google Sheet", "code": services.SheetsErrorCode(err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Google Sheet saved", "data": integ})
}

func DisconnectGoogleSheet(c *gin.Context) {
	shopID, kind, ok := sheetsParams(c, true)
	if !ok {
		return
	}
	if err := initializers.DB.Where("shop_id = ? AND kind = ?", shopID, kind).Delete(&models.GoogleSheetsIntegration{}).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to disconnect", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Google Sheet disconnected"})
}

// TestGoogleSheet re-runs the live access check against the stored target.
func TestGoogleSheet(c *gin.Context) {
	shopID, kind, ok := sheetsParams(c, true)
	if !ok {
		return
	}
	var integ models.GoogleSheetsIntegration
	if err := initializers.DB.Where("shop_id = ? AND kind = ?", shopID, kind).First(&integ).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "No Google Sheet connected"})
		return
	}
	svc, err := services.SheetsClientFor(&integ)
	if err == nil {
		err = services.WriteHeaderRow(svc, integ.SpreadsheetID, integ.SheetName, services.SheetHeaders(integ.Columns), false)
	}
	if err != nil {
		log.Printf("%s %s: google sheet test failed: %v", c.Request.Method, c.Request.URL.Path, err)
		integ.LastError = services.SheetsErrorCode(err)
		initializers.DB.Save(&integ)
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "Connection test failed", "code": integ.LastError, "data": integ})
		return
	}
	now := time.Now()
	integ.LastSyncedAt = &now
	integ.LastError = ""
	initializers.DB.Save(&integ)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Connection OK", "code": "", "data": integ})
}
