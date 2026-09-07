package controllers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/services"
	"github.com/chtiwa/dzbazar-server/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const andersonOrdersCacheTTL = 2 * time.Minute

// Anderson delivery (ECOTRACK platform). Auth is a bare api_token query param
// on every call — no bearer header, no request body on the single-order
// endpoint (it's all query params too).
const andersonBaseURL = "https://anderson-ecommerce.ecotrack.dz"

// ── Token validation ─────────────────────────────────────────────────────────

func validateAndersonToken(token string) (bool, string) {
	client := &http.Client{Timeout: 10 * time.Second}
	reqURL := fmt.Sprintf("%s/api/v1/validate/token?api_token=%s", andersonBaseURL, url.QueryEscape(token))
	resp, err := client.Get(reqURL)
	if err != nil {
		return false, "Impossible de joindre Anderson. Vérifiez votre connexion."
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("anderson: validate-token returned status %d: %s", resp.StatusCode, string(body))
		return false, "Token Anderson invalide ou expiré."
	}
	return true, ""
}

// ── Integration lookup ───────────────────────────────────────────────────────

func findAndersonIntegration(shopID uuid.UUID) (*models.DeliveryCompany, error) {
	var integration models.DeliveryCompany
	err := initializers.DB.
		Preload("AvailableDeliveryCompany").
		Joins("JOIN available_delivery_companies adc ON adc.id = delivery_companies.available_delivery_company_id").
		Where("delivery_companies.shop_id = ? AND LOWER(adc.name) LIKE ?", shopID, "%anderson%").
		First(&integration).Error
	if err != nil {
		return nil, err
	}
	return &integration, nil
}

// ── Ship order ────────────────────────────────────────────────────────────────

type andersonShipError struct {
	status int
	msg    string
}

func (e *andersonShipError) Error() string { return e.msg }

// shipOrderToAnderson creates an Anderson (Ecotrack) order for the given local
// order via the create/order query-param endpoint. The order must have Client
// and Items.Product preloaded.
func shipOrderToAnderson(c *gin.Context, order *models.Order, integration *models.DeliveryCompany) (map[string]any, error) {
	stopDesk := "0"
	cityName := order.Client.City
	if order.ShippingMethod != "Domicile" {
		stopDesk = "1"
		cityName = order.Client.StopdeskPoint
	}

	amount := math.Floor(order.TotalPrice/10) * 10

	params := url.Values{}
	params.Set("api_token", integration.Token)
	params.Set("reference", order.ID.String())
	params.Set("nom_client", order.Client.FullName)
	params.Set("telephone", order.Client.PhoneNumber)
	params.Set("adresse", cityName)
	params.Set("commune", cityName)
	params.Set("code_wilaya", order.Client.StateCode)
	params.Set("montant", fmt.Sprintf("%.0f", amount))
	params.Set("remarque", order.Note)
	params.Set("produit", buildShipmentDescription(order))
	params.Set("type", "1") // Livraison
	params.Set("stop_desk", stopDesk)
	if order.Fragile {
		params.Set("fragile", "1")
	}

	reqURL := fmt.Sprintf("%s/api/v1/create/order?%s", andersonBaseURL, params.Encode())
	httpClient := &http.Client{Timeout: 15 * time.Second}
	resp, err := httpClient.Post(reqURL, "application/json", nil)
	if err != nil {
		log.Printf("anderson: ship order %s request failed: %v", order.ID, err)
		return nil, &andersonShipError{http.StatusBadGateway, "Impossible de joindre Anderson"}
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var andersonResp map[string]any
	json.Unmarshal(respBody, &andersonResp)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		msg := "Anderson a refusé la commande"
		if m, ok := andersonResp["message"].(string); ok && m != "" {
			msg = m
		}
		return nil, &andersonShipError{http.StatusBadRequest, msg}
	}

	// A merchant can ship straight from "En attente" without ever marking the
	// order Confirmé first — that's a legitimate flow, but it would silently
	// exclude the order from the confirmation-rate metric (which counts orders
	// ever audit-logged into "Confirmé"). Backfill that transition here so
	// shipping always implies confirmed.
	if order.Status != "Confirmé" {
		utils.LogAudit(c, "order.status_changed", "Order", &order.ID, map[string]string{
			"from": order.Status,
			"to":   "Confirmé",
		})
	}

	updates := map[string]any{
		"is_shipped":     true,
		"status":         "Expedié",
		"shipped_at":     time.Now(),
		"shipped_via_id": integration.AvailableDeliveryCompanyID,
	}
	if trackingID, ok := andersonResp["tracking"].(string); ok && trackingID != "" {
		updates["tracking_number"] = trackingID
	}
	result := initializers.DB.Model(&models.Order{}).Where("id = ?", order.ID).Updates(updates)
	if result.Error != nil || result.RowsAffected == 0 {
		log.Printf("anderson: order %s shipped at carrier but local status update failed: %v", order.ID, result.Error)
		return nil, &andersonShipError{http.StatusInternalServerError, "Commande expédiée chez Anderson mais échec de la mise à jour locale du statut"}
	}

	if err := services.DecrementOrderItemsStock(initializers.DB, order.Items); err != nil {
		log.Printf("anderson: order %s shipped but stock decrement failed: %v", order.ID, err)
		return nil, &andersonShipError{http.StatusInternalServerError, "Commande expédiée chez Anderson mais échec de la mise à jour du stock"}
	}

	invalidateOrdersListCache(order.ShopID)

	return andersonResp, nil
}

// GetAndersonOrders proxies Ecotrack's paginated orders-with-status list.
func GetAndersonOrders(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	integration, err := findAndersonIntegration(shopID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Anderson n'est pas connecté à cette boutique"})
		return
	}

	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}

	cacheKey := fmt.Sprintf("anderson:orders:%s:%d", shopID, page)
	if cached, err := initializers.RClient.Get(initializers.Ctx, cacheKey).Result(); err == nil {
		var cachedResp map[string]any
		if json.Unmarshal([]byte(cached), &cachedResp) == nil {
			c.JSON(http.StatusOK, gin.H{"success": true, "data": cachedResp})
			return
		}
	}

	reqURL := fmt.Sprintf("%s/api/v1/get/orders?api_token=%s&page=%d",
		andersonBaseURL, url.QueryEscape(integration.Token), page)

	httpClient := &http.Client{Timeout: 15 * time.Second}
	resp, err := httpClient.Get(reqURL)
	if err != nil {
		log.Printf("anderson: GetAndersonOrders request failed: %v", err)
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": "Impossible de joindre Anderson"})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": "Erreur Anderson", "status": resp.StatusCode})
		return
	}

	var andersonResp map[string]any
	json.Unmarshal(body, &andersonResp)

	if encoded, err := json.Marshal(andersonResp); err == nil {
		initializers.RClient.Set(initializers.Ctx, cacheKey, encoded, andersonOrdersCacheTTL)
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": andersonResp})
}

type createAndersonOrderInput struct {
	OrderID string `json:"orderId" binding:"required"`
}

// CreateAndersonOrder creates an Anderson order mapped from a shop order.
func CreateAndersonOrder(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	var body createAndersonOrderInput
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "orderId est requis"})
		return
	}

	orderID, err := uuid.Parse(body.OrderID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid order ID"})
		return
	}

	var order models.Order
	if err := initializers.DB.
		Preload("Client").
		Preload("Items.Product").
		Preload("Items.ProductVariantCombination").
		Where("id = ? AND shop_id = ?", orderID, shopID).
		First(&order).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Commande introuvable"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Erreur base de données"})
		}
		return
	}

	if order.IsShipped {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Cette commande a déjà été expédiée"})
		return
	}

	integration, err := findAndersonIntegration(shopID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Anderson n'est pas connecté à cette boutique"})
		return
	}

	andersonOrder, err := shipOrderToAnderson(c, &order, integration)
	if err != nil {
		var shipErr *andersonShipError
		if errors.As(err, &shipErr) {
			c.JSON(shipErr.status, gin.H{"success": false, "message": shipErr.msg})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		}
		return
	}

	c.JSON(http.StatusCreated, gin.H{"success": true, "data": andersonOrder})
}

// ── Bulk shipping ─────────────────────────────────────────────────────────────

type bulkAndersonOrderInput struct {
	OrderIDs []string `json:"orderIds" binding:"required"`
}

type bulkAndersonShipResult struct {
	OrderID    string `json:"orderId"`
	Success    bool   `json:"success"`
	Message    string `json:"message,omitempty"`
	TrackingID string `json:"trackingId,omitempty"`
}

// BulkCreateAndersonOrders ships multiple shop orders to Anderson in one request.
// Ecotrack has a native multi-order endpoint (create/orders, up to 100), but
// results there come back keyed by array index with no per-order error detail
// worth the extra request-shape complexity — looping the single endpoint
// gives the same per-order success/failure list the bulk UI needs.
func BulkCreateAndersonOrders(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	var body bulkAndersonOrderInput
	if err := c.ShouldBindJSON(&body); err != nil || len(body.OrderIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "orderIds est requis"})
		return
	}

	integration, err := findAndersonIntegration(shopID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Anderson n'est pas connecté à cette boutique"})
		return
	}

	results := make([]bulkAndersonShipResult, 0, len(body.OrderIDs))

	for _, idStr := range body.OrderIDs {
		orderID, err := uuid.Parse(idStr)
		if err != nil {
			results = append(results, bulkAndersonShipResult{OrderID: idStr, Success: false, Message: "ID de commande invalide"})
			continue
		}

		var order models.Order
		if err := initializers.DB.
			Preload("Client").
			Preload("Items.Product").
			Preload("Items.ProductVariantCombination").
			Where("id = ? AND shop_id = ?", orderID, shopID).
			First(&order).Error; err != nil {
			results = append(results, bulkAndersonShipResult{OrderID: idStr, Success: false, Message: "Commande introuvable"})
			continue
		}

		if order.IsShipped {
			results = append(results, bulkAndersonShipResult{OrderID: idStr, Success: false, Message: "Déjà expédiée"})
			continue
		}

		andersonOrder, err := shipOrderToAnderson(c, &order, integration)
		if err != nil {
			results = append(results, bulkAndersonShipResult{OrderID: idStr, Success: false, Message: err.Error()})
			continue
		}

		trackingID, _ := andersonOrder["tracking"].(string)
		results = append(results, bulkAndersonShipResult{OrderID: idStr, Success: true, TrackingID: trackingID})
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": results})
}
