package controllers

import (
	"bytes"
	"fmt"
	"io"
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

	// Nested client object matches your new JSON payload
	Client OrderClientInput `json:"client" binding:"required"`

	// Dive validates the nested slice
	Items []OrderItemInput `json:"items" binding:"required,min=1,dive"`
}

// Add this new struct for the client
type OrderClientInput struct {
	FullName      string `json:"fullName" binding:"required"`
	PhoneNumber   string `json:"phoneNumber" binding:"required"`
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
	Ouvrable       *bool             `json:"ouvrable"`
	Fragile        *bool             `json:"fragile"`
	Essayable      *bool             `json:"essayable"`
	IsShipped      *bool             `json:"isShipped"`
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

	baseQuery := initializers.DB.Model(&models.Order{}).Where("orders.shop_id = ?", shopID)

	if status != "" && status != "Tous" {
		baseQuery = baseQuery.Where("status = ?", status)
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

		calculatedTotalPrice += body.ShippingPrice

		order = models.Order{
			ShopID:         parsedShopID,
			ClientID:       client.ID,
			ShippingMethod: body.ShippingMethod,
			ShippingPrice:  body.ShippingPrice,
			TotalPrice:     calculatedTotalPrice,
			Status:         "En attente",
			Note:           body.Note,
			// FIX 3: Map the missing boolean flags from the body
			Ouvrable:         body.Ouvrable,
			Fragile:          body.Fragile,
			Essayable:        body.Essayable,
			FBclid:           body.FBclid,
			FBc:              body.FBc,
			FBp:              body.FBp,
			ConversionSource: body.ConversionSource,
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
	go func(orderID uuid.UUID, ua, ip string) {
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

		testCode := os.Getenv("FACEBOOK_TEST_CODE")

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

		if testCode == "" && fullOrder.Status != "Confirmé" {
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

		if fullOrder.Status == "Abandonné" {
			return
		}

		realtime.Broadcast <- realtime.Message{
			Event: "order_created",
			Data: map[string]any{
				"productName": mainProductName,
				"totalPrice":  fullOrder.TotalPrice,
				"itemsCount":  len(fullOrder.Items),
			},
		}

		if fullOrder.ConversionSource == "facebook" {
			var fbPixel models.Pixel
			pixelErr := initializers.DB.
				Where("shop_id = ? AND platform = ? AND is_active = ?", fullOrder.ShopID, "facebook", true).
				First(&fbPixel).Error

			// CAPI only runs when the shop configured an access token; otherwise the
			// browser pixel already fires Purchase client-side (see usePixelEvents.ts).
			if pixelErr == nil && fbPixel.HasAccessToken && fbPixel.AccessToken != "" {
				fbErr := utils.SendFacebookPurchase(
					fbPixel.PixelID,
					fbPixel.AccessToken,
					fullOrder.ID.String(),
					fullOrder.Client.FullName,
					fullOrder.Client.PhoneNumber,
					fullOrder.TotalPrice,
					"DZD",
					fullOrder.FBc,
					fullOrder.FBp,
					time.Now(),
					ua,
					ip,
					testCode,
				)
				if fbErr != nil {
					fmt.Println("Error passing payload downstream towards Facebook Graph servers:", fbErr)
				} else {
					fmt.Println("Facebook conversion parameters pushed smoothly")
				}
			}
		}
	}(order.ID, clientUserAgent, clientIP)

	InvalidateDashboardCache(parsedShopID)

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"message":  "The order was created successfully",
		"order_id": order.ID,
	})
}

func CreateZrOrder(c *gin.Context) {
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to parse the body",
		})
		return
	}

	zrApi := os.Getenv("ZR_EXPRESS_URL")
	token := os.Getenv("ZR_EXPRESS_TOKEN")
	key := os.Getenv("ZR_EXPRESS_KEY")

	req, err := http.NewRequest("POST", fmt.Sprint(zrApi, "/parcels"), bytes.NewBuffer(bodyBytes))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"success": false,
			"error":   "Failed to create api request",
		})
		return
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tenant", token)
	req.Header.Set("X-Api-Key", key)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"success": false,
			"error":   "Failed to contact external api",
		})
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to read the api response",
		})
		return
	}

	c.Data(resp.StatusCode, "application/json", respBody)
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

	err = initializers.DB.Transaction(func(tx *gorm.DB) error {
		var order models.Order
		if err := tx.
			Preload("Client").
			Preload("Items").
			First(&order, "id = ? AND shop_id = ?", orderID, shopID).Error; err != nil {
			return err
		}

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

		if body.IsShipped != nil {
			updates["is_shipped"] = *body.IsShipped
			if *body.IsShipped && !order.IsShipped {
				decrementOrderItemsStock(tx, order.Items)
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

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Order was deleted successfully",
	})
}
