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
	"strings"
	"time"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const osenBaseURL = "https://backendmain.osenexpress.com"

// buildShipmentDescription renders a carrier-facing description covering every
// item on the order (not just the first), so a customer who ordered e.g. a
// brown AND a black variant sees both on the label instead of just one — a
// parcel that looks like it's missing half the order often gets refused at
// the door. The order must have Items.Product and
// Items.ProductVariantCombination preloaded.
func buildShipmentDescription(order *models.Order) string {
	if len(order.Items) == 0 {
		return "Produit"
	}

	parts := make([]string, 0, len(order.Items))
	for _, item := range order.Items {
		title := item.Product.Title
		if title == "" {
			title = "Produit"
		}
		part := title
		if item.ProductVariantCombination.CombinationString != "" {
			part += " x " + item.ProductVariantCombination.CombinationString
		}
		part += fmt.Sprintf(" — Qté: %d", item.Quantity)
		parts = append(parts, part)
	}

	return strings.Join(parts, ", ")
}

// ── Token validation ──────────────────────────────────────────────────────────

type osenValidateTokenReq struct {
	APIToken string `json:"api_token"`
}

type osenValidateTokenResp struct {
	Valid bool   `json:"valid"`
	Error string `json:"error"`
}

// validateOsenToken calls the Osen validate-token endpoint (no auth header required).
// Returns (true, "") on success or (false, humanReadableReason) on failure.
func validateOsenToken(token string) (bool, string) {
	payload, _ := json.Marshal(osenValidateTokenReq{APIToken: token})
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(
		osenBaseURL+"/v1/users/auth/customers/validate-token",
		"application/json",
		bytes.NewBuffer(payload),
	)
	if err != nil {
		return false, "Impossible de joindre Osen Express. Vérifiez votre connexion."
	}
	defer resp.Body.Close()

	var result osenValidateTokenResp
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, "Réponse invalide de Osen Express."
	}
	if !result.Valid {
		if result.Error != "" {
			return false, result.Error
		}
		return false, "Token invalide ou expiré."
	}
	return true, ""
}

// ── Geo data ──────────────────────────────────────────────────────────────────
// Osen's province/municipality list is bundled as a static embedded JSON file
// (see initializers/osen_geo.go) instead of being fetched live, since it rarely
// changes and the live geo endpoint adds avoidable latency to order creation.

// findOsenMunicipalityID maps a wilaya stateCode + city name to an Osen municipality ID.
// Osen province IDs map 1:1 to Algerian wilaya codes (1–58).
// Falls back to the first municipality in the province if no city name match.
func findOsenMunicipalityID(stateCode, cityName string) (int, error) {
	provinces, err := initializers.GetOsenMunicipalities()
	if err != nil {
		return 0, fmt.Errorf("failed to load Osen geography: %w", err)
	}

	var target *initializers.OsenProvinceSeed
	for i, p := range provinces {
		if fmt.Sprintf("%d", p.ID) == stateCode {
			target = &provinces[i]
			break
		}
	}
	if target == nil || len(target.Municipalities) == 0 {
		return 0, fmt.Errorf("province %s not found in Osen coverage", stateCode)
	}

	cityLower := strings.ToLower(strings.TrimSpace(cityName))
	if cityLower != "" {
		// Exact match
		for _, m := range target.Municipalities {
			if strings.ToLower(m.NameLatin) == cityLower {
				return m.ID, nil
			}
		}
		// Contains match
		for _, m := range target.Municipalities {
			mLower := strings.ToLower(m.NameLatin)
			if strings.Contains(mLower, cityLower) || strings.Contains(cityLower, mLower) {
				return m.ID, nil
			}
		}
	}
	// Fallback: first municipality
	return target.Municipalities[0].ID, nil
}

// ── Orders cache ──────────────────────────────────────────────────────────────
// Osen's orders API is slow, so successful responses are cached per shop/page in
// Redis. A per-shop version counter is bumped whenever a new order is created so
// the cache is invalidated without scanning keys.

const osenOrdersCacheTTL = 2 * time.Minute

func osenOrdersCacheVersionKey(shopID uuid.UUID) string {
	return fmt.Sprintf("osen:orders:version:%s", shopID)
}

func osenOrdersCacheVersion(shopID uuid.UUID) int64 {
	val, err := initializers.RClient.Get(initializers.Ctx, osenOrdersCacheVersionKey(shopID)).Int64()
	if err != nil {
		return 0
	}
	return val
}

func bumpOsenOrdersCacheVersion(shopID uuid.UUID) {
	initializers.RClient.Incr(initializers.Ctx, osenOrdersCacheVersionKey(shopID))
}

// ── Integration lookup ────────────────────────────────────────────────────────

func findOsenIntegration(shopID uuid.UUID) (*models.DeliveryCompany, error) {
	var integration models.DeliveryCompany
	err := initializers.DB.
		Preload("AvailableDeliveryCompany").
		Joins("JOIN available_delivery_companies adc ON adc.id = delivery_companies.available_delivery_company_id").
		Where("delivery_companies.shop_id = ? AND LOWER(adc.name) LIKE ?", shopID, "%osen%").
		First(&integration).Error
	if err != nil {
		return nil, err
	}
	return &integration, nil
}

// ── Osen order types ──────────────────────────────────────────────────────────

type osenMuniRef struct {
	ID int `json:"id"`
}

type osenOrderTarget struct {
	FullName           string      `json:"fullName"`
	PhoneNumberPrimary string      `json:"phoneNumberPrimary"`
	Address            string      `json:"address,omitempty"`
	Municipality       osenMuniRef `json:"municipality"`
}

type osenOrderParcel struct {
	Description string  `json:"description"`
	Weight      float64 `json:"weight"`
	Fragile     bool    `json:"fragile"`
	Openable    bool    `json:"openable"`
	Tryable     bool    `json:"tryable"`
	Amount      float64 `json:"amount"`
	RefrenceID  string  `json:"refrenceId,omitempty"`
	Note        string  `json:"note,omitempty"`
}

type osenOrderOperation struct {
	DeliveryType string `json:"deliveryType"` // DOORSTEP | STOPDESK
}

type osenCashCollection struct {
	TotalPostTarget float64 `json:"totalPostTarget"`
}

type osenCreateOrderReq struct {
	Target         osenOrderTarget    `json:"target"`
	Parcel         osenOrderParcel    `json:"parcel"`
	Operation      osenOrderOperation `json:"operation"`
	CashCollection osenCashCollection `json:"cashCollection"`
}

// ── Controllers ───────────────────────────────────────────────────────────────

// GetOsenOrders proxies GET /v1/orders from the shop's Osen integration.
func GetOsenOrders(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	integration, err := findOsenIntegration(shopID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Osen Express n'est pas connecté à cette boutique"})
		return
	}

	skip := c.DefaultQuery("skip", "0")
	take := c.DefaultQuery("take", "20")

	cacheKey := fmt.Sprintf("osen:orders:%s:%d:%s:%s", shopID, osenOrdersCacheVersion(shopID), skip, take)

	if cached, err := initializers.RClient.Get(initializers.Ctx, cacheKey).Result(); err == nil {
		var cachedResp map[string]any
		if json.Unmarshal([]byte(cached), &cachedResp) == nil {
			c.JSON(http.StatusOK, gin.H{"success": true, "data": cachedResp})
			return
		}
	}

	req, _ := http.NewRequest("GET",
		fmt.Sprintf("%s/v1/orders?skip=%s&take=%s", osenBaseURL, skip, take), nil)
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(integration.Token))

	httpClient := &http.Client{Timeout: 15 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("osen: GetOsenOrders request failed: %v", err)
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": "Impossible de joindre Osen Express"})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": "Erreur Osen Express", "status": resp.StatusCode})
		return
	}

	var osenResp map[string]any
	json.Unmarshal(body, &osenResp)

	if encoded, err := json.Marshal(osenResp); err == nil {
		initializers.RClient.Set(initializers.Ctx, cacheKey, encoded, osenOrdersCacheTTL)
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": osenResp})
}

type createOsenOrderInput struct {
	OrderID string `json:"orderId" binding:"required"`
}

// osenShipError carries an HTTP status code alongside a user-facing message,
// so callers can map shipment failures to the right response code.
type osenShipError struct {
	status int
	msg    string
}

func (e *osenShipError) Error() string { return e.msg }

// shipOrderToOsen creates + validates an Osen order for the given local order.
// On success it stores Osen's tracking number, marks the order as shipped, and
// decrements stock for each ordered variant. The order must have Client and
// Items.Product preloaded.
func shipOrderToOsen(order *models.Order, integration *models.DeliveryCompany) (map[string]any, error) {
	// For Stopdesk orders the commune is stored in StopdeskPoint (the bureau
	// name); City is left empty by the storefront checkout in that case.
	cityName := order.Client.City
	if order.ShippingMethod != "Domicile" {
		cityName = order.Client.StopdeskPoint
	}

	municipalityID, err := findOsenMunicipalityID(order.Client.StateCode, cityName)
	if err != nil {
		return nil, &osenShipError{http.StatusBadRequest, fmt.Sprintf("Zone de livraison non couverte: %s", err.Error())}
	}

	description := buildShipmentDescription(order)

	// Round COD and parcel value down to nearest 10 (Osen requirement).
	cod := math.Floor(order.TotalPrice/10) * 10
	parcelAmt := math.Floor(order.TotalPrice/10) * 10

	deliveryType := "DOORSTEP"
	if order.ShippingMethod != "Domicile" {
		deliveryType = "STOPDESK"
	}

	osenReq := osenCreateOrderReq{
		Target: osenOrderTarget{
			FullName:           order.Client.FullName,
			PhoneNumberPrimary: order.Client.PhoneNumber,
			Municipality:       osenMuniRef{ID: municipalityID},
		},
		Parcel: osenOrderParcel{
			Description: description,
			Fragile:     order.Fragile,
			Openable:    order.Essayable,
			Tryable:     order.Essayable,
			Amount:      parcelAmt,
			RefrenceID:  order.ID.String(),
			Note:        order.Note,
		},
		Operation:      osenOrderOperation{DeliveryType: deliveryType},
		CashCollection: osenCashCollection{TotalPostTarget: cod},
	}

	reqBody, _ := json.Marshal(osenReq)
	createReq, _ := http.NewRequest("POST", osenBaseURL+"/v1/orders", bytes.NewBuffer(reqBody))
	createReq.Header.Set("Authorization", "Bearer "+strings.TrimSpace(integration.Token))
	createReq.Header.Set("Content-Type", "application/json")

	httpClient := &http.Client{Timeout: 15 * time.Second}
	resp, err := httpClient.Do(createReq)
	if err != nil {
		log.Printf("osen: ship order %s request failed: %v", order.ID, err)
		return nil, &osenShipError{http.StatusBadGateway, "Impossible de joindre Osen Express"}
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusCreated {
		var osenErr map[string]any
		json.Unmarshal(respBody, &osenErr)
		msg := "Osen Express a refusé la commande"
		if m, ok := osenErr["message"].(string); ok && m != "" {
			msg = m
		}
		return nil, &osenShipError{http.StatusBadRequest, msg}
	}

	var osenOrder map[string]any
	json.Unmarshal(respBody, &osenOrder)

	// Auto-validate: submit to carrier in the same flow.
	if osenOrderID, ok := osenOrder["id"].(string); ok && osenOrderID != "" {
		validateReq, _ := http.NewRequest("PATCH",
			fmt.Sprintf("%s/v1/orders/%s/validate", osenBaseURL, osenOrderID), nil)
		validateReq.Header.Set("Authorization", "Bearer "+strings.TrimSpace(integration.Token))
		if _, err := httpClient.Do(validateReq); err != nil {
			log.Printf("osen: order %s created but validate call failed: %v", osenOrderID, err)
		}
	}

	updates := map[string]any{
		"is_shipped":     true,
		"status":         "Expedié",
		"shipped_at":     time.Now(),
		"shipped_via_id": integration.AvailableDeliveryCompanyID,
	}
	if trackingID, ok := osenOrder["trackingId"].(string); ok && trackingID != "" {
		updates["tracking_number"] = trackingID
	}
	// The parcel already exists at Osen at this point — if this write fails we
	// must surface it as an error rather than report success, otherwise the
	// merchant sees a success toast while the order silently stays unshipped
	// locally (and risks being shipped twice).
	result := initializers.DB.Model(&models.Order{}).Where("id = ?", order.ID).Updates(updates)
	if result.Error != nil {
		log.Printf("osen: order %s shipped at carrier but local status update failed: %v", order.ID, result.Error)
		return nil, &osenShipError{http.StatusInternalServerError, "Commande expédiée chez Osen Express mais échec de la mise à jour locale du statut"}
	}
	if result.RowsAffected == 0 {
		log.Printf("osen: order %s shipped at carrier but local status update affected 0 rows", order.ID)
		return nil, &osenShipError{http.StatusInternalServerError, "Commande expédiée chez Osen Express mais le statut local n'a pas pu être mis à jour"}
	}

	decrementOrderItemsStock(initializers.DB, order.Items)

	bumpOsenOrdersCacheVersion(order.ShopID)
	invalidateOrdersListCache(order.ShopID)

	return osenOrder, nil
}

// CreateOsenOrder creates + validates an Osen order mapped from a shop order.
func CreateOsenOrder(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	var body createOsenOrderInput
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

	integration, err := findOsenIntegration(shopID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Osen Express n'est pas connecté à cette boutique"})
		return
	}

	osenOrder, err := shipOrderToOsen(&order, integration)
	if err != nil {
		var shipErr *osenShipError
		if errors.As(err, &shipErr) {
			c.JSON(shipErr.status, gin.H{"success": false, "message": shipErr.msg})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		}
		return
	}

	c.JSON(http.StatusCreated, gin.H{"success": true, "data": osenOrder})
}

// ── Bulk shipping ─────────────────────────────────────────────────────────────

type bulkOsenOrderInput struct {
	OrderIDs []string `json:"orderIds" binding:"required"`
}

type bulkOsenShipResult struct {
	OrderID    string `json:"orderId"`
	Success    bool   `json:"success"`
	Message    string `json:"message,omitempty"`
	TrackingID string `json:"trackingId,omitempty"`
}

// BulkCreateOsenOrders ships multiple shop orders to Osen Express in one request.
func BulkCreateOsenOrders(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	var body bulkOsenOrderInput
	if err := c.ShouldBindJSON(&body); err != nil || len(body.OrderIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "orderIds est requis"})
		return
	}

	integration, err := findOsenIntegration(shopID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Osen Express n'est pas connecté à cette boutique"})
		return
	}

	results := make([]bulkOsenShipResult, 0, len(body.OrderIDs))

	for _, idStr := range body.OrderIDs {
		orderID, err := uuid.Parse(idStr)
		if err != nil {
			results = append(results, bulkOsenShipResult{OrderID: idStr, Success: false, Message: "ID de commande invalide"})
			continue
		}

		var order models.Order
		if err := initializers.DB.
			Preload("Client").
			Preload("Items.Product").
			Preload("Items.ProductVariantCombination").
			Where("id = ? AND shop_id = ?", orderID, shopID).
			First(&order).Error; err != nil {
			results = append(results, bulkOsenShipResult{OrderID: idStr, Success: false, Message: "Commande introuvable"})
			continue
		}

		if order.IsShipped {
			results = append(results, bulkOsenShipResult{OrderID: idStr, Success: false, Message: "Déjà expédiée"})
			continue
		}

		osenOrder, err := shipOrderToOsen(&order, integration)
		if err != nil {
			results = append(results, bulkOsenShipResult{OrderID: idStr, Success: false, Message: err.Error()})
			continue
		}

		trackingID, _ := osenOrder["trackingId"].(string)
		results = append(results, bulkOsenShipResult{OrderID: idStr, Success: true, TrackingID: trackingID})
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": results})
}
