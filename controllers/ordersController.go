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
	ShippingMethod   string  `json:"shippingMethod"`
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
	FullName    string `json:"fullName" binding:"required"`
	PhoneNumber string `json:"phoneNumber" binding:"required"`
	State       string `json:"state" binding:"required"`
	StateCode   string `json:"stateCode"`
	City        string `json:"city" binding:"required"`
}

// Replace your old item struct with this
type OrderItemInput struct {
	ProductID                   string  `json:"productId" binding:"required"`
	ProductVariantCombinationID string  `json:"productVariantCombinationID" binding:"required"` // Match JSON exactly
	Quantity                    uint    `json:"quantity" binding:"required,min=1"`
	Price                       float64 `json:"price" binding:"required"`
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

	query := initializers.DB.Model(&models.Order{}).Where("shop_id = ?", shopID)

	if status != "" && status != "Tous" {
		query = query.Where("status = ?", status)
	}

	if search != "" {
		likeSearch := "%" + search + "%"
		query = query.
			Joins("JOIN clients client ON client.id = orders.client_id").
			Where("client.full_name ILIKE ? OR client.phone_number LIKE ?", likeSearch, likeSearch)
	}

	var totalRows int64
	if err := query.Count(&totalRows).Error; err != nil {
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
	if err := initializers.DB.
		Where("shop_id = ?", shopID).
		Scopes(func(db *gorm.DB) *gorm.DB {
			if status != "" && status != "Tous" {
				db = db.Where("status = ?", status)
			}
			if search != "" {
				likeSearch := "%" + search + "%"
				db = db.Joins("JOIN clients client ON client.id = orders.client_id").
					Where("client.full_name ILIKE ? OR client.phone_number LIKE ?", likeSearch, likeSearch)
			}
			return db
		}).
		Preload("Client").
		Preload("Items").
		Preload("Items.Product").
		Preload("Items.ProductVariantCombination").
		Order("updated_at DESC").
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
					// FIX 2: Assign the ShopID to the new client!
					ShopID:      parsedShopID,
					FullName:    body.Client.FullName,
					PhoneNumber: body.Client.PhoneNumber,
					State:       body.Client.State,
					StateCode:   body.Client.StateCode,
					City:        body.Client.City,
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
			Preload("Items.ProductVariantCombination").
			First(&fullOrder, "id = ?", orderID).Error

		if preloadErr != nil {
			fmt.Printf("Error hydrating order details context for async tasks: %v\n", preloadErr)
			return
		}

		testCode := os.Getenv("FACEBOOK_TEST_CODE")

		mainProductName := "Multi-item Order"
		if len(fullOrder.Items) > 0 {
			comboStr := fullOrder.Items[0].ProductVariantCombination.CombinationString
			if comboStr == "" {
				comboStr = "Standard"
			}
			mainProductName = fmt.Sprintf("Product SKU Variant: %s", comboStr)
		}

		if testCode == "" && fullOrder.Status != "Confirmé" {
			if emailErr := utils.SendOrderEmail(
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
			Data: map[string]interface{}{
				"productName": mainProductName,
				"totalPrice":  fullOrder.TotalPrice,
				"itemsCount":  len(fullOrder.Items),
			},
		}

		if fullOrder.ConversionSource == "facebook" {
			fbErr := utils.SendFacebookPurchase(
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
	}(order.ID, clientUserAgent, clientIP)

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
	// 1. Extract and validate Shop ID from route params or context middleware to enforce multi-tenancy boundaries
	shopIDStr := c.Param("shopId")
	shopID, err := uuid.Parse(shopIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid or missing Shop ID parameter",
		})
		return
	}

	// 2. Extract and validate Order ID
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

	// 3. Thread-safe query execution with deep eager loading
	// We pass .Preload("Items.Product") if you ever need to display product images/metadata on the order detail page
	err = initializers.DB.Model(&models.Order{}).
		Where("id = ? AND shop_id = ?", orderID, shopID).
		Preload("Client").
		Preload("Items").
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

	fmt.Println(body)

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
