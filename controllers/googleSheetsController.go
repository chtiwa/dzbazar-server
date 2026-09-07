package controllers

import (
	"net/http"
	"strings"
	"time"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/services"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type ConnectGoogleSheetsInput struct {
	ServiceAccountJSON string `json:"serviceAccountJson" binding:"required"`
	SpreadsheetID       string `json:"spreadsheetId" binding:"required"`
	SheetName           string `json:"sheetName"`
}

type UpdateGoogleSheetsInput struct {
	ServiceAccountJSON string `json:"serviceAccountJson"`
	SpreadsheetID       string `json:"spreadsheetId"`
	SheetName           string `json:"sheetName"`
	IsActive            *bool  `json:"isActive"`
}

// sheetsStatusResponse mirrors GoogleSheetsIntegration but never echoes the
// raw service-account key back — HasCredentials reports presence only, same
// spirit as Pixel.HasAccessToken.
type sheetsStatusResponse struct {
	ID             uuid.UUID `json:"id"`
	ShopID         uuid.UUID `json:"shopId"`
	HasCredentials bool      `json:"hasCredentials"`
	SpreadsheetID  string    `json:"spreadsheetId"`
	SheetName      string    `json:"sheetName"`
	IsActive       bool      `json:"isActive"`
	LastSyncedAt   *string   `json:"lastSyncedAt"`
	LastError      string    `json:"lastError"`
}

// sheetsAccessErrorMessage picks a user-facing message matching the actual
// EnsureHeaderRow failure — "not shared" is only one of three causes seen in
// practice (API not enabled on the GCP project, transient network failure
// reaching Google, actual sharing/ID mistake).
func sheetsAccessErrorMessage(err error) string {
	msg := err.Error()
	if strings.Contains(msg, "accessNotConfigured") {
		return "Google Sheets API is not enabled on this service account's Google Cloud project. Enable it in Google Cloud Console, wait a minute, then retry."
	}
	if strings.Contains(msg, "network error reaching Google") {
		return "Could not reach Google right now. This is usually transient — retry in a moment."
	}
	if strings.Contains(msg, "not found in the sheet") {
		return "That tab name doesn't exist in the spreadsheet. Check the tab at the bottom of the sheet (new sheets default to \"Sheet1\", not \"Orders\") and match it exactly."
	}
	return "Could not access the sheet. Make sure it's shared with the service account email and the spreadsheet ID is correct."
}

func toSheetsStatusResponse(in models.GoogleSheetsIntegration) sheetsStatusResponse {
	var lastSynced *string
	if in.LastSyncedAt != nil {
		s := in.LastSyncedAt.Format(time.RFC3339)
		lastSynced = &s
	}
	return sheetsStatusResponse{
		ID:             in.ID,
		ShopID:         in.ShopID,
		HasCredentials: in.ServiceAccountJSON != "",
		SpreadsheetID:  in.SpreadsheetID,
		SheetName:      in.SheetName,
		IsActive:       in.IsActive,
		LastSyncedAt:   lastSynced,
		LastError:      in.LastError,
	}
}

// ConnectGoogleSheets creates the shop's Google Sheets integration. The
// service-account key and spreadsheet ID are validated live against the
// Google Sheets API (EnsureHeaderRow) before persisting — an invalid key or
// a sheet not shared with the service account fails here, not silently at
// the first order.
func ConnectGoogleSheets(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	var body ConnectGoogleSheetsInput
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid request body", "error": err.Error()})
		return
	}

	sheetName := strings.TrimSpace(body.SheetName)
	if sheetName == "" {
		sheetName = "Orders"
	}
	spreadsheetID := strings.TrimSpace(body.SpreadsheetID)
	serviceAccountJSON := strings.TrimSpace(body.ServiceAccountJSON)

	svc, err := services.NewSheetsClient(serviceAccountJSON)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid service account credentials", "error": err.Error()})
		return
	}
	if err := services.EnsureHeaderRow(svc, spreadsheetID, sheetName); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": sheetsAccessErrorMessage(err), "error": err.Error()})
		return
	}

	integration := models.GoogleSheetsIntegration{
		ShopID:             shopID,
		ServiceAccountJSON: serviceAccountJSON,
		SpreadsheetID:      spreadsheetID,
		SheetName:          sheetName,
		IsActive:           true,
	}

	if err := initializers.DB.Create(&integration).Error; err != nil {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "Failed to create integration. It may already exist for this shop.", "error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"success": true, "message": "Google Sheets connected successfully", "data": toSheetsStatusResponse(integration)})
}

// UpdateGoogleSheetsCredentials replaces the key/sheet target and/or flips
// IsActive. Re-validates against the live API whenever credentials or the
// sheet target change (same as connect); toggling only IsActive with no
// other fields skips re-validation.
func UpdateGoogleSheetsCredentials(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	var integration models.GoogleSheetsIntegration
	if err := initializers.DB.Where("shop_id = ?", shopID).First(&integration).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "No Google Sheets integration found for this shop"})
		return
	}

	var body UpdateGoogleSheetsInput
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid request body", "error": err.Error()})
		return
	}

	credentialsChanged := false
	if strings.TrimSpace(body.ServiceAccountJSON) != "" {
		integration.ServiceAccountJSON = strings.TrimSpace(body.ServiceAccountJSON)
		credentialsChanged = true
	}
	if strings.TrimSpace(body.SpreadsheetID) != "" {
		integration.SpreadsheetID = strings.TrimSpace(body.SpreadsheetID)
		credentialsChanged = true
	}
	if strings.TrimSpace(body.SheetName) != "" {
		integration.SheetName = strings.TrimSpace(body.SheetName)
		credentialsChanged = true
	}

	if credentialsChanged {
		svc, err := services.NewSheetsClient(integration.ServiceAccountJSON)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid service account credentials", "error": err.Error()})
			return
		}
		if err := services.EnsureHeaderRow(svc, integration.SpreadsheetID, integration.SheetName); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": sheetsAccessErrorMessage(err), "error": err.Error()})
			return
		}
		integration.LastError = ""
	}

	if body.IsActive != nil {
		integration.IsActive = *body.IsActive
	}

	if err := initializers.DB.Save(&integration).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to update integration", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Google Sheets integration updated", "data": toSheetsStatusResponse(integration)})
}

// DisconnectGoogleSheets removes the shop's integration entirely.
func DisconnectGoogleSheets(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	if err := initializers.DB.Where("shop_id = ?", shopID).Delete(&models.GoogleSheetsIntegration{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to disconnect", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Google Sheets disconnected"})
}

// GetGoogleSheetsStatus returns the shop's integration status, or a
// success:true with data:null if none exists (not an error — "not
// connected" is a normal state for a settings page to render).
func GetGoogleSheetsStatus(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	var integration models.GoogleSheetsIntegration
	if err := initializers.DB.Where("shop_id = ?", shopID).First(&integration).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "No integration connected", "data": nil})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "OK", "data": toSheetsStatusResponse(integration)})
}

// TestGoogleSheetsConnection re-runs the live EnsureHeaderRow check against
// stored credentials, without changing them, and updates LastSyncedAt /
// LastError so the admin UI can show fresh status.
func TestGoogleSheetsConnection(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	var integration models.GoogleSheetsIntegration
	if err := initializers.DB.Where("shop_id = ?", shopID).First(&integration).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "No Google Sheets integration found for this shop"})
		return
	}

	svc, err := services.NewSheetsClient(integration.ServiceAccountJSON)
	if err == nil {
		err = services.EnsureHeaderRow(svc, integration.SpreadsheetID, integration.SheetName)
	}

	if err != nil {
		integration.LastError = err.Error()
		initializers.DB.Save(&integration)
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "Connection test failed", "error": err.Error(), "data": toSheetsStatusResponse(integration)})
		return
	}

	now := time.Now()
	integration.LastSyncedAt = &now
	integration.LastError = ""
	initializers.DB.Save(&integration)

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Connection OK", "data": toSheetsStatusResponse(integration)})
}
