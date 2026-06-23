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

const procolisBaseURL = "https://procolis.com/api_v1"

// ── Integration lookup ────────────────────────────────────────────────────────

func findLeopardIntegration(shopID uuid.UUID) (*models.DeliveryCompany, error) {
	var integration models.DeliveryCompany
	err := initializers.DB.
		Preload("AvailableDeliveryCompany").
		Joins("JOIN available_delivery_companies adc ON adc.id = delivery_companies.available_delivery_company_id").
		Where("delivery_companies.shop_id = ? AND LOWER(adc.name) LIKE ?", shopID, "%leopard%").
		First(&integration).Error
	if err != nil {
		return nil, err
	}
	return &integration, nil
}

// ── Procolis order types ──────────────────────────────────────────────────────

type procolisColis struct {
	Tracking      string `json:"Tracking"`
	TypeLivraison string `json:"TypeLivraison"` // "0" Domicile, "1" Stopdesk
	TypeColis     string `json:"TypeColis"`     // "0" normal, "1" Echange
	Confrimee     string `json:"Confrimee"`     // "1" ready to ship, "" pending
	Client        string `json:"Client"`
	MobileA       string `json:"MobileA"`
	MobileB       string `json:"MobileB"`
	Adresse       string `json:"Adresse"`
	IDWilaya      string `json:"IDWilaya"`
	Commune       string `json:"Commune"`
	Total         string `json:"Total"`
	Note          string `json:"Note"`
	TProduit      string `json:"TProduit"`
	IDExterne     string `json:"id_Externe"`
	Source        string `json:"Source"`
}

type procolisAddColisReq struct {
	Colis []procolisColis `json:"Colis"`
}

// ── Controllers ───────────────────────────────────────────────────────────────

// shipOrderToLeopard creates a Leopard Express (Procolis) parcel for the given
// local order. On success it stores the tracking code, marks the order as
// shipped, and decrements stock for each ordered variant. The order must have
// Client and Items.Product preloaded.
func shipOrderToLeopard(order *models.Order, integration *models.DeliveryCompany) (map[string]any, error) {
	commune := order.Client.City
	if order.ShippingMethod != "Domicile" {
		commune = order.Client.StopdeskPoint
	}

	typeLivraison := "0"
	if order.ShippingMethod != "Domicile" {
		typeLivraison = "1"
	}

	description := buildShipmentDescription(order)

	tracking := order.ID.String()
	total := fmt.Sprintf("%.0f", math.Floor(order.TotalPrice))

	colis := procolisColis{
		Tracking:      tracking,
		TypeLivraison: typeLivraison,
		TypeColis:     "0",
		Confrimee:     "",
		Client:        order.Client.FullName,
		MobileA:       order.Client.PhoneNumber,
		MobileB:       order.Client.PhoneNumber2,
		Adresse:       "",
		IDWilaya:      order.Client.StateCode,
		Commune:       commune,
		Total:         total,
		Note:          order.Note,
		TProduit:      description,
		IDExterne:     tracking,
		Source:        "",
	}

	reqBody, _ := json.Marshal(procolisAddColisReq{Colis: []procolisColis{colis}})
	createReq, err := http.NewRequest("POST", procolisBaseURL+"/add_colis", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, &osenShipError{http.StatusInternalServerError, "Impossible de créer la requête Leopard Express"}
	}
	createReq.Header.Set("token", strings.TrimSpace(integration.Token))
	createReq.Header.Set("key", strings.TrimSpace(integration.MerchantID))
	createReq.Header.Set("Content-Type", "application/json")

	httpClient := &http.Client{Timeout: 15 * time.Second}
	resp, err := httpClient.Do(createReq)
	if err != nil {
		log.Printf("leopard: ship order %s request failed: %v", order.ID, err)
		return nil, &osenShipError{http.StatusBadGateway, "Impossible de joindre Leopard Express"}
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("leopard: order %s rejected, status %d: %s", order.ID, resp.StatusCode, string(respBody))
		return nil, &osenShipError{http.StatusBadRequest, "Leopard Express a refusé la commande"}
	}

	// The parcel already exists at Leopard at this point — if this write fails
	// we must surface it as an error rather than report success, otherwise the
	// merchant sees a success toast while the order silently stays unshipped
	// locally (and risks being shipped twice).
	result := initializers.DB.Model(&models.Order{}).Where("id = ?", order.ID).Updates(map[string]any{
		"is_shipped":      true,
		"status":          "Expedié",
		"tracking_number": tracking,
		"shipped_at":      time.Now(),
		"shipped_via_id":  integration.AvailableDeliveryCompanyID,
	})
	if result.Error != nil {
		log.Printf("leopard: order %s shipped at carrier but local status update failed: %v", order.ID, result.Error)
		return nil, &osenShipError{http.StatusInternalServerError, "Commande expédiée chez Leopard Express mais échec de la mise à jour locale du statut"}
	}
	if result.RowsAffected == 0 {
		log.Printf("leopard: order %s shipped at carrier but local status update affected 0 rows", order.ID)
		return nil, &osenShipError{http.StatusInternalServerError, "Commande expédiée chez Leopard Express mais le statut local n'a pas pu être mis à jour"}
	}

	decrementOrderItemsStock(initializers.DB, order.Items)

	return map[string]any{"tracking": tracking, "raw": string(respBody)}, nil
}

type createLeopardOrderInput struct {
	OrderID string `json:"orderId" binding:"required"`
}

// CreateLeopardOrder creates a Leopard Express order mapped from a shop order.
func CreateLeopardOrder(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	var body createLeopardOrderInput
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

	integration, err := findLeopardIntegration(shopID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Leopard Express n'est pas connecté à cette boutique"})
		return
	}

	result, err := shipOrderToLeopard(&order, integration)
	if err != nil {
		var shipErr *osenShipError
		if errors.As(err, &shipErr) {
			c.JSON(shipErr.status, gin.H{"success": false, "message": shipErr.msg})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		}
		return
	}

	c.JSON(http.StatusCreated, gin.H{"success": true, "data": result})
}
