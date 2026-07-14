package controllers

import (
	"fmt"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/realtime"
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
	CouponCode       string  `json:"couponCode"`

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

// decrementOrderItemsStock reduces the stock quantity of each ordered variant
// combination by the quantity ordered. Called once, when an order transitions
// to "shipped" for the first time.
func decrementOrderItemsStock(tx *gorm.DB, items []models.OrderItem) {
	for _, item := range items {
		tx.Model(&models.ProductVariantCombination{}).
			Where("id = ?", item.ProductVariantCombinationID).
			UpdateColumn("quantity", gorm.Expr("quantity - ?", item.Quantity))
	}
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

	baseQuery := initializers.DB.Model(&models.Order{}).Where("orders.shop_id = ? AND orders.is_hidden = ?", shopID, flaggedOnly)

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

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"message":    "Orders were retrieved successfully",
		"data":       orders,
		"pagination": pagination,
	})
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

	// Per-browser, per-shop 24h cooldown. Unlike the guards below, this is an
	// honest UX rule — tell the customer, don't fake success.
	if _, err := c.Cookie(orderCooldownCookie(parsedShopID)); err == nil {
		c.JSON(http.StatusTooManyRequests, gin.H{
			"success": false,
			"message": "لقد قمت بطلب من هذا المتجر مؤخراً. يرجى الانتظار قبل إرسال طلب آخر.",
			"code":    "ORDER_COOLDOWN",
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

	// Phone-per-shop rate limit: 1 order per 30 min. Silent drop on breach.
	phoneKey := phoneOrderKey(parsedShopID, body.Client.PhoneNumber)
	set, redisErr := initializers.RClient.SetNX(initializers.Ctx, phoneKey, 1, phoneOrderWindow).Result()
	if redisErr == nil && !set {
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

		// 2. Loop and map checkout lines into database OrderItems structures
		for _, item := range body.Items {
			prodID, parseErr := uuid.Parse(item.ProductID)
			if parseErr != nil {
				return fmt.Errorf("invalid product uuid provided: %s", item.ProductID)
			}

			prodVarComID, parseErr := uuid.Parse(item.ProductVariantCombinationID)
			if parseErr != nil {
				return fmt.Errorf("invalid combination uuid provided: %s", item.ProductVariantCombinationID)
			}

			orderItems = append(orderItems, models.OrderItem{
				ProductID:                   prodID,
				ProductVariantCombinationID: prodVarComID,
				Quantity:                    item.Quantity,
				Price:                       item.Price,
			})

			calculatedTotalPrice += item.Price * float64(item.Quantity)
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

		calculatedTotalPrice += body.ShippingPrice

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

		order = models.Order{
			ShopID:         parsedShopID,
			ClientID:       client.ID,
			ShippingMethod: body.ShippingMethod,
			ShippingPrice:  body.ShippingPrice,
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
			IsHidden:         containsCussword,
			ClientIP:         clientIP,
			ClientUserAgent:  clientUserAgent,
			Items:            orderItems,
		}

		if createOrderErr := tx.Omit("Client", "Items.Product", "Items.ProductVariantCombination").Create(&order).Error; createOrderErr != nil {
			return createOrderErr
		}

		// A completed order means this phone number is no longer "abandoned" —
		// soft-delete any prior abandoned lead so it stops being counted/shown as one.
		if delErr := tx.Where("shop_id = ? AND phone_number = ?", parsedShopID, body.Client.PhoneNumber).
			Delete(&models.AbandonedLead{}).Error; delErr != nil {
			return delErr
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

	// 3. Asynchronous Micro-tasks (Safe fire-and-forget routines)
	go func(orderID uuid.UUID) {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("Recovered from panic inside order async tasks routine: %v\n", r)
			}
		}()

		var fullOrder models.Order
		preloadErr := initializers.DB.
			Preload("Client").
			Preload("Items").
			Preload("Items.Product").
			Preload("Items.ProductVariantCombination").
			First(&fullOrder, "id = ?", orderID).Error

		if preloadErr != nil {
			fmt.Printf("Error hydrating order details context for async tasks: %v\n", preloadErr)
			return
		}

		isProduction := os.Getenv("APP_ENV") == "production"

		mainProductName := "Multi-item Order"
		if len(fullOrder.Items) > 0 {
			item := fullOrder.Items[0]
			if item.Product.Title != "" {
				mainProductName = item.Product.Title
			} else {
				comboStr := item.ProductVariantCombination.CombinationString
				if comboStr == "" {
					comboStr = "Standard"
				}
				mainProductName = fmt.Sprintf("Product SKU Variant: %s", comboStr)
			}
		}

		// Cussword orders are silently accepted for the client but kept out of
		// sight of the admin entirely — no notification email, no live broadcast.
		if isProduction && fullOrder.Status != "Confirmé" && !fullOrder.IsHidden {
			var shop models.Shop
			initializers.DB.Select("name").First(&shop, "id = ?", fullOrder.ShopID)

			var members []models.ShopMember
			initializers.DB.Preload("User").Where("shop_id = ?", fullOrder.ShopID).Find(&members)

			recipients := make([]string, 0, len(members))
			seen := make(map[string]struct{})
			for _, m := range members {
				if m.User.Email != "" {
					if _, ok := seen[m.User.Email]; !ok {
						seen[m.User.Email] = struct{}{}
						recipients = append(recipients, m.User.Email)
					}
				}
			}

			if emailErr := utils.SendOrderEmail(
				shop.Name,
				recipients,
				fullOrder.Client.FullName,
				fullOrder.Client.PhoneNumber,
				fullOrder.Client.State,
				fullOrder.Client.City,
				mainProductName,
				"See order components explicitly",
				fullOrder.ShippingMethod,
				1,
				fullOrder.TotalPrice,
				fullOrder.ShippingPrice,
				fullOrder.TotalPrice,
			); emailErr != nil {
				fmt.Println("Error sending notification alert email flow:", emailErr)
			}
		}

		if fullOrder.Status == "Abandonné" || fullOrder.IsHidden {
			return
		}

		realtime.Broadcast <- realtime.Message{
			Event:  "order_created",
			ShopID: fullOrder.ShopID.String(),
			Data: map[string]any{
				"productName": mainProductName,
				"totalPrice":  fullOrder.TotalPrice,
				"itemsCount":  len(fullOrder.Items),
			},
		}

		// Meta Purchase fires here, at creation. Firing it on confirmation instead
		// tanked ad delivery — Meta's optimization needs the signal close to the
		// click, not hours later once an admin gets to it.
		if fullOrder.ConversionSource == "facebook" {
			var px models.Pixel
			pixelErr := initializers.DB.
				Where("shop_id = ? AND platform = ? AND is_active = ?", fullOrder.ShopID, "facebook", true).
				First(&px).Error

			if pixelErr == nil && px.HasAccessToken && px.AccessToken != "" {
				testCode := ""
				if strings.Contains(strings.ToLower(fullOrder.Client.FullName), "test") {
					testCode = os.Getenv("FACEBOOK_TEST_CODE")
				}

				fbErr := utils.SendFacebookPurchase(
					px.PixelID,
					px.AccessToken,
					fullOrder.ID.String(),
					fullOrder.Client.FullName,
					fullOrder.Client.PhoneNumber,
					fullOrder.TotalPrice,
					"DZD",
					fullOrder.FBc,
					fullOrder.FBp,
					fullOrder.CreatedAt,
					fullOrder.ClientUserAgent,
					fullOrder.ClientIP,
					testCode,
				)
				if fbErr != nil {
					fmt.Println("Meta CAPI purchase send failed:", fbErr)
				} else {
					initializers.DB.Model(&models.Order{}).Where("id = ?", orderID).
						Update("meta_purchase_sent_at", time.Now())
				}
			}
		}
	}(order.ID)

	InvalidateDashboardCache(parsedShopID)
	invalidateProductCaches(uuid.Nil, parsedShopID)
	initializers.RClient.Del(initializers.Ctx, landingPagesCacheKeyByShop(parsedShopID))

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

		shippingPrice := order.ShippingPrice
		if body.ShippingPrice != nil {
			shippingPrice = *body.ShippingPrice
		}
		calculatedTotalPrice := shippingPrice
		newOrderItems := make([]models.OrderItem, 0)

		if len(body.Items) > 0 {
			if err := tx.Where("order_id = ?", order.ID).Delete(&models.OrderItem{}).Error; err != nil {
				return err
			}

			for _, item := range body.Items {
				prodID, parseErr := uuid.Parse(item.ProductID)
				if parseErr != nil {
					return fmt.Errorf("invalid product uuid provided: %s", item.ProductID)
				}

				prodVarComID, parseErr := uuid.Parse(item.ProductVariantCombinationID)
				if parseErr != nil {
					return fmt.Errorf("invalid combination uuid provided: %s", item.ProductVariantCombinationID)
				}

				newOrderItems = append(newOrderItems, models.OrderItem{
					OrderID:                     order.ID,
					ProductID:                   prodID,
					ProductVariantCombinationID: prodVarComID,
					Quantity:                    item.Quantity,
					Price:                       item.Price,
				})

				calculatedTotalPrice += item.Price * float64(item.Quantity)
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
				decrementOrderItemsStock(tx, order.Items)
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
	initializers.RClient.Del(initializers.Ctx, landingPagesCacheKeyByShop(shopID))

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

	if clientID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "This order has no trackable browser ID (organic order) — nothing to ban",
		})
		return
	}

	err = initializers.DB.Transaction(func(tx *gorm.DB) error {
		var existing models.FlaggedClient
		alreadyBanned := tx.Where("shop_id = ? AND platform = ? AND client_id = ?", shopID, platform, clientID).
			First(&existing).Error == nil

		if !alreadyBanned {
			if err := tx.Create(&models.FlaggedClient{ShopID: shopID, Platform: platform, ClientID: clientID}).Error; err != nil {
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

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Client banned — future orders from this browser will be silently hidden",
	})
}
