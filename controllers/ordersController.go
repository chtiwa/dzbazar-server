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

// ItemInput handles individual nested checkout arrays cleanly
type ItemInput struct {
	ProductID     string  `json:"productId" binding:"required"`
	VariantString string  `json:"variantString" binding:"required"` // e.g., "Black / 41"
	Quantity      uint    `json:"quantity" binding:"required,min=1"`
	Price         float64 `json:"price" binding:"required"`
}

type CreateOrderInput struct {
	ShopID           string      `json:"shopId" binding:"required"`
	FullName         string      `json:"fullName" binding:"required"`
	PhoneNumber      string      `json:"phoneNumber" binding:"required"`
	State            string      `json:"state" binding:"required"` // e.g., Wilaya
	StateCode        string      `json:"stateCode"`
	City             string      `json:"city" binding:"required"` // e.g., Commune
	ShippingMethod   string      `json:"shippingMethod"`
	ShippingPrice    float64     `json:"shippingPrice"`
	ConversionSource string      `json:"conversionSource"`
	Note             string      `json:"note"`
	FBclid           string      `json:"fbclid"`
	FBc              string      `json:"fbc"`
	FBp              string      `json:"fbp"`
	Ttclid           string      `json:"ttclid"`
	Items            []ItemInput `json:"items" binding:"required,dive"`
}

func GetOrdersByShopID(c *gin.Context) {
	// 1. Enforce Multi-Tenancy: Extract and validate ShopID from route parameters or context middleware
	shopIDStr := c.Param("shopId")
	shopID, err := uuid.Parse(shopIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid or missing Shop ID",
		})
		return
	}

	// get the user
	userInterface, ok := c.Get("user")
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Error while fetching the user from context",
		})
		return
	}

	// parse the user info
	user, _ := userInterface.(models.User)

	// check if the user belongs to the shop
	if shopID != *user.ShopID {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Authentication required to access the shop info",
		})
		return
	}

	// 2. Parse and sanitize pagination parameters
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

	// 3. Thread-Safe Query Building: Scope everything cleanly to a local instance
	// Always enforce the tenant boundary right at initialization
	query := initializers.DB.Model(&models.Order{}).Where("shop_id = ?", shopID)

	// Apply optional filters
	if status != "" && status != "Tous" {
		query = query.Where("status = ?", status)
	}

	if search != "" {
		likeSearch := "%" + search + "%"
		// Using ILIKE for Postgres case-insensitive text search matching adjusted schema names
		query = query.Where(
			"client.full_name ILIKE ? OR client.phone_number LIKE ?",
			likeSearch, likeSearch,
		)
	}

	// 4. Execute safe count query
	var totalRows int64
	if err := query.Count(&totalRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Error while counting the orders",
			"error":   err.Error(),
		})
		return
	}

	// 5. Calculate structural pagination boundaries
	totalPages := int(math.Ceil(float64(totalRows) / float64(perPage)))
	if totalPages == 0 {
		totalPages = 1
	}

	if page > totalPages {
		page = totalPages
	}

	offset := (page - 1) * perPage

	// 6. Retrieve records safely matching exact parameters
	var orders []models.Order
	if err := query.Order("updated_at DESC").
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

func CreateOrder(c *gin.Context) {
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

	// Use an ACID Transaction block to securely update tables together
	err = initializers.DB.Transaction(func(tx *gorm.DB) error {
		// 1. Resolve or Create the Client based on phone matching parameters
		var client models.Client
		err := tx.Where("phone_number = ?", body.PhoneNumber).First(&client).Error
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				client = models.Client{
					FullName:    body.FullName,
					PhoneNumber: body.PhoneNumber,
					State:       body.State,
					StateCode:   body.StateCode,
					City:        body.City,
				}
				if createErr := tx.Create(&client).Error; createErr != nil {
					return createErr
				}
			} else {
				return err
			}
		}

		// 2. Loop and map checkout lines into database OrderItems array structures
		var orderItems []models.OrderItem
		var calculatedTotalPrice float64

		for _, item := range body.Items {
			prodID, parseErr := uuid.Parse(item.ProductID)
			if parseErr != nil {
				return fmt.Errorf("invalid product uuid provided: %s", item.ProductID)
			}

			orderItems = append(orderItems, models.OrderItem{
				ProductID:     prodID,
				VariantString: item.VariantString,
				Quantity:      item.Quantity,
				Price:         item.Price,
			})

			calculatedTotalPrice += item.Price * float64(item.Quantity)
		}

		// Append regional logistical metrics to complete calculations
		calculatedTotalPrice += body.ShippingPrice

		// 3. Assemble and build out structural Order model context boundaries
		order = models.Order{
			ShopID:           parsedShopID,
			ClientID:         client.ID,
			Client:           client, // Pre-assign for safe passing downstream to worker routines
			ShippingMethod:   body.ShippingMethod,
			ShippingPrice:    body.ShippingPrice,
			TotalPrice:       calculatedTotalPrice,
			Status:           "En attente",
			Note:             body.Note,
			FBclid:           body.FBclid,
			FBc:              body.FBc,
			FBp:              body.FBp,
			ConversionSource: body.ConversionSource,
			Items:            orderItems,
		}

		if createOrderErr := tx.Create(&order).Error; createOrderErr != nil {
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

	// 4. Fire-and-Forget Asynchronous Micro-tasks (Safely passed by deep copied value)
	go func(o models.Order, ua, ip string) {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("Recovered from panic inside order async tasks routine: %v\n", r)
			}
		}()

		testCode := os.Getenv("FACEBOOK_TEST_CODE")

		// Prepare a readable summary string for legacy email tracking channels
		mainProductName := "Multi-item Order"
		if len(o.Items) > 0 {
			mainProductName = fmt.Sprintf("%s (%s)", o.Items[0].VariantString, o.Items[0].ProductID.String())
		}

		if testCode == "" && o.Status != "Confirmé" {
			if emailErr := utils.SendOrderEmail(
				o.Client.FullName,
				o.Client.PhoneNumber,
				o.Client.State,
				o.Client.City,
				mainProductName,
				"See order components explicitly",
				o.ShippingMethod,
				1, // Normalized count metrics
				o.TotalPrice,
				o.ShippingPrice,
				o.TotalPrice,
			); emailErr != nil {
				fmt.Println("Error sending notification alert email flow:", emailErr)
			}
		}

		if o.Status == "Abandonné" {
			return
		}

		// Broadcast reactive event triggers through internal websockets pipelines
		realtime.Broadcast <- realtime.Message{
			Event: "order_created",
			Data: map[string]interface{}{
				"productName": mainProductName,
				"totalPrice":  o.TotalPrice,
				"itemsCount":  len(o.Items),
			},
		}

		// Send exact parameters over Meta Conversions Graph Engine
		if o.ConversionSource == "facebook" {
			fbErr := utils.SendFacebookPurchase(
				o.BaseModel.ID.String(), // Fallback dynamically to your BaseModel identity strategy
				o.Client.FullName,
				o.Client.PhoneNumber,
				o.TotalPrice,
				"DZD",
				o.FBc,
				o.FBp,
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
	}(order, clientUserAgent, clientIP)

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"message":  "The order was created successfully",
		"order_id": order.BaseModel.ID,
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

	// get the user
	userInterface, ok := c.Get("user")
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Error while fetching the user from context",
		})
		return
	}

	// parse the user info
	user, _ := userInterface.(models.User)

	// check if the user belongs to the shop
	if shopID != *user.ShopID {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Authentication required to access the shop info",
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

// func UpdateOrder(c *gin.Context) {
// 	id, err := uuid.Parse(c.Param("id"))
// 	if err != nil {
// 		c.JSON(http.StatusBadRequest, gin.H{
// 			"success": false,
// 			"message": "error while parsing the id",
// 		})
// 		return
// 	}

// 	var body struct {
// 		ProductName    *string
// 		Quantity       *uint
// 		FullName       *string
// 		PhoneNumber    *string
// 		PhoneNumber2   *string
// 		State          *string
// 		StateNumber    *string
// 		City           *string
// 		Variant        *string
// 		Price          *float64
// 		ShippingMethod *string
// 		ShippingPrice  *float64
// 		TotalPrice     *float64
// 		Status         *string
// 		IsShipped      *bool
// 		Note           *string
// 	}

// 	if err = c.ShouldBindJSON(&body); err != nil {
// 		c.JSON(http.StatusBadRequest, gin.H{
// 			"success": false,
// 			"message": "Error while parsing the body!",
// 			"error":   err.Error(),
// 		})
// 		return
// 	}

// 	var order models.Order
// 	if err := initializers.DB.First(&order, id).Error; err != nil {
// 		c.JSON(http.StatusBadRequest, gin.H{
// 			"success": false,
// 			"message": "Error while retrieving the order",
// 		})
// 		return
// 	}

// 	oldStatus := order.Status
// 	newStatus := oldStatus
// 	if body.Status != nil {
// 		newStatus = *body.Status
// 	}

// 	err = initializers.DB.Transaction(func(tx *gorm.DB) error {
// 		if err := tx.Model(&order).Updates(body).Error; err != nil {
// 			return err
// 		}

// 		if oldStatus != "Expedié" && newStatus == "Expedié" && order.VariantItemId != uuid.Nil {
// 			result := tx.Model(&models.VariantItem{}).
// 				Where("id = ? AND quantity >= ?", order.VariantItemId, order.Quantity).
// 				UpdateColumn("quantity", gorm.Expr("quantity - ?", order.Quantity))

// 			if result.Error != nil {
// 				return result.Error
// 			}

// 			if result.RowsAffected == 0 {
// 				return fmt.Errorf("variant item not found or insufficient quantity")
// 			}
// 		}

// 		return nil
// 	})

// 	var variantItem models.VariantItem

// 	result := initializers.DB.First(&variantItem, order.VariantItemId)
// 	if result.Error != nil {
// 		c.JSON(http.StatusBadRequest, gin.H{
// 			"success": false,
// 			"message": "Error while retrieving the variant item",
// 			"error":   result.Error.Error(),
// 		})
// 		return
// 	}

// 	c.JSON(http.StatusOK, gin.H{
// 		"success": true,
// 		"message": "Order update was successful",
// 		"data":    order,
// 	})
// }

func DeleteOrder(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))

	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "error while parsing the id",
		})
		return
	}

	result := initializers.DB.Delete(&models.Order{}, id)
	if result.Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Error while deleting the order",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "The order was deleted successfully",
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

// func GetOrdersBySearch(c *gin.Context) {
// 	search := c.Query("search")

// 	var orders []models.Order
// 	query := initializers.DB.Order("updated_at DESC")

// 	if search != "" {
// 		query = query.Where("full_name ILIKE ? OR phone_number ILIKE ?", "%"+search+"%", "%"+search+"%")
// 	}

// 	result := query.Limit(10).Find(&orders)

// 	if result.Error != nil {
// 		c.JSON(http.StatusBadRequest, gin.H{
// 			"success": false,
// 			"message": "Error retrieving the orders",
// 		})
// 		return
// 	}

// 	c.JSON(http.StatusOK, gin.H{
// 		"success": true,
// 		"message": "Orders were retrieved successfully",
// 		"data":    orders,
// 	})
// }

// func GetOrdersByStatusByShopID(c *gin.Context) {
// 	// get the shop id
// 	shopIDStr := c.Param("shopId")
// 	shopID, err := uuid.Parse(shopIDStr)
// 	if err != nil {
// 		c.JSON(http.StatusBadRequest, gin.H{
// 			"success": false,
// 			"message": "Invalid or missing Shop ID",
// 		})
// 		return
// 	}

// 	// get the user
// 	userInterface, ok := c.Get("user")
// 	if !ok {
// 		c.JSON(http.StatusInternalServerError, gin.H{
// 			"success": false,
// 			"message": "Error while fetching the user from context",
// 		})
// 		return
// 	}

// 	// parse the user info
// 	user, _ := userInterface.(models.User)

// 	// check if the user belongs to the shop
// 	if shopID != user.ShopID {
// 		c.JSON(http.StatusUnauthorized, gin.H{
// 			"success": false,
// 			"message": "Authentication required to access the shop info",
// 		})
// 		return
// 	}

// 	status := c.Query("status")
// 	perPage := 10
// 	page := 1
// 	pageString := c.Query("page")
// 	perPageString := c.Query("perPage")

// 	if pageString != "" {
// 		page, _ = strconv.Atoi(pageString)
// 	}
// 	if perPageString != "" {
// 		perPage, _ = strconv.Atoi(perPageString)
// 	}

// 	db := initializers.DB.Model(&models.Order{}).Where("shop_id = ?", shopID)

// 	if status != "" {
// 		db = db.Where("status = ?", status)
// 	}

// 	var totalRows int64
// 	result := db.Count(&totalRows)
// 	if result.Error != nil {
// 		c.JSON(http.StatusBadRequest, gin.H{
// 			"success": false,
// 			"message": "Error while counting the orders",
// 			"error":   result.Error.Error(),
// 		})
// 		return
// 	}

// 	totalPages := math.Ceil(float64(totalRows) / float64(perPage))
// 	offset := (page - 1) * int(perPage)

// 	var orders []models.Order
// 	result = db.Order("updated_at DESC").Limit(int(perPage)).Offset(offset).Find(&orders)

// 	if result.Error != nil {
// 		c.JSON(http.StatusBadRequest, gin.H{
// 			"success": false,
// 			"message": "Error while fetching the orders",
// 			"error":   result.Error.Error(),
// 		})
// 		return
// 	}

// 	pagination := utils.GetPaginationData(page, int(totalPages), "/orders/filters")

// 	c.JSON(http.StatusOK, gin.H{
// 		"success":    true,
// 		"message":    "Orders were retrieved successfully",
// 		"data":       orders,
// 		"pagination": pagination,
// 	})
// }
