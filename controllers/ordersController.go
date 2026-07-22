package controllers

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/services"
	"github.com/chtiwa/dzbazar-server/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Replace your old CreateOrderInput with this:
type CreateOrderInput struct {
	ShopID           string  `json:"shopId" binding:"required"`
	ShippingMethod   string  `json:"shippingMethod" binding:"required"`
	ShippingPrice    float64 `json:"shippingPrice"`
	TotalPrice       float64 `json:"totalPrice"`
	Note             string  `json:"note"`
	Ouvrable         bool    `json:"ouvrable"`
	Fragile          bool    `json:"fragile"`
	Essayable        bool    `json:"essayable"`
	ConversionSource string  `json:"conversionSource"`
	FBclid           string  `json:"fbclid"`
	FBc              string  `json:"fbc"`
	FBp              string  `json:"fbp"`
	TTclid           string  `json:"ttclid"`
	TTp              string  `json:"ttp"`
	// The customer's browser URL at checkout (window.location.href),
	// forwarded as event_source_url/page.url on the Meta/TikTok CAPI Purchase
	// sends (see orderEvents.go) instead of a hardcoded domain literal.
	// Attribution/display only — never blocks checkout if missing.
	PageURL    string `json:"pageUrl"`
	CouponCode string `json:"couponCode"`
	// Landing page this order originated from, if any. Optional, attribution-only.
	LandingPageID string `json:"landingPageId"`

	// Honeypot: hidden "Email" field no real customer sees or fills (this
	// form has no genuine email field). Bot autofillers populate it; any
	// non-empty value here means the request is spam.
	Email string `json:"email"`

	// Nested client object matches your new JSON payload
	Client OrderClientInput `json:"client" binding:"required"`

	// Dive validates the nested slice
	Items []OrderItemInput `json:"items" binding:"required,min=1,dive"`
}

// Add this new struct for the client
type OrderClientInput struct {
	FullName      string `json:"fullName" binding:"required"`
	PhoneNumber   string `json:"phoneNumber" binding:"required"`
	PhoneNumber2  string `json:"phoneNumber2"`
	State         string `json:"state" binding:"required"`
	StateCode     string `json:"stateCode"`
	City          string `json:"city"`
	StopdeskPoint string `json:"stopdeskPoint"`
}

// Replace your old item struct with this
type OrderItemInput struct {
	ProductID                   string  `json:"productId" binding:"required"`
	ProductVariantCombinationID string  `json:"productVariantCombinationID" binding:"required"` // Match JSON exactly
	Quantity                    uint    `json:"quantity" binding:"required,min=1"`
	Price                       float64 `json:"price" binding:"required"`
	// OfferID is optional and only ever used to look up which published offer's
	// discount rule applies — the resulting price is always recomputed
	// server-side (see priceForOrderItem in ordersController.go). Price above
	// is never trusted, same as everywhere else in this handler.
	OfferID *string `json:"offerId"`
}

type UpdateOrderInput struct {
	ShippingMethod *string           `json:"shippingMethod"`
	ShippingPrice  *float64          `json:"shippingPrice"`
	Note           *string           `json:"note"`
	Status         string            `json:"status"`
	ReportedDate   *string           `json:"reportedDate"`
	Ouvrable       *bool             `json:"ouvrable"`
	Fragile        *bool             `json:"fragile"`
	Essayable      *bool             `json:"essayable"`
	IsShipped      *bool             `json:"isShipped"`
	ShippedViaID   *string           `json:"shippedViaId"`
	Client         *OrderClientInput `json:"client"`
	Items          []OrderItemInput  `json:"items"`
}

// Only the unfiltered first page gets cached — any status/search/date filter
// hits the DB directly. That's the view merchants have open by default and
// poll/refresh most; filtered views are one-off lookups not worth a cache key.
const ordersListCacheTTL = 15 * time.Minute

func ordersListCacheKey(shopID uuid.UUID) string {
	return fmt.Sprintf("orders:list:default:%s", shopID)
}

// invalidateOrdersListCache must be called after ANY write to an order row
// (create/update/delete/status change/ship), from any file — the cached page
// renders order fields (status, tracking, etc), not just row presence.
func invalidateOrdersListCache(shopID uuid.UUID) {
	initializers.RClient.Del(initializers.Ctx, ordersListCacheKey(shopID))
}

func GetOrdersByShopID(c *gin.Context) {
	user, ok := c.Get("user")
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to retrieve authenticated session user",
		})
		return
	}

	userData, ok := user.(models.User)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Invalid session user structure context",
		})
		return
	}

	shopIDStr := c.Param("shopId")
	shopID, err := uuid.Parse(shopIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid or missing Shop ID",
		})
		return
	}

	var membership models.ShopMember
	if err := initializers.DB.
		Where("shop_id = ? AND user_id = ?", shopID, userData.ID).
		First(&membership).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "You do not have access to this shop",
		})
		return
	}

	page := 1
	perPage := 10

	if pageString := c.Query("page"); pageString != "" {
		if parsedPage, err := strconv.Atoi(pageString); err == nil && parsedPage > 0 {
			page = parsedPage
		}
	}

	if perPageString := c.Query("perPage"); perPageString != "" {
		if parsedPerPage, err := strconv.Atoi(perPageString); err == nil && parsedPerPage > 0 {
			perPage = parsedPerPage
		}
	}

	status := c.Query("status")
	search := strings.TrimSpace(c.Query("search"))
	dateFrom := c.Query("dateFrom")
	dateTo := c.Query("dateTo")
	flaggedOnly := c.Query("flagged") == "true"
	productID := strings.TrimSpace(c.Query("productId"))

	// A confirmatrice only ever sees orders assigned to her — never the
	// shop's shared default-view cache, which isn't scoped per-member.
	isConfirmatrice := membership.Role == "confirmation"

	isDefaultView := !isConfirmatrice && page == 1 && perPage == 10 && (status == "" || status == "Tous") &&
		search == "" && dateFrom == "" && dateTo == "" && !flaggedOnly && productID == ""

	if isDefaultView {
		if cached, err := initializers.RClient.Get(initializers.Ctx, ordersListCacheKey(shopID)).Bytes(); err == nil {
			c.Data(http.StatusOK, "application/json", cached)
			return
		}
	}

	baseQuery := initializers.DB.Model(&models.Order{}).Where("orders.shop_id = ? AND orders.is_hidden = ?", shopID, flaggedOnly)

	if isConfirmatrice {
		baseQuery = baseQuery.Where("orders.assigned_member_id = ?", membership.ID)
	}

	if status != "" && status != "Tous" {
		baseQuery = baseQuery.Where("status = ?", status)
	}

	// A start date with no end date means "that single day only".
	if dateTo == "" {
		dateTo = dateFrom
	}

	if dateFrom != "" {
		if parsed, err := time.Parse("2006-01-02", dateFrom); err == nil {
			baseQuery = baseQuery.Where("orders.created_at >= ?", parsed)
		}
	}

	if dateTo != "" {
		if parsed, err := time.Parse("2006-01-02", dateTo); err == nil {
			baseQuery = baseQuery.Where("orders.created_at < ?", parsed.AddDate(0, 0, 1))
		}
	}

	if search != "" {
		likeSearch := "%" + search + "%"
		baseQuery = baseQuery.
			Joins("JOIN clients client ON client.id = orders.client_id").
			Where("client.full_name ILIKE ? OR client.phone_number LIKE ?", likeSearch, likeSearch)
	}

	if productID != "" {
		if parsedProductID, err := uuid.Parse(productID); err == nil {
			baseQuery = baseQuery.Where(
				"EXISTS (SELECT 1 FROM order_items oi WHERE oi.order_id = orders.id AND oi.product_id = ?)",
				parsedProductID,
			)
		}
	}

	var totalRows int64
	if err := baseQuery.Count(&totalRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Error while counting the orders",
			"error":   err.Error(),
		})
		return
	}

	totalPages := int(math.Ceil(float64(totalRows) / float64(perPage)))
	if totalPages == 0 {
		totalPages = 1
	}

	if page > totalPages {
		page = totalPages
	}

	offset := (page - 1) * perPage

	var orders []models.Order
	if err := baseQuery.
		Preload("Client").
		Preload("Items").
		Preload("Items.Product").
		Preload("Items.ProductVariantCombination").
		Preload("ShippedVia").
		Preload("ShippedVia.Image").
		Preload("AssignedMember").
		Preload("AssignedMember.User").
		Order("created_at DESC").
		Limit(perPage).
		Offset(offset).
		Find(&orders).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Error retrieving the orders",
			"error":   err.Error(),
		})
		return
	}

	pagination := utils.GetPaginationData(page, totalPages, "/orders")

	body, err := json.Marshal(gin.H{
		"success":    true,
		"message":    "Orders were retrieved successfully",
		"data":       orders,
		"pagination": pagination,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to encode orders response"})
		return
	}

	if isDefaultView {
		initializers.RClient.Set(initializers.Ctx, ordersListCacheKey(shopID), body, ordersListCacheTTL)
	}

	c.Data(http.StatusOK, "application/json", body)
}

const (
	phoneOrderWindow = 30 * time.Minute
)

func phoneOrderKey(shopID uuid.UUID, phone string) string {
	return fmt.Sprintf("ratelimit:order:phone:%s:%s", shopID, phone)
}

func orderCooldownCookie(shopID uuid.UUID) string {
	return "order_cd_" + shopID.String()
}

func CreateOrderByShopID(c *gin.Context) {
	var body CreateOrderInput
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Error while binding JSON request context",
			"error":   err.Error(),
		})
		return
	}

	parsedShopID, err := uuid.Parse(body.ShopID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid Shop ID payload format"})
		return
	}

	// Honeypot short-circuit: as early as possible, before any DB round trip.
	// A real customer never sees this field; a bot autofiller that populates
	// it gets the same silent-success response as the banned-client paths.
	if strings.TrimSpace(body.Email) != "" {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "Order received successfully",
		})
		return
	}

	if err := services.CheckOrderLimit(parsedShopID); err != nil {
		if errors.Is(err, services.ErrPlanLimitReached) {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "This store has reached its order limit for the current plan and cannot accept new orders right now.",
				"code":    "PLAN_LIMIT_REACHED",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to verify plan limits", "error": err.Error()})
		return
	}

	isStaffOrder := middleware.IsStaffOrder(c)

	// Per-browser, per-shop 24h cooldown. Unlike the guards below, this is an
	// honest UX rule — tell the customer, don't fake success. Staff placing
	// orders from the dashboard aren't the customer this guards against.
	if !isStaffOrder {
		if _, err := c.Cookie(orderCooldownCookie(parsedShopID)); err == nil {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"success": false,
				"message": "لقد قمت بطلب من هذا المتجر مؤخراً. يرجى الانتظار قبل إرسال طلب آخر.",
				"code":    "ORDER_COOLDOWN",
			})
			return
		}
	}

	// Server-side enforcement of the same 05/06/07 + 8-digit rule the client
	// checks in OrderForm.tsx — hitting this endpoint directly bypasses the
	// client entirely, so it must be re-checked here.
	if !services.IsValidPhoneNumber(body.Client.PhoneNumber) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid phone number format",
		})
		return
	}

	// Banned-client short circuit: a client id banned by the owner (via
	// BanOrderClient) never touches the DB again — no client row, no order
	// row, nothing to review or clean up. Same silent-success response as
	// the phone rate limit below, so the troll sees an ordinary success page.
	banPlatform, banClientID := "", ""
	switch body.ConversionSource {
	case "facebook":
		banPlatform, banClientID = "facebook", body.FBp
	case "tiktok":
		banPlatform, banClientID = "tiktok", body.TTp
	}

	if banClientID != "" {
		var banned models.FlaggedClient
		isBanned := initializers.DB.
			Where("shop_id = ? AND platform = ? AND client_id = ?", parsedShopID, banPlatform, banClientID).
			First(&banned).Error == nil

		if isBanned {
			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"message": "Order received successfully",
			})
			return
		}
	}

	// Phone-based ban short circuit: complementary to the fbp/ttp mechanism
	// above — catches clients banned via BanOrderClient's organic fallback
	// (see BanOrderClient), who never had a platform click-id to ban by.
	var banCheckClient models.Client
	if initializers.DB.
		Where("phone_number = ? AND shop_id = ?", body.Client.PhoneNumber, parsedShopID).
		First(&banCheckClient).Error == nil && banCheckClient.Banned {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "Order received successfully",
		})
		return
	}

	// Phone-per-shop rate limit: 1 order per 30 min. Silent drop on breach.
	// Staff manually creating orders (e.g. re-entering the same customer) are
	// exempt — this guard exists for anonymous spam, not their workflow.
	phoneKey := phoneOrderKey(parsedShopID, body.Client.PhoneNumber)
	set, redisErr := initializers.RClient.SetNX(initializers.Ctx, phoneKey, 1, phoneOrderWindow).Result()
	if !isStaffOrder && redisErr == nil && !set {
		// Key already exists — this phone already ordered within the window.
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "Order received successfully",
		})
		return
	}

	var order models.Order
	clientUserAgent := c.Request.UserAgent()
	clientIP := c.ClientIP()

	// 1. ACID Transaction block to securely update tables together
	err = initializers.DB.Transaction(func(tx *gorm.DB) error {
		var client models.Client
		// FIX 1: Search for the client by phone number AND the specific Shop ID
		err := tx.Where("phone_number = ? AND shop_id = ?", body.Client.PhoneNumber, parsedShopID).First(&client).Error
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				client = models.Client{
					ShopID:        parsedShopID,
					FullName:      body.Client.FullName,
					PhoneNumber:   body.Client.PhoneNumber,
					PhoneNumber2:  body.Client.PhoneNumber2,
					State:         body.Client.State,
					StateCode:     body.Client.StateCode,
					City:          body.Client.City,
					StopdeskPoint: body.Client.StopdeskPoint,
				}
				if createErr := tx.Create(&client).Error; createErr != nil {
					return createErr // This will no longer fail!
				}
			} else {
				return err
			}
		} else {
			// Existing client: refresh their stored info so it reflects this latest
			// order rather than staying frozen on whatever their first order had.
			updates := map[string]any{
				"full_name":      body.Client.FullName,
				"phone_number2":  body.Client.PhoneNumber2,
				"state":          body.Client.State,
				"state_code":     body.Client.StateCode,
				"city":           body.Client.City,
				"stopdesk_point": body.Client.StopdeskPoint,
			}
			if updErr := tx.Model(&models.Client{}).
				Where("id = ? AND shop_id = ?", client.ID, parsedShopID).
				Updates(updates).Error; updErr != nil {
				return updErr
			}
			client.FullName = body.Client.FullName
			client.PhoneNumber2 = body.Client.PhoneNumber2
			client.State = body.Client.State
			client.StateCode = body.Client.StateCode
			client.City = body.Client.City
			client.StopdeskPoint = body.Client.StopdeskPoint
		}

		var orderItems []models.OrderItem
		var calculatedTotalPrice float64

		// 2. Loop and map checkout lines into database OrderItems structures.
		// Price and ProductID are never taken from the client — both are looked
		// up from the combination's own DB row inside this tx, scoped to this
		// shop, so a tampered checkout payload can't set an arbitrary price or
		// attach an item to a product it doesn't belong to.
		comboIDs := make([]uuid.UUID, 0, len(body.Items))
		for _, item := range body.Items {
			comboID, parseErr := uuid.Parse(item.ProductVariantCombinationID)
			if parseErr != nil {
				return fmt.Errorf("invalid combination uuid provided: %s", item.ProductVariantCombinationID)
			}
			comboIDs = append(comboIDs, comboID)
		}

		comboByID, err := services.FetchCombinationsForShop(tx, parsedShopID, comboIDs)
		if err != nil {
			return err
		}

		// Offer-linked items (offerId set) get their price recomputed from the
		// offer's own discount rule — never from combo.Price alone, and never
		// from the client's item.Price. An offerId that doesn't resolve to a
		// published offer owned by this shop is silently ignored and the item
		// falls back to plain combo.Price, same as an item with no offerId.
		offerIDs := make([]*string, len(body.Items))
		for i, item := range body.Items {
			offerIDs[i] = item.OfferID
		}
		offerByID, err := services.FetchPublishedOffersForShop(tx, parsedShopID, offerIDs)
		if err != nil {
			return err
		}

		for i, item := range body.Items {
			combo, ok := comboByID[comboIDs[i]]
			if !ok {
				return fmt.Errorf("combination not found for this shop: %s", item.ProductVariantCombinationID)
			}

			unitPrice, lineTotal := services.PricedOrderItem(combo, item.Quantity, item.OfferID, offerByID)

			orderItems = append(orderItems, models.OrderItem{
				ProductID:                   combo.ProductID,
				ProductVariantCombinationID: combo.ID,
				Quantity:                    item.Quantity,
				Price:                       unitPrice,
			})

			calculatedTotalPrice += lineTotal
		}

		// Coupon discount is recomputed server-side, never trusted from the client —
		// same trust boundary as the item-price total above. An invalid, inactive, or
		// out-of-scope code is silently ignored so checkout never fails on a stale promo.
		var couponID *uuid.UUID
		var discountAmount float64
		if code := strings.TrimSpace(body.CouponCode); code != "" {
			var coupon models.Coupon
			if err := tx.Preload("Products").Preload("LandingPages").
				Where("shop_id = ? AND UPPER(code) = ?", parsedShopID, strings.ToUpper(code)).
				First(&coupon).Error; err == nil {
				productIDs := make([]uuid.UUID, len(orderItems))
				for i, oi := range orderItems {
					productIDs[i] = oi.ProductID
				}
				if discount, matched := couponDiscount(coupon, productIDs, calculatedTotalPrice); matched {
					discountAmount = discount
					couponID = &coupon.ID
					calculatedTotalPrice -= discountAmount
				}
			}
		}

		// Shipping price is looked up from this shop's DeliveryRate for the
		// client's wilaya, never trusted from body.ShippingPrice.
		wilayaID, atoiErr := strconv.Atoi(body.Client.StateCode)
		if atoiErr != nil {
			return fmt.Errorf("invalid or missing wilaya code: %q", body.Client.StateCode)
		}
		var rate models.DeliveryRate
		if err := tx.Where("shop_id = ? AND wilaya_id = ?", parsedShopID, wilayaID).First(&rate).Error; err != nil {
			return err
		}
		shippingPrice, shipErr := services.ResolveShipping(rate, body.ShippingMethod)
		if shipErr != nil {
			return shipErr
		}
		calculatedTotalPrice += shippingPrice

		// A banned client id never reaches this point at all (see the
		// short-circuit above, before this transaction started). This is
		// purely first-offense detection: a cussword name on an id that
		// isn't banned yet gets auto-flagged for next time.
		containsCussword := utils.ContainsBannedWords(body.Client.FullName)
		if containsCussword && banClientID != "" {
			if flagErr := tx.Create(&models.FlaggedClient{ShopID: parsedShopID, Platform: banPlatform, ClientID: banClientID}).Error; flagErr != nil {
				return flagErr
			}
		}

		// Attribution only — an unparseable or missing landing page id just means this
		// order isn't attributed to one, never a reason to fail checkout.
		var landingPageID *uuid.UUID
		if trimmed := strings.TrimSpace(body.LandingPageID); trimmed != "" {
			if parsed, parseErr := uuid.Parse(trimmed); parseErr == nil {
				landingPageID = &parsed
			}
		}

		order = models.Order{
			ShopID:         parsedShopID,
			ClientID:       client.ID,
			ShippingMethod: body.ShippingMethod,
			ShippingPrice:  shippingPrice,
			TotalPrice:     calculatedTotalPrice,
			Status:         "En attente",
			Note:           body.Note,
			CouponID:       couponID,
			DiscountAmount: discountAmount,
			// FIX 3: Map the missing boolean flags from the body
			Ouvrable:         body.Ouvrable,
			Fragile:          body.Fragile,
			Essayable:        body.Essayable,
			FBclid:           body.FBclid,
			FBc:              body.FBc,
			FBp:              body.FBp,
			TTclid:           body.TTclid,
			TTp:              body.TTp,
			ConversionSource: body.ConversionSource,
			PageURL:          body.PageURL,
			LandingPageID:    landingPageID,
			IsHidden:         containsCussword,
			ClientIP:         clientIP,
			ClientUserAgent:  clientUserAgent,
			Items:            orderItems,
		}

		if createOrderErr := tx.Omit("Client", "Items.Product", "Items.ProductVariantCombination").Create(&order).Error; createOrderErr != nil {
			return createOrderErr
		}

		// Best-effort round-robin assignment to an eligible confirmatrice — never
		// fails checkout (mirrors DecideExperimentIfReady below).
		if assignErr := services.AutoAssignOrder(tx, parsedShopID, &order); assignErr != nil {
			fmt.Println("AutoAssignOrder: failed to auto-assign order:", assignErr)
		}

		// A completed order means this phone number is no longer "abandoned" —
		// soft-delete any prior abandoned lead so it stops being counted/shown as one.
		if delErr := tx.Where("shop_id = ? AND phone_number = ?", parsedShopID, body.Client.PhoneNumber).
			Delete(&models.AbandonedLead{}).Error; delErr != nil {
			return delErr
		}

		// A/B test winner check — best-effort, must never fail checkout.
		if landingPageID != nil {
			services.DecideExperimentIfReady(tx, *landingPageID)
		}

		return nil
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed saving records inside database transactions securely",
			"error":   err.Error(),
		})
		return
	}

	// 3. Async side-effects (email, Meta CAPI, live broadcast) run on the
	// bounded order-event worker pool — see controllers/orderEvents.go.
	enqueueOrderEvent(order.ID)

	InvalidateDashboardCache(parsedShopID)
	invalidateProductCaches(uuid.Nil, parsedShopID)
	invalidateOrdersListCache(parsedShopID)
	initializers.RClient.Del(initializers.Ctx, services.LandingPagesCacheKeyByShop(parsedShopID))

	isProduction := os.Getenv("APP_ENV") == "production"
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(orderCooldownCookie(parsedShopID), "1", 24*60*60, "/", "", isProduction, true)

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"message":  "The order was created successfully",
		"order_id": order.ID,
	})
}

func IndexOrderByShopID(c *gin.Context) {
	shopIDStr := c.Param("shopId")
	shopID, err := uuid.Parse(shopIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid or missing Shop ID parameter",
		})
		return
	}

	orderIDStr := c.Param("id")
	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid Order ID format",
		})
		return
	}

	var order models.Order

	err = initializers.DB.
		Model(&models.Order{}).
		Where("id = ? AND shop_id = ?", orderID, shopID).
		Preload("Client").
		Preload("Items").
		Preload("Items.Product").
		Preload("Items.Product.Images").
		Preload("Items.ProductVariantCombination").
		Preload("Items.ProductVariantCombination.Option1").
		Preload("Items.ProductVariantCombination.Option2").
		Preload("Items.ProductVariantCombination.Option3").
		Preload("ShippedVia").
		Preload("ShippedVia.Image").
		First(&order).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "Order not found or does not belong to this shop",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Database error while fetching order details",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Order was retrieved successfully",
		"data":    order,
	})
}

type FileExportResponse struct {
	StatusCode int
	FileBytes  []byte
	FileName   string
	MimeType   string
	Error      error
}

func ExportAsExcel(c *gin.Context) {
	var body []models.Order

	err := c.ShouldBindJSON(&body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "error while parsing the body",
			"error":   err.Error(),
		})
		return
	}

	// generate excel file
	excelBytes, err := utils.GenerateExcel(body)
	var result FileExportResponse
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "failed to generate the excel file",
		})
		return
	}

	result = FileExportResponse{
		StatusCode: http.StatusOK,
		FileBytes:  excelBytes,
		FileName:   "orders.xlsx",
		MimeType:   "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	}

	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", result.FileName))
	c.Data(result.StatusCode, result.MimeType, result.FileBytes)
}

func UpdateOrderByShopID(c *gin.Context) {
	shopIDStr := c.Param("shopId")
	shopID, err := uuid.Parse(shopIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid or missing Shop ID",
		})
		return
	}

	orderIDStr := c.Param("id")
	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid Order ID format",
		})
		return
	}

	var body UpdateOrderInput
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Error while binding JSON request context",
			"error":   err.Error(),
		})
		return
	}

	var oldStatus string

	err = initializers.DB.Transaction(func(tx *gorm.DB) error {
		var order models.Order
		if err := tx.
			Preload("Client").
			Preload("Items").
			First(&order, "id = ? AND shop_id = ?", orderID, shopID).Error; err != nil {
			return err
		}
		oldStatus = order.Status

		if body.Client != nil {
			if err := tx.Model(&models.Client{}).
				Where("id = ? AND shop_id = ?", order.ClientID, shopID).
				Updates(map[string]any{
					"full_name":      body.Client.FullName,
					"phone_number":   body.Client.PhoneNumber,
					"state":          body.Client.State,
					"state_code":     body.Client.StateCode,
					"city":           body.Client.City,
					"stopdesk_point": body.Client.StopdeskPoint,
				}).Error; err != nil {
				return err
			}
		}

		// Shipping price is only recomputed when the caller signals intent to
		// change it (body.ShippingPrice != nil); the value stored is always
		// looked up from DeliveryRate, never taken from the client.
		shippingPrice := order.ShippingPrice
		if body.ShippingPrice != nil {
			method := order.ShippingMethod
			if body.ShippingMethod != nil {
				method = *body.ShippingMethod
			}
			stateCode := order.Client.StateCode
			if body.Client != nil {
				stateCode = body.Client.StateCode
			}

			wilayaID, atoiErr := strconv.Atoi(stateCode)
			if atoiErr != nil {
				return fmt.Errorf("invalid or missing wilaya code: %q", stateCode)
			}
			var rate models.DeliveryRate
			if err := tx.Where("shop_id = ? AND wilaya_id = ?", shopID, wilayaID).First(&rate).Error; err != nil {
				return err
			}
			resolved, shipErr := services.ResolveShipping(rate, method)
			if shipErr != nil {
				return shipErr
			}
			shippingPrice = resolved
		}
		calculatedTotalPrice := shippingPrice
		newOrderItems := make([]models.OrderItem, 0)

		if len(body.Items) > 0 {
			if err := tx.Where("order_id = ?", order.ID).Delete(&models.OrderItem{}).Error; err != nil {
				return err
			}

			// Price and ProductID are looked up from the combination's own DB
			// row, scoped to this shop — never taken from the client body.
			comboIDs := make([]uuid.UUID, 0, len(body.Items))
			for _, item := range body.Items {
				comboID, parseErr := uuid.Parse(item.ProductVariantCombinationID)
				if parseErr != nil {
					return fmt.Errorf("invalid combination uuid provided: %s", item.ProductVariantCombinationID)
				}
				comboIDs = append(comboIDs, comboID)
			}

			comboByID, err := services.FetchCombinationsForShop(tx, shopID, comboIDs)
			if err != nil {
				return err
			}

			for i, item := range body.Items {
				combo, ok := comboByID[comboIDs[i]]
				if !ok {
					return fmt.Errorf("combination not found for this shop: %s", item.ProductVariantCombinationID)
				}

				newOrderItems = append(newOrderItems, models.OrderItem{
					OrderID:                     order.ID,
					ProductID:                   combo.ProductID,
					ProductVariantCombinationID: combo.ID,
					Quantity:                    item.Quantity,
					Price:                       combo.Price,
				})

				calculatedTotalPrice += combo.Price * float64(item.Quantity)
			}

			if err := tx.Create(&newOrderItems).Error; err != nil {
				return err
			}
		} else {
			var existingItems []models.OrderItem
			if err := tx.Where("order_id = ?", order.ID).Find(&existingItems).Error; err != nil {
				return err
			}

			for _, item := range existingItems {
				calculatedTotalPrice += item.Price * float64(item.Quantity)
			}
		}

		updates := map[string]any{
			"shipping_price": shippingPrice,
			"total_price":    calculatedTotalPrice,
		}

		if body.ShippingMethod != nil {
			updates["shipping_method"] = *body.ShippingMethod
		}

		if body.Note != nil {
			updates["note"] = *body.Note
		}

		if body.Ouvrable != nil {
			updates["ouvrable"] = *body.Ouvrable
		}

		if body.Fragile != nil {
			updates["fragile"] = *body.Fragile
		}

		if body.Essayable != nil {
			updates["essayable"] = *body.Essayable
		}

		if strings.TrimSpace(body.Status) != "" {
			updates["status"] = body.Status
		}

		if body.ReportedDate != nil {
			if strings.TrimSpace(*body.ReportedDate) == "" {
				updates["reported_date"] = nil
			} else if parsed, parseErr := time.Parse("2006-01-02", *body.ReportedDate); parseErr == nil {
				updates["reported_date"] = parsed
			}
		}

		if body.IsShipped != nil {
			updates["is_shipped"] = *body.IsShipped
			if *body.IsShipped && !order.IsShipped {
				if err := services.DecrementOrderItemsStock(tx, order.Items); err != nil {
					return err
				}
				updates["shipped_at"] = time.Now()
				if body.ShippedViaID != nil {
					if shippedViaID, parseErr := uuid.Parse(*body.ShippedViaID); parseErr == nil {
						updates["shipped_via_id"] = shippedViaID
					}
				}
			}
		}

		if err := tx.Model(&models.Order{}).
			Where("id = ? AND shop_id = ?", order.ID, shopID).
			Updates(updates).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "Order not found or does not belong to this shop",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed updating order securely inside database transaction",
			"error":   err.Error(),
		})
		return
	}

	if strings.TrimSpace(body.Status) != "" && body.Status != oldStatus {
		utils.LogAudit(c, "order.status_changed", "Order", &orderID, map[string]string{
			"from": oldStatus,
			"to":   body.Status,
		})
	}

	var updatedOrder models.Order
	if err := initializers.DB.
		Preload("Client").
		Preload("Items").
		Preload("Items.Product").
		Preload("Items.ProductVariantCombination").
		First(&updatedOrder, "id = ? AND shop_id = ?", orderID, shopID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Order updated but failed to reload final payload",
			"error":   err.Error(),
		})
		return
	}

	InvalidateDashboardCache(shopID)
	invalidateOrdersListCache(shopID)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Order was updated successfully",
		"data":    updatedOrder,
	})
}

// GetOrderStatusHistory lists who changed an order's status and when.
// Owner-only — route-gated so confirmatrices can't see who's watching them.
func GetOrderStatusHistory(c *gin.Context) {
	shopIDStr := c.Param("shopId")
	shopID, err := uuid.Parse(shopIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid or missing Shop ID",
		})
		return
	}

	orderIDStr := c.Param("id")
	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid Order ID format",
		})
		return
	}

	var order models.Order
	if err := initializers.DB.
		Select("id").
		First(&order, "id = ? AND shop_id = ?", orderID, shopID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "Order not found or does not belong to this shop",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Database error while looking up order",
			"error":   err.Error(),
		})
		return
	}

	var logs []models.AuditLog
	if err := initializers.DB.
		Where("target_type = ? AND target_id = ? AND action = ?", "Order", orderID, "order.status_changed").
		Order("created_at DESC").
		Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to fetch order status history",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    logs,
	})
}

func DeleteOrderByShopID(c *gin.Context) {
	shopIDStr := c.Param("shopId")
	shopID, err := uuid.Parse(shopIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid or missing Shop ID",
		})
		return
	}

	orderIDStr := c.Param("id")
	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid Order ID format",
		})
		return
	}

	var order models.Order
	if err := initializers.DB.
		First(&order, "id = ? AND shop_id = ?", orderID, shopID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "Order not found or does not belong to this shop",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Database error while finding order before deletion",
			"error":   err.Error(),
		})
		return
	}

	if err := initializers.DB.Delete(&order).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to delete order",
			"error":   err.Error(),
		})
		return
	}

	InvalidateDashboardCache(shopID)
	invalidateProductCaches(uuid.Nil, shopID)
	invalidateOrdersListCache(shopID)
	initializers.RClient.Del(initializers.Ctx, services.LandingPagesCacheKeyByShop(shopID))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Order was deleted successfully",
	})
}

// BanOrderClient permanently bans the fbp/ttp behind one order from ever
// placing a visible order again: every future order from that same browser
// id is auto-hidden and skips the pixel event, exactly like a cussword-flagged
// order — same silent treatment, just owner-triggered instead of automatic.
// Rejecting the request outright instead would tip the troll off to switch
// devices; silently no-op'ing keeps them ordering into a void.
func BanOrderClient(c *gin.Context) {
	shopIDStr := c.Param("shopId")
	shopID, err := uuid.Parse(shopIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid or missing Shop ID",
		})
		return
	}

	orderIDStr := c.Param("id")
	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid Order ID format",
		})
		return
	}

	var order models.Order
	if err := initializers.DB.First(&order, "id = ? AND shop_id = ?", orderID, shopID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "Order not found or does not belong to this shop",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Database error while looking up order",
			"error":   err.Error(),
		})
		return
	}

	platform, clientID := "", ""
	switch order.ConversionSource {
	case "facebook":
		platform, clientID = "facebook", order.FBp
	case "tiktok":
		platform, clientID = "tiktok", order.TTp
	}

	hasPlatformID := clientID != ""
	hasResolvableClient := order.ClientID != uuid.Nil

	if !hasPlatformID && !hasResolvableClient {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "This order has no trackable browser ID (organic order) — nothing to ban",
		})
		return
	}

	err = initializers.DB.Transaction(func(tx *gorm.DB) error {
		if hasPlatformID {
			var existing models.FlaggedClient
			alreadyBanned := tx.Where("shop_id = ? AND platform = ? AND client_id = ?", shopID, platform, clientID).
				First(&existing).Error == nil

			if !alreadyBanned {
				if err := tx.Create(&models.FlaggedClient{ShopID: shopID, Platform: platform, ClientID: clientID}).Error; err != nil {
					return err
				}
			}
		}

		// Complementary, not exclusive: also ban the Client row itself, so an
		// organic order (no fbp/ttp) is no longer permanently unbannable.
		if hasResolvableClient {
			if err := tx.Model(&models.Client{}).Where("id = ?", order.ClientID).Update("banned", true).Error; err != nil {
				return err
			}
		}

		return tx.Model(&models.Order{}).Where("id = ?", order.ID).Update("is_hidden", true).Error
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to ban client",
			"error":   err.Error(),
		})
		return
	}

	invalidateOrdersListCache(shopID)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Client banned — future orders from this browser will be silently hidden",
	})
}

type AssignOrderInput struct {
	MemberID *string `json:"memberId"`
}

// AssignOrderByShopID manually assigns/reassigns/clears which confirmatrice
// owns this order. memberID nil clears the assignment.
func AssignOrderByShopID(c *gin.Context) {
	shopIDStr := c.Param("shopId")
	shopID, err := uuid.Parse(shopIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid or missing Shop ID"})
		return
	}

	orderIDStr := c.Param("id")
	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid Order ID format"})
		return
	}

	var body AssignOrderInput
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Error while binding JSON request context", "error": err.Error()})
		return
	}

	var memberID *uuid.UUID
	if body.MemberID != nil && strings.TrimSpace(*body.MemberID) != "" {
		parsed, parseErr := uuid.Parse(*body.MemberID)
		if parseErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid member ID format"})
			return
		}
		memberID = &parsed
	}

	if err := services.AssignOrder(shopID, orderID, memberID); err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Order not found, or member is not a confirmatrice of this shop"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to assign order", "error": err.Error()})
		return
	}

	invalidateOrdersListCache(shopID)

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Order assignment updated"})
}

type BulkAssignOrdersInput struct {
	OrderIDs []string `json:"orderIds" binding:"required,min=1"`
	MemberID *string  `json:"memberId"`
}

// BulkAssignOrdersByShopID assigns/reassigns/clears a batch of orders in one
// request — the "worker called in sick, hand off her queue" path.
func BulkAssignOrdersByShopID(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid or missing Shop ID"})
		return
	}

	var body BulkAssignOrdersInput
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Error while binding JSON request context", "error": err.Error()})
		return
	}

	orderIDs := make([]uuid.UUID, 0, len(body.OrderIDs))
	for _, idStr := range body.OrderIDs {
		parsed, parseErr := uuid.Parse(idStr)
		if parseErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid order ID: " + idStr})
			return
		}
		orderIDs = append(orderIDs, parsed)
	}

	var memberID *uuid.UUID
	if body.MemberID != nil && strings.TrimSpace(*body.MemberID) != "" {
		parsed, parseErr := uuid.Parse(*body.MemberID)
		if parseErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid member ID format"})
			return
		}
		memberID = &parsed
	}

	assigned, err := services.BulkAssignOrders(shopID, orderIDs, memberID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Member is not a confirmatrice of this shop"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to bulk-assign orders", "error": err.Error()})
		return
	}

	invalidateOrdersListCache(shopID)

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Orders assigned", "assigned": assigned})
}
