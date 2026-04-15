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

	"github.com/chtiwa/lk-parfumo-server/initializers"
	"github.com/chtiwa/lk-parfumo-server/models"
	"github.com/chtiwa/lk-parfumo-server/realtime"
	"github.com/chtiwa/lk-parfumo-server/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func GetOrders(c *gin.Context) {
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

	db := initializers.DB.Model(&models.Order{})

	if status != "" && status != "Tous" {
		db = db.Where("status = ?", status)
	}

	if search != "" {
		likeSearch := "%" + search + "%"
		db = db.Where(
			"full_name LIKE ? OR phone_number LIKE ?",
			likeSearch, likeSearch,
		)
	}

	var totalRows int64
	if err := db.Count(&totalRows).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
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
	if err := db.Order("updated_at DESC").
		Limit(perPage).
		Offset(offset).
		Find(&orders).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Error retrieving the orders",
			"error":   err.Error(),
		})
		return
	}

	pagination := utils.GetPaginationData(page, int(totalPages), "/orders")
	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"message":    "Orders were retrieved successfully",
		"data":       orders,
		"pagination": pagination,
	})
}

func GetOrdersByStatus(c *gin.Context) {
	status := c.Query("status")
	perPage := 10
	page := 1
	pageString := c.Query("page")
	perPageString := c.Query("perPage")

	if pageString != "" {
		page, _ = strconv.Atoi(pageString)
	}
	if perPageString != "" {
		perPage, _ = strconv.Atoi(perPageString)
	}

	db := initializers.DB.Model(&models.Order{})

	if status != "" {
		db = db.Where("status = ?", status)
	}

	var totalRows int64
	result := db.Count(&totalRows)
	if result.Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Error while counting the orders",
			"error":   result.Error.Error(),
		})
		return
	}

	totalPages := math.Ceil(float64(totalRows) / float64(perPage))
	offset := (page - 1) * int(perPage)

	var orders []models.Order
	result = db.Order("updated_at DESC").Limit(int(perPage)).Offset(offset).Find(&orders)

	if result.Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Error while fetching the orders",
			"error":   result.Error.Error(),
		})
		return
	}

	pagination := utils.GetPaginationData(page, int(totalPages), "/orders/filters")

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"message":    "Orders were retrieved successfully",
		"data":       orders,
		"pagination": pagination,
	})
}

func GetOrdersBySearch(c *gin.Context) {
	search := c.Query("search")

	var orders []models.Order
	query := initializers.DB.Order("updated_at DESC")

	if search != "" {
		query = query.Where("full_name ILIKE ? OR phone_number ILIKE ?", "%"+search+"%", "%"+search+"%")
	}

	result := query.Limit(10).Find(&orders)

	if result.Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Error retrieving the orders",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Orders were retrieved successfully",
		"data":    orders,
	})
}

func CreateOrder(c *gin.Context) {
	var body struct {
		ShopName         string
		FullName         string
		PhoneNumber      string
		State            string
		StateNumber      string
		StateId          string
		City             string
		CityId           string
		HubId            string
		ProductID        string
		ProductName      string
		Variant          string
		VariantItemId    string
		ConversionSource string
		Price            float64
		ShippingMethod   string
		ShippingPrice    float64
		Quantity         uint
		TotalPrice       float64
		Status           string
		FBclid           string
		FBc              string
		FBp              string
		Ttclid           string
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Error while binding the body",
			"error":   err.Error(),
		})
		return
	}

	productID, err := uuid.Parse(body.ProductID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Something went wrong while parsing the product id",
			"error":   err.Error(),
		})
		return
	}

	variantItemID, err := uuid.Parse(body.VariantItemId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Something went wrong while parsing the variant item id",
			"error":   err.Error(),
		})
		return
	}

	order := models.Order{
		Client: models.Client{
			FullName:    body.FullName,
			PhoneNumber: body.PhoneNumber,
			State:       body.State,
			StateNumber: body.StateNumber,
			City:        body.City,
			CityId:      body.CityId,
			StateId:     body.StateId,
			HubId:       body.HubId,
		},
		ShopName:         body.ShopName,
		ProductID:        productID,
		ConversionSource: body.ConversionSource,
		ProductName:      body.ProductName,
		Price:            body.Price,
		Variant:          body.Variant,
		VariantItemId:    variantItemID,
		ShippingMethod:   body.ShippingMethod,
		ShippingPrice:    body.ShippingPrice,
		Quantity:         body.Quantity,
		TotalPrice:       body.TotalPrice,
		Status:           body.Status,
		FBclid:           body.FBclid,
		FBc:              body.FBc,
		FBp:              body.FBp,
		Ttclid:           body.Ttclid,
	}

	result := initializers.DB.Create(&order)
	if result.Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Error while creating the order",
			"error":   result.Error.Error(),
		})
		return
	}

	clientUserAgent := c.Request.UserAgent()
	clientIP := c.ClientIP()

	go func(o models.Order, userAgent, ip string) {
		testCode := os.Getenv("FACEBOOK_TEST_CODE")

		if testCode == "" && o.Status != "Confirmé" {
			if err := utils.SendEmail(
				o.FullName,
				o.PhoneNumber,
				o.State,
				o.City,
				o.ProductName,
				o.Variant,
				o.ShippingMethod,
				o.Quantity,
				o.Price,
				o.ShippingPrice,
				o.TotalPrice,
			); err != nil {
				fmt.Println("Error sending email:", err)
			}
		}

		if o.Status == "Abandonné" {
			return
		}

		realtime.Broadcast <- realtime.Message{
			Event: "order_created",
			Data: map[string]interface{}{
				"productName": o.ProductName,
			},
		}

		if o.ConversionSource == "facebook" {
			testCode := os.Getenv("FACEBOOK_TEST_CODE")
			err := utils.SendFacebookPurchase(
				o.ID.String(),
				o.Client.FullName,
				o.Client.PhoneNumber,
				o.TotalPrice,
				"DZD",
				o.FBc,
				o.FBp,
				o.CreatedAt,
				userAgent,
				ip,
				testCode,
			)
			if err != nil {
				fmt.Println("Error sending purchase event:", err)
			} else {
				fmt.Println("Facebook Event was sent successfully")
			}
		}

		// else if o.ConversionSource == "tiktok" {
		// 	testCode := os.Getenv("TIKTOK_TEST_CODE")
		// 	err := utils.SendTikTokPurchase(
		// 		o.ID.String(),
		// 		o.ProductName,
		// 		o.FullName,
		// 		o.Client.PhoneNumber,
		// 		o.Ttclid,
		// 		o.TotalPrice,
		// 		"DZD",
		// 		o.CreatedAt,
		// 		testCode,
		// 		userAgent,
		// 		ip,
		// 	)
		// 	if err != nil {
		// 		fmt.Println("Error sending purchase event:", err)
		// 	} else {
		// 		fmt.Println("Tiktok Event was sent successfully")
		// 	}
		// }
	}(order, clientUserAgent, clientIP)

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

func GetOrder(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Error while parsing the id",
		})
		return
	}

	var order models.Order
	result := initializers.DB.First(&order, id)

	if result.Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Error retrieving the orders",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Order was retrieved successfully",
		"data":    order,
	})
}

func UpdateOrder(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "error while parsing the id",
		})
		return
	}

	var body struct {
		ProductName    *string
		Quantity       *uint
		FullName       *string
		PhoneNumber    *string
		PhoneNumber2   *string
		State          *string
		StateNumber    *string
		City           *string
		Variant        *string
		Price          *float64
		ShippingMethod *string
		ShippingPrice  *float64
		TotalPrice     *float64
		Status         *string
		IsShipped      *bool
		Note           *string
	}

	if err = c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Error while parsing the body!",
			"error":   err.Error(),
		})
		return
	}

	var order models.Order
	if err := initializers.DB.First(&order, id).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Error while retrieving the order",
		})
		return
	}

	oldStatus := order.Status
	newStatus := oldStatus
	if body.Status != nil {
		newStatus = *body.Status
	}

	err = initializers.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&order).Updates(body).Error; err != nil {
			return err
		}

		if oldStatus != "Expedié" && newStatus == "Expedié" {
			result := tx.Model(&models.VariantItem{}).
				Where("id = ? AND quantity >= ?", order.VariantItemId, order.Quantity).
				UpdateColumn("quantity", gorm.Expr("quantity - ?", order.Quantity))

			if result.Error != nil {
				return result.Error
			}

			if result.RowsAffected == 0 {
				return fmt.Errorf("variant item not found or insufficient quantity")
			}
		}

		return nil
	})

	var variantItem models.VariantItem

	result := initializers.DB.First(&variantItem, order.VariantItemId)
	if result.Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Error while retrieving the variant item",
			"error":   result.Error.Error(),
		})
		return
	}

	if oldStatus != "Expedié" && newStatus == "Expedié" && variantItem.Quantity < 10 {
		go func(productName, variant string, quantity int) {
			if err := utils.SendLowStockEmail(productName, variant, int(quantity)); err != nil {
				fmt.Println("Error sending low stock email:", err)
			}
		}(order.ProductName, order.Variant, variantItem.Quantity)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Order update was successful",
		"data":    order,
	})
}

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
