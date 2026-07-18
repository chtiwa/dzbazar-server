package controllers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/services"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const zrBaseURL = "https://api.zrexpress.app"
const zrDefaultWeightKg = 1.0

// ── Auth ──────────────────────────────────────────────────────────────────────
// ZR Express auth: X-Tenant identifies the merchant's account, X-Api-Key is
// their API key. Both live per-shop on DeliveryCompany (MerchantID = tenant ID,
// Token = API key), same slot Leopard reuses for its token+key pair.

func zrAuthHeaders(req *http.Request, integration *models.DeliveryCompany) {
	req.Header.Set("X-Tenant", strings.TrimSpace(integration.MerchantID))
	req.Header.Set("X-Api-Key", strings.TrimSpace(integration.Token))
	req.Header.Set("Accept", "application/json")
}

// validateZrCredentials probes ZR with a cheap authenticated GET, since ZR has
// no documented dedicated validate-token endpoint.
func validateZrCredentials(token, tenantID string) (bool, string) {
	req, err := http.NewRequest("GET", zrBaseURL+"/api/v1/delivery-pricing/rates", nil)
	if err != nil {
		return false, "Impossible de créer la requête de validation ZR Express"
	}
	req.Header.Set("X-Tenant", strings.TrimSpace(tenantID))
	req.Header.Set("X-Api-Key", strings.TrimSpace(token))
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, "Impossible de joindre ZR Express. Vérifiez votre connexion."
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return false, "Identifiants ZR Express invalides."
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, fmt.Sprintf("ZR Express a répondu avec une erreur (%d)", resp.StatusCode)
	}
	return true, ""
}

// ── Integration lookup ────────────────────────────────────────────────────────

func findZrIntegration(shopID uuid.UUID) (*models.DeliveryCompany, error) {
	var integration models.DeliveryCompany
	err := initializers.DB.
		Preload("AvailableDeliveryCompany").
		Joins("JOIN available_delivery_companies adc ON adc.id = delivery_companies.available_delivery_company_id").
		Where("delivery_companies.shop_id = ? AND LOWER(adc.name) LIKE ?", shopID, "%zr%").
		First(&integration).Error
	if err != nil {
		return nil, err
	}
	return &integration, nil
}

// ── ZR order types ────────────────────────────────────────────────────────────

type zrPhone struct {
	Number1 string `json:"number1"`
}

type zrCustomer struct {
	CustomerID string  `json:"customerId"`
	Name       string  `json:"name"`
	Phone      zrPhone `json:"phone"`
}

// zrDeliveryAddress mirrors DeliveryAddressInputDto exactly as documented
// (additionalProperties: false — city/country/district/postalCode aren't in
// this schema at all and were rejected as unknown properties).
type zrDeliveryAddress struct {
	CityTerritoryID     string `json:"cityTerritoryId"`
	DistrictTerritoryID string `json:"districtTerritoryId"`
	Street              string `json:"street,omitempty"`
}

type zrOrderedProduct struct {
	UnitPrice   float64 `json:"unitPrice"`
	Quantity    int     `json:"quantity"`
	ProductName string  `json:"productName"`
	StockType   string  `json:"stockType"`
}

// zrWeight mirrors ParcelWeightDto exactly as documented: weight is an
// object, not a plain number — sending a bare number here is a type mismatch
// that crashes the API's deserializer with an empty body before any
// validation runs (confirmed: docs.zrexpress.app/reference/createparcelendpoint).
type zrWeight struct {
	Weight            float64  `json:"weight"`
	DimensionalWeight *float64 `json:"dimensionalWeight,omitempty"`
}

type zrCreateParcelReq struct {
	Customer        zrCustomer         `json:"customer"`
	DeliveryAddress zrDeliveryAddress  `json:"deliveryAddress"`
	Amount          float64            `json:"amount"`
	Weight          zrWeight           `json:"weight"`
	OrderedProducts []zrOrderedProduct `json:"orderedProducts"`
	DeliveryType    string             `json:"deliveryType"`
	HubID           string             `json:"hubId,omitempty"`
	Description     string             `json:"description,omitempty"`
	ExternalID      string             `json:"externalId,omitempty"`
}

// formatZrPhone normalizes a local Algerian phone number to +213 E.164 form.
func formatZrPhone(phone string) string {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return ""
	}
	if strings.HasPrefix(phone, "0") {
		return "+213" + phone[1:]
	}
	if strings.HasPrefix(phone, "+213") {
		return phone
	}
	return "+213" + phone
}

// ── Orders cache ──────────────────────────────────────────────────────────────

const zrOrdersCacheTTL = 2 * time.Minute

func zrOrdersCacheVersionKey(shopID uuid.UUID) string {
	return fmt.Sprintf("zr:orders:version:%s", shopID)
}

func zrOrdersCacheVersion(shopID uuid.UUID) int64 {
	val, err := initializers.RClient.Get(initializers.Ctx, zrOrdersCacheVersionKey(shopID)).Int64()
	if err != nil {
		return 0
	}
	return val
}

func bumpZrOrdersCacheVersion(shopID uuid.UUID) {
	initializers.RClient.Incr(initializers.Ctx, zrOrdersCacheVersionKey(shopID))
}

// ── Error handling ────────────────────────────────────────────────────────────

type zrErrorDetail struct {
	Description string `json:"description"`
}

type zrErrorResp struct {
	Detail  string          `json:"detail"`
	Title   string          `json:"title"`
	Message string          `json:"message"`
	Errors  []zrErrorDetail `json:"errors"`
}

// extractZrErrorMessage pulls the most specific human-readable message out of
// a ZR Express error response. ZR uses RFC 7807 problem+json: "title" is a
// generic category (e.g. "General.Validation"), while the actionable detail
// is in "errors[].description" (validation failures) or "detail".
func extractZrErrorMessage(body []byte) string {
	var zrErr zrErrorResp
	if err := json.Unmarshal(body, &zrErr); err != nil {
		return fmt.Sprintf("ZR Express a refusé la commande (réponse illisible: %s)", truncateForLog(body))
	}

	if len(zrErr.Errors) > 0 {
		descriptions := make([]string, 0, len(zrErr.Errors))
		for _, e := range zrErr.Errors {
			if e.Description != "" {
				descriptions = append(descriptions, e.Description)
			}
		}
		if len(descriptions) > 0 {
			return strings.Join(descriptions, "; ")
		}
	}
	if zrErr.Detail != "" && zrErr.Detail != "An unexpected error occurred" {
		return zrErr.Detail
	}
	if zrErr.Message != "" {
		return zrErr.Message
	}
	if zrErr.Title != "" {
		return zrErr.Title
	}
	// Nothing recognized matched — surface the raw body so the failure is
	// self-diagnosing from the Network tab instead of needing server logs.
	return fmt.Sprintf("ZR Express a refusé la commande: %s", truncateForLog(body))
}

// ── Shipping ──────────────────────────────────────────────────────────────────

// shipOrderToZr creates a ZR Express parcel for the given local order. On
// success it stores ZR's tracking number, marks the order as shipped, and
// decrements stock for each ordered variant. The order must have Client and
// Items.Product/Items.ProductVariantCombination preloaded.
func shipOrderToZr(order *models.Order, integration *models.DeliveryCompany) (map[string]any, error) {
	deliveryType := "home"
	if order.ShippingMethod != "Domicile" {
		deliveryType = "pickup-point"
	}

	// For Stopdesk orders the commune is stored in StopdeskPoint; City is left
	// empty by the storefront checkout in that case (same convention Osen and
	// Leopard already rely on).
	cityName := order.Client.City
	if deliveryType == "pickup-point" {
		cityName = order.Client.StopdeskPoint
	}

	wilayaID, districtID, err := resolveZrTerritoryID(order.Client.StateCode, order.Client.State, cityName)
	if err != nil {
		return nil, &osenShipError{http.StatusBadRequest, fmt.Sprintf("Zone de livraison ZR Express non couverte: %s", err.Error())}
	}

	var hubID string
	if deliveryType == "pickup-point" {
		hubID, err = resolveZrHubID(order.ShopID, integration, order.Client.StateCode, order.Client.State, cityName)
		if err != nil {
			return nil, &osenShipError{http.StatusBadRequest, "Point de relais ZR Express introuvable pour cette wilaya"}
		}
	}

	orderedProducts := make([]zrOrderedProduct, 0, len(order.Items))
	for _, item := range order.Items {
		title := item.Product.Title
		if title == "" {
			title = "Produit"
		}
		if item.ProductVariantCombination.CombinationString != "" {
			title += " x " + item.ProductVariantCombination.CombinationString
		}
		orderedProducts = append(orderedProducts, zrOrderedProduct{
			UnitPrice:   item.Price,
			Quantity:    int(item.Quantity),
			ProductName: title,
			StockType:   "none",
		})
	}

	zrReq := zrCreateParcelReq{
		Customer: zrCustomer{
			CustomerID: uuid.NewString(),
			Name:       order.Client.FullName,
			Phone:      zrPhone{Number1: formatZrPhone(order.Client.PhoneNumber)},
		},
		DeliveryAddress: zrDeliveryAddress{
			CityTerritoryID:     wilayaID,
			DistrictTerritoryID: districtID,
		},
		Amount:          math.Floor(order.TotalPrice),
		Weight:          zrWeight{Weight: zrDefaultWeightKg},
		OrderedProducts: orderedProducts,
		DeliveryType:    deliveryType,
		HubID:           hubID,
		Description:     buildShipmentDescription(order),
		ExternalID:      order.ID.String(),
	}

	reqBody, _ := json.Marshal(zrReq)
	createReq, err := http.NewRequest("POST", zrBaseURL+"/api/v1/parcels", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, &osenShipError{http.StatusInternalServerError, "Impossible de créer la requête ZR Express"}
	}
	zrAuthHeaders(createReq, integration)
	createReq.Header.Set("Content-Type", "application/json")

	httpClient := &http.Client{Timeout: 15 * time.Second}
	resp, err := httpClient.Do(createReq)
	if err != nil {
		log.Printf("zr: ship order %s request failed: %v", order.ID, err)
		return nil, &osenShipError{http.StatusBadGateway, fmt.Sprintf("Impossible de joindre ZR Express: %s", err.Error())}
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	// Confirmed live: ZR returns 200 OK on a successful parcel creation, not
	// the 201 Created its REST convention would suggest.
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, &osenShipError{http.StatusBadRequest, fmt.Sprintf("[HTTP %d] %s", resp.StatusCode, extractZrErrorMessage(respBody))}
	}

	var zrParcel map[string]any
	json.Unmarshal(respBody, &zrParcel)

	updates := map[string]any{
		"is_shipped":     true,
		"status":         "Expedié",
		"shipped_at":     time.Now(),
		"shipped_via_id": integration.AvailableDeliveryCompanyID,
	}
	for _, key := range []string{"trackingNumber", "tracking", "id"} {
		if tracking, ok := zrParcel[key].(string); ok && tracking != "" {
			updates["tracking_number"] = tracking
			break
		}
	}

	// The parcel already exists at ZR Express at this point — if this write
	// fails we must surface it as an error rather than report success,
	// otherwise the merchant sees a success toast while the order silently
	// stays unshipped locally (and risks being shipped twice).
	result := initializers.DB.Model(&models.Order{}).Where("id = ?", order.ID).Updates(updates)
	if result.Error != nil {
		log.Printf("zr: order %s shipped at carrier but local status update failed: %v", order.ID, result.Error)
		return nil, &osenShipError{http.StatusInternalServerError, "Commande expédiée chez ZR Express mais échec de la mise à jour locale du statut"}
	}
	if result.RowsAffected == 0 {
		log.Printf("zr: order %s shipped at carrier but local status update affected 0 rows", order.ID)
		return nil, &osenShipError{http.StatusInternalServerError, "Commande expédiée chez ZR Express mais le statut local n'a pas pu être mis à jour"}
	}

	if err := services.DecrementOrderItemsStock(initializers.DB, order.Items); err != nil {
		log.Printf("zr: order %s shipped but stock decrement failed: %v", order.ID, err)
		return nil, &osenShipError{http.StatusInternalServerError, "Commande expédiée chez ZR Express mais échec de la mise à jour du stock"}
	}

	bumpZrOrdersCacheVersion(order.ShopID)
	invalidateOrdersListCache(order.ShopID)

	return zrParcel, nil
}

type createZrOrderInput struct {
	OrderID string `json:"orderId" binding:"required"`
}

// CreateZrOrder ships a single shop order to ZR Express.
func CreateZrOrder(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	var body createZrOrderInput
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

	integration, err := findZrIntegration(shopID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "ZR Express n'est pas connecté à cette boutique"})
		return
	}

	zrOrder, err := shipOrderToZr(&order, integration)
	if err != nil {
		var shipErr *osenShipError
		if errors.As(err, &shipErr) {
			c.JSON(shipErr.status, gin.H{"success": false, "message": shipErr.msg})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		}
		return
	}

	c.JSON(http.StatusCreated, gin.H{"success": true, "data": zrOrder})
}

// ── Bulk shipping ─────────────────────────────────────────────────────────────

type bulkZrOrderInput struct {
	OrderIDs []string `json:"orderIds" binding:"required"`
}

type bulkZrShipResult struct {
	OrderID    string `json:"orderId"`
	Success    bool   `json:"success"`
	Message    string `json:"message,omitempty"`
	TrackingID string `json:"trackingId,omitempty"`
}

// BulkCreateZrOrders ships multiple shop orders to ZR Express in one request.
func BulkCreateZrOrders(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	var body bulkZrOrderInput
	if err := c.ShouldBindJSON(&body); err != nil || len(body.OrderIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "orderIds est requis"})
		return
	}

	integration, err := findZrIntegration(shopID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "ZR Express n'est pas connecté à cette boutique"})
		return
	}

	results := make([]bulkZrShipResult, 0, len(body.OrderIDs))

	for _, idStr := range body.OrderIDs {
		orderID, err := uuid.Parse(idStr)
		if err != nil {
			results = append(results, bulkZrShipResult{OrderID: idStr, Success: false, Message: "ID de commande invalide"})
			continue
		}

		var order models.Order
		if err := initializers.DB.
			Preload("Client").
			Preload("Items.Product").
			Preload("Items.ProductVariantCombination").
			Where("id = ? AND shop_id = ?", orderID, shopID).
			First(&order).Error; err != nil {
			results = append(results, bulkZrShipResult{OrderID: idStr, Success: false, Message: "Commande introuvable"})
			continue
		}

		if order.IsShipped {
			results = append(results, bulkZrShipResult{OrderID: idStr, Success: false, Message: "Déjà expédiée"})
			continue
		}

		zrOrder, err := shipOrderToZr(&order, integration)
		if err != nil {
			results = append(results, bulkZrShipResult{OrderID: idStr, Success: false, Message: err.Error()})
			continue
		}

		var trackingID string
		for _, key := range []string{"trackingNumber", "tracking", "id"} {
			if t, ok := zrOrder[key].(string); ok && t != "" {
				trackingID = t
				break
			}
		}
		results = append(results, bulkZrShipResult{OrderID: idStr, Success: true, TrackingID: trackingID})
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": results})
}

// ── Tracking list ─────────────────────────────────────────────────────────────

// zrSearchParcelsReq's optional fields all use omitempty: ZR's generic search
// endpoints 500 if these are sent as explicit null and 400 if sent as {} —
// omitting them entirely (confirmed live) is the only shape that works.
type zrSearchParcelsReq struct {
	AdvancedSearch  any      `json:"advancedSearch,omitempty"`
	Keyword         any      `json:"keyword,omitempty"`
	AdvancedFilter  any      `json:"advancedFilter,omitempty"`
	PageSize        int      `json:"pageSize"`
	PageNumber      int      `json:"pageNumber"`
	OrderBy         []string `json:"orderBy,omitempty"`
	IncludeProducts bool     `json:"includeProducts,omitempty"`
	ParcelTypes     any      `json:"parcelTypes,omitempty"`
}

// GetZrOrders proxies ZR Express's parcel search (tracking list) for the shop's
// integration, translating skip/take into ZR's pageNumber/pageSize.
func GetZrOrders(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	integration, err := findZrIntegration(shopID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "ZR Express n'est pas connecté à cette boutique"})
		return
	}

	skip, err := strconv.Atoi(c.DefaultQuery("skip", "0"))
	if err != nil || skip < 0 {
		skip = 0
	}
	take, err := strconv.Atoi(c.DefaultQuery("take", "20"))
	if err != nil || take <= 0 {
		take = 20
	}
	pageNumber := skip/take + 1

	cacheKey := fmt.Sprintf("zr:orders:%s:%d:%d:%d", shopID, zrOrdersCacheVersion(shopID), skip, take)

	if cached, err := initializers.RClient.Get(initializers.Ctx, cacheKey).Result(); err == nil {
		var cachedResp map[string]any
		if json.Unmarshal([]byte(cached), &cachedResp) == nil {
			c.JSON(http.StatusOK, gin.H{"success": true, "data": cachedResp})
			return
		}
	}

	searchReq := zrSearchParcelsReq{
		PageSize:        take,
		PageNumber:      pageNumber,
		IncludeProducts: false,
	}
	reqBody, _ := json.Marshal(searchReq)

	req, _ := http.NewRequest("POST", zrBaseURL+"/api/v1/parcels/search", bytes.NewBuffer(reqBody))
	zrAuthHeaders(req, integration)
	req.Header.Set("Content-Type", "application/json")

	httpClient := &http.Client{Timeout: 15 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("zr: GetZrOrders request failed: %v", err)
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": fmt.Sprintf("Impossible de joindre ZR Express: %s", err.Error())})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": "Erreur ZR Express", "status": resp.StatusCode})
		return
	}

	var zrResp map[string]any
	json.Unmarshal(body, &zrResp)

	if encoded, err := json.Marshal(zrResp); err == nil {
		initializers.RClient.Set(initializers.Ctx, cacheKey, encoded, zrOrdersCacheTTL)
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": zrResp})
}
