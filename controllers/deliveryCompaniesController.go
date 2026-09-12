package controllers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/services"
	"github.com/chtiwa/dzbazar-server/utils"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// Available delivery companies (global, admin-managed)
// ---------------------------------------------------------------------------

type CreateAvailableDeliveryCompanyInput struct {
	Name string `form:"name" binding:"required"`
	URL  string `form:"url" binding:"required"`
}

type UpdateAvailableDeliveryCompanyInput struct {
	Name     *string `form:"name"`
	URL      *string `form:"url"`
	IsActive *bool   `form:"isActive"`
}

func GetAvailableDeliveryCompanies(c *gin.Context) {
	var companies []models.AvailableDeliveryCompany
	if err := initializers.DB.Preload("Image").Where("is_active = ?", true).Find(&companies).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to fetch available delivery companies",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": companies})
}

// ListAllAvailableDeliveryCompanies is the super-admin variant — returns both
// active and inactive entries so operators can re-enable a disabled carrier.
func ListAllAvailableDeliveryCompanies(c *gin.Context) {
	var companies []models.AvailableDeliveryCompany
	if err := initializers.DB.Preload("Image").Find(&companies).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to fetch available delivery companies",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": companies})
}

func CreateAvailableDeliveryCompany(c *gin.Context) {
	var body CreateAvailableDeliveryCompanyInput
	if err := c.ShouldBindWith(&body, binding.FormMultipart); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Validation failed", "error": err.Error()})
		return
	}

	uploadedImageURL, uploadErr := uploadAvailableDeliveryCompanyImage(c)
	if uploadErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": uploadErr.Error()})
		return
	}

	var company models.AvailableDeliveryCompany

	err := initializers.DB.Transaction(func(tx *gorm.DB) error {
		company = models.AvailableDeliveryCompany{
			Name: strings.TrimSpace(body.Name),
			URL:  strings.TrimSpace(body.URL),
		}

		if err := tx.Omit("Image").Create(&company).Error; err != nil {
			return err
		}

		if uploadedImageURL != "" {
			img := models.AvailableDeliveryCompanyImage{
				AvailableDeliveryCompanyID: company.ID,
				URL:                        uploadedImageURL,
			}
			if err := tx.Create(&img).Error; err != nil {
				return err
			}
			company.Image = &img
		}

		return nil
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to create available delivery company",
			"error":   err.Error(),
		})
		return
	}

	utils.LogAudit(c, "available_delivery_company.create", "AvailableDeliveryCompany", &company.ID, gin.H{
		"name":     company.Name,
		"url":      company.URL,
		"hasImage": company.Image != nil,
	})

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "Available delivery company created successfully",
		"data":    company,
	})
}

func UpdateAvailableDeliveryCompany(c *gin.Context) {
	companyID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid delivery company ID"})
		return
	}

	var body UpdateAvailableDeliveryCompanyInput
	if err := c.ShouldBindWith(&body, binding.FormMultipart); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Validation failed", "error": err.Error()})
		return
	}

	var company models.AvailableDeliveryCompany
	if err := initializers.DB.Preload("Image").First(&company, "id = ?", companyID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Delivery company not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Database error", "error": err.Error()})
		return
	}

	uploadedImageURL, uploadErr := uploadAvailableDeliveryCompanyImage(c)
	if uploadErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": uploadErr.Error()})
		return
	}

	updates := map[string]any{}
	if body.Name != nil {
		updates["name"] = strings.TrimSpace(*body.Name)
	}
	if body.URL != nil {
		updates["url"] = strings.TrimSpace(*body.URL)
	}
	if body.IsActive != nil {
		updates["is_active"] = *body.IsActive
	}

	err = initializers.DB.Transaction(func(tx *gorm.DB) error {
		if len(updates) > 0 {
			if err := tx.Model(&company).Updates(updates).Error; err != nil {
				return err
			}
		}

		if uploadedImageURL != "" {
			if company.Image != nil {
				if err := tx.Model(company.Image).Update("url", uploadedImageURL).Error; err != nil {
					return err
				}
			} else {
				img := models.AvailableDeliveryCompanyImage{
					AvailableDeliveryCompanyID: company.ID,
					URL:                        uploadedImageURL,
				}
				if err := tx.Create(&img).Error; err != nil {
					return err
				}
			}
		}

		return nil
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to update delivery company",
			"error":   err.Error(),
		})
		return
	}

	initializers.DB.Preload("Image").First(&company, "id = ?", company.ID)

	utils.LogAudit(c, "available_delivery_company.update", "AvailableDeliveryCompany", &company.ID, gin.H{
		"updates":      updates,
		"imageChanged": uploadedImageURL != "",
	})

	if body.IsActive != nil {
		utils.LogAudit(c, "available_delivery_company.toggle", "AvailableDeliveryCompany", &company.ID, gin.H{
			"name":     company.Name,
			"isActive": *body.IsActive,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Available delivery company updated successfully",
		"data":    company,
	})
}

func DeleteAvailableDeliveryCompany(c *gin.Context) {
	companyID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid delivery company ID"})
		return
	}

	var company models.AvailableDeliveryCompany
	if err := initializers.DB.First(&company, "id = ?", companyID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Delivery company not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Database error", "error": err.Error()})
		return
	}

	if err := initializers.DB.Delete(&company).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to delete delivery company",
			"error":   err.Error(),
		})
		return
	}

	utils.LogAudit(c, "available_delivery_company.delete", "AvailableDeliveryCompany", &company.ID, gin.H{
		"name": company.Name,
		"url":  company.URL,
	})

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Delivery company deleted successfully"})
}

// ---------------------------------------------------------------------------
// Shop delivery company integrations (per-shop credentials)
// ---------------------------------------------------------------------------

type ConnectDeliveryCompanyInput struct {
	AvailableDeliveryCompanyID string `json:"availableDeliveryCompanyId" binding:"required"`
	Token                      string `json:"token"`
	MerchantID                 string `json:"merchantId"`
}

type UpdateDeliveryCompanyCredentialsInput struct {
	Token      *string `json:"token"`
	MerchantID *string `json:"merchantId"`
}

// deliveryCompanyResponse is the credential-free projection of
// models.DeliveryCompany. Token/MerchantID are carrier auth material (used
// as API keys/tenant IDs in leopardController.go/zrController.go) and never
// belong in an HTTP response — only whether one is set, plus the last 4
// chars of Token for the admin to visually confirm which credential is live.
type deliveryCompanyResponse struct {
	ID                         uuid.UUID                       `json:"id"`
	ShopID                     uuid.UUID                       `json:"shopId"`
	AvailableDeliveryCompanyID uuid.UUID                       `json:"availableDeliveryCompanyId"`
	AvailableDeliveryCompany   models.AvailableDeliveryCompany `json:"availableDeliveryCompany"`
	HasToken                   bool                            `json:"hasToken"`
	TokenLast4                 string                          `json:"tokenLast4"`
	IsActive                   bool                            `json:"isActive"`
}

// decryptDeliveryCompanyCredentials decrypts integration.Token/MerchantID in
// place. Shared by every carrier's find*Integration lookup (leopardController.go,
// osenController.go, zrController.go, andersonController.go) — the single
// choke point every outbound carrier call routes through, so the many call
// sites that build HTTP requests from integration.Token/.MerchantID need no
// change themselves.
func decryptDeliveryCompanyCredentials(integration *models.DeliveryCompany) error {
	token, err := services.DecryptField(integration.Token)
	if err != nil {
		return err
	}
	merchantID, err := services.DecryptField(integration.MerchantID)
	if err != nil {
		return err
	}
	integration.Token = token
	integration.MerchantID = merchantID
	return nil
}

func toDeliveryCompanyResponse(d models.DeliveryCompany) deliveryCompanyResponse {
	// ponytail: decrypt here (not an outbound-call site) only to preserve the
	// pre-existing masked "last 4 chars" display for the admin; d.Token itself
	// is still never returned. Decrypt failure just hides the hint, not fatal.
	tokenLast4 := ""
	if d.Token != "" {
		if plainToken, err := services.DecryptField(d.Token); err == nil {
			if len(plainToken) <= 4 {
				tokenLast4 = plainToken
			} else {
				tokenLast4 = plainToken[len(plainToken)-4:]
			}
		}
	}
	return deliveryCompanyResponse{
		ID:                         d.ID,
		ShopID:                     d.ShopID,
		AvailableDeliveryCompanyID: d.AvailableDeliveryCompanyID,
		AvailableDeliveryCompany:   d.AvailableDeliveryCompany,
		HasToken:                   d.Token != "",
		TokenLast4:                 tokenLast4,
		IsActive:                   d.IsActive,
	}
}

func toDeliveryCompanyResponses(companies []models.DeliveryCompany) []deliveryCompanyResponse {
	out := make([]deliveryCompanyResponse, 0, len(companies))
	for _, d := range companies {
		out = append(out, toDeliveryCompanyResponse(d))
	}
	return out
}

func GetShopDeliveryCompanies(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	var integrations []models.DeliveryCompany
	if err := initializers.DB.
		Preload("AvailableDeliveryCompany").
		Preload("AvailableDeliveryCompany.Image").
		Where("shop_id = ?", shopID).
		Find(&integrations).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to fetch shop delivery companies",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": toDeliveryCompanyResponses(integrations)})
}

func ConnectDeliveryCompany(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	var body ConnectDeliveryCompanyInput
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Validation failed", "error": err.Error()})
		return
	}
	body.Token = strings.TrimSpace(body.Token)
	body.MerchantID = strings.TrimSpace(body.MerchantID)

	availableID, err := uuid.Parse(body.AvailableDeliveryCompanyID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid available delivery company ID"})
		return
	}

	var available models.AvailableDeliveryCompany
	if err := initializers.DB.First(&available, "id = ?", availableID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Available delivery company not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Database error", "error": err.Error()})
		return
	}

	// Validate credentials against the carrier's API before saving.
	if strings.EqualFold(strings.TrimSpace(available.Name), "osen express") {
		valid, errMsg := validateOsenToken(body.Token)
		if !valid {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": fmt.Sprintf("Token Osen Express invalide: %s", errMsg),
			})
			return
		}
	}
	if strings.EqualFold(strings.TrimSpace(available.Name), "zr express") {
		valid, errMsg := validateZrCredentials(body.Token, body.MerchantID)
		if !valid {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": fmt.Sprintf("Identifiants ZR Express invalides: %s", errMsg),
			})
			return
		}
	}
	if strings.Contains(strings.ToLower(strings.TrimSpace(available.Name)), "anderson") {
		valid, errMsg := validateAndersonToken(body.Token)
		if !valid {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": fmt.Sprintf("Token Anderson invalide: %s", errMsg),
			})
			return
		}
	}

	var existing models.DeliveryCompany
	err = initializers.DB.
		Where("shop_id = ? AND available_delivery_company_id = ?", shopID, availableID).
		First(&existing).Error
	if err == nil {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"message": fmt.Sprintf("%s is already connected to this shop", available.Name),
		})
		return
	}
	if err != gorm.ErrRecordNotFound {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Database error", "error": err.Error()})
		return
	}

	encryptedToken, encryptedMerchantID, err := services.EncryptDeliveryCredentials(body.Token, body.MerchantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to secure delivery company credentials",
			"error":   err.Error(),
		})
		return
	}

	integration := models.DeliveryCompany{
		ShopID:                     shopID,
		AvailableDeliveryCompanyID: availableID,
		Token:                      encryptedToken,
		MerchantID:                 encryptedMerchantID,
	}

	if err := initializers.DB.Create(&integration).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to connect delivery company",
			"error":   err.Error(),
		})
		return
	}

	initializers.DB.
		Preload("AvailableDeliveryCompany").
		Preload("AvailableDeliveryCompany.Image").
		First(&integration, "id = ?", integration.ID)

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "Delivery company connected successfully",
		"data":    toDeliveryCompanyResponse(integration),
	})
}

func UpdateDeliveryCompanyCredentials(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	integrationID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid integration ID"})
		return
	}

	var body UpdateDeliveryCompanyCredentialsInput
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Validation failed", "error": err.Error()})
		return
	}

	var integration models.DeliveryCompany
	if err := initializers.DB.
		Where("id = ? AND shop_id = ?", integrationID, shopID).
		First(&integration).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Integration not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Database error", "error": err.Error()})
		return
	}

	updates := map[string]any{}
	if body.Token != nil {
		updates["token"] = strings.TrimSpace(*body.Token)
	}
	if body.MerchantID != nil {
		updates["merchant_id"] = strings.TrimSpace(*body.MerchantID)
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "No fields provided for update"})
		return
	}

	if rawToken, ok := updates["token"].(string); ok {
		if encryptedToken, err := services.EncryptField(rawToken); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to secure token", "error": err.Error()})
			return
		} else {
			updates["token"] = encryptedToken
		}
	}
	if rawMerchantID, ok := updates["merchant_id"].(string); ok {
		if encryptedMerchantID, err := services.EncryptField(rawMerchantID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to secure merchant ID", "error": err.Error()})
			return
		} else {
			updates["merchant_id"] = encryptedMerchantID
		}
	}

	if err := initializers.DB.Model(&integration).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to update credentials",
			"error":   err.Error(),
		})
		return
	}

	initializers.DB.
		Preload("AvailableDeliveryCompany").
		Preload("AvailableDeliveryCompany.Image").
		First(&integration, "id = ?", integration.ID)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Credentials updated successfully",
		"data":    toDeliveryCompanyResponse(integration),
	})
}

type UpdateDeliveryCompanyActiveInput struct {
	IsActive bool `json:"isActive"`
}

// UpdateDeliveryCompanyActive sets (not toggles) whether this shop's carrier
// integration is a live ship target. The client always sends the target
// state rather than asking the server to flip it, avoiding a read-then-write
// race if two tabs are open.
func UpdateDeliveryCompanyActive(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	integrationID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid integration ID"})
		return
	}

	var body UpdateDeliveryCompanyActiveInput
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Validation failed", "error": err.Error()})
		return
	}

	var integration models.DeliveryCompany
	if err := initializers.DB.
		Where("id = ? AND shop_id = ?", integrationID, shopID).
		First(&integration).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Integration not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Database error", "error": err.Error()})
		return
	}

	if err := initializers.DB.Model(&integration).Update("is_active", body.IsActive).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to update carrier status",
			"error":   err.Error(),
		})
		return
	}

	initializers.DB.
		Preload("AvailableDeliveryCompany").
		Preload("AvailableDeliveryCompany.Image").
		First(&integration, "id = ?", integration.ID)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Carrier status updated successfully",
		"data":    integration,
	})
}

// bureauOption is one selectable entry in the manual-pick dropdown that
// Phase 5's ManualBureauPickModal renders for a needsManualPick order.
type bureauOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// GetDeliveryCompanyBureauOptions lists the real hubs/municipalities a
// carrier resolves to for a given wilaya, powering the manual bureau-pick
// dropdown for batch-ship orders that came back needsManualPick. Only ZR and
// Osen have a location-resolution layer (Decision 1) — Leopard/Anderson 404,
// since the client never calls this route for them.
func GetDeliveryCompanyBureauOptions(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	integrationID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid integration ID"})
		return
	}

	wilayaID := strings.TrimSpace(c.Query("wilayaId"))
	if wilayaID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "wilayaId est requis"})
		return
	}

	var integration models.DeliveryCompany
	if err := initializers.DB.
		Preload("AvailableDeliveryCompany").
		Where("id = ? AND shop_id = ?", integrationID, shopID).
		First(&integration).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Integration not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Database error", "error": err.Error()})
		return
	}
	if err := decryptDeliveryCompanyCredentials(&integration); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to decrypt delivery company credentials", "error": err.Error()})
		return
	}

	name := strings.ToLower(integration.AvailableDeliveryCompany.Name)

	switch {
	case strings.Contains(name, "zr"):
		wilaya, ok := findZrWilayaTerritory(wilayaID)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"success": true, "data": []bureauOption{}})
			return
		}
		hubs, err := loadZrHubs(shopID, &integration)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": "Impossible de charger les points de relais ZR Express", "error": err.Error()})
			return
		}
		options := make([]bureauOption, 0, len(hubs))
		for _, h := range hubs {
			if !h.IsPickupPoint || !h.IsVisible || h.Address.CityTerritoryID != wilaya.ID {
				continue
			}
			options = append(options, bureauOption{ID: h.ID, Label: h.Address.District})
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": options})
		return

	case strings.Contains(name, "osen"):
		stateCodeInt, err := strconv.Atoi(wilayaID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "wilayaId invalide"})
			return
		}
		provinces, err := initializers.GetOsenMunicipalities()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Impossible de charger la géographie Osen Express", "error": err.Error()})
			return
		}
		options := []bureauOption{}
		for _, p := range provinces {
			if p.ID != stateCodeInt {
				continue
			}
			for _, m := range p.Municipalities {
				options = append(options, bureauOption{ID: strconv.Itoa(m.ID), Label: m.NameLatin})
			}
			break
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": options})
		return

	default:
		// Leopard/Anderson have no location-resolution layer in this codebase
		// (Decision 1) — this route is never called for them client-side, so
		// a misrouted call fails loudly rather than returning nonsense.
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Sélection manuelle du bureau non prise en charge pour ce transporteur"})
		return
	}
}

func DisconnectDeliveryCompany(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	integrationID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid integration ID"})
		return
	}

	var integration models.DeliveryCompany
	if err := initializers.DB.
		Where("id = ? AND shop_id = ?", integrationID, shopID).
		First(&integration).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Integration not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Database error", "error": err.Error()})
		return
	}

	if err := initializers.DB.Delete(&integration).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to disconnect delivery company",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Delivery company disconnected successfully"})
}

// ---------------------------------------------------------------------------
// Image upload helper
// ---------------------------------------------------------------------------

func uploadAvailableDeliveryCompanyImage(c *gin.Context) (string, error) {
	file, err := c.FormFile("image")
	if err != nil || file == nil {
		return "", nil
	}

	src, err := file.Open()
	if err != nil {
		return "", fmt.Errorf("failed to open uploaded image")
	}
	defer src.Close()

	buf := make([]byte, 512)
	n, err := src.Read(buf)
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("failed to read uploaded image")
	}

	contentType := http.DetectContentType(buf[:n])
	if !strings.HasPrefix(contentType, "image/") {
		return "", fmt.Errorf("only image files are allowed")
	}

	seeker, ok := src.(io.Seeker)
	if !ok {
		return "", fmt.Errorf("failed to process image stream")
	}
	if _, err := seeker.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("failed to reset image stream")
	}

	cleanFileName := filepath.Base(file.Filename)
	key := fmt.Sprintf("uploads/delivery-companies/%d_%s", time.Now().UnixNano(), cleanFileName)

	bucketName := os.Getenv("B2_BUCKET_NAME")
	b2Region := os.Getenv("B2_REGION")
	b2PublicBaseURL := strings.TrimSpace(os.Getenv("B2_PUBLIC_BASE_URL"))

	_, putErr := initializers.S3Client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket:        aws.String(bucketName),
		Key:           aws.String(key),
		Body:          src,
		ContentType:   aws.String(contentType),
		ContentLength: aws.Int64(file.Size),
	})
	if putErr != nil {
		return "", fmt.Errorf("failed to upload image: %w", putErr)
	}

	if b2PublicBaseURL != "" {
		return fmt.Sprintf("%s/%s", strings.TrimRight(b2PublicBaseURL, "/"), key), nil
	}
	return fmt.Sprintf("https://%s.s3.%s.backblazeb2.com/%s", bucketName, b2Region, key), nil
}
