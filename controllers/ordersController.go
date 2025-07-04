package controllers

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"

	"github.com/chtiwa/herbs-store-client/initializers"
	"github.com/chtiwa/herbs-store-client/models"
	"github.com/chtiwa/herbs-store-client/realtime"
	"github.com/chtiwa/herbs-store-client/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func GetOrders(c *gin.Context) {
	page := 1
	pageString := c.Query("page")

	if pageString != "" {
		page, _ = strconv.Atoi(pageString)
	}

	var totalRows int64
	result := initializers.DB.Model(&models.Order{}).Count(&totalRows)
	if result.Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Error white counting the orders",
		})
		return
	}

	perPage := 10.0
	totalPages := math.Ceil(float64(totalRows) / perPage)

	offset := (page - 1) * int(perPage)

	var orders []models.Order
	result = initializers.DB.Order("updated_at DESC").Limit(int(perPage)).Offset(offset).Find(&orders)
	pagination := utils.GetPaginationData(page, int(totalPages), "/orders")

	if result.Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Error retrieving the orders",
		})
		return
	}

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
		ShopName       string
		FullName       string
		PhoneNumber    string
		State          string
		StateNumber    uint
		City           string
		ProductName    string
		Variant        string
		Price          float64
		ShippingMethod string
		ShippingPrice  float64
		Quantity       uint
		TotalPrice     float64
		Status         string // to check if the order is abandoned
	}

	err := c.ShouldBindJSON(&body)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Error while binding the body",
			"error":   err,
		})
		return
	}

	order := models.Order{Client: models.Client{FullName: body.FullName, PhoneNumber: body.PhoneNumber, State: body.State, StateNumber: body.StateNumber, City: body.City}, ShopName: body.ShopName, ProductName: body.ProductName, Price: body.Price, Variant: body.Variant, ShippingMethod: body.ShippingMethod, ShippingPrice: body.ShippingPrice, Quantity: body.Quantity, TotalPrice: body.TotalPrice, Status: body.Status}

	result := initializers.DB.Create(&order)

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Error while creating the order",
		})
		return
	}

	go func() {
		err = utils.SendEmail(order.FullName, order.PhoneNumber, order.State, order.City, order.ProductName, order.Variant, order.ShippingMethod, order.Quantity, order.Price, order.ShippingPrice, order.TotalPrice)

		realtime.Broadcast <- realtime.Message{
			Event: "order_created",
			Data: map[string]interface{}{
				"productName": order.ProductName,
			},
		}

		fmt.Println("event broadcast")

		if err != nil {
			fmt.Println(err)
		}
	}()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    order,
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

	procolisApi := os.Getenv("PROCOLIS_URL")
	token := os.Getenv("TOKEN")
	key := os.Getenv("KEY")

	req, err := http.NewRequest("POST", procolisApi, bytes.NewBuffer(bodyBytes))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"success": false,
			"error":   "Failed to create api request",
		})
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("token", token)
	req.Header.Set("key", key)

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
		c.JSON(http.StatusOK, gin.H{
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
		State          *string
		StateNumber    *uint
		City           *string
		Price          *float64
		ShippingMethod *string
		ShippingPrice  *float64
		TotalPrice     *float64
		Status         *string
	}
	err = c.ShouldBindJSON(&body)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Error while parsing the body!",
		})
		return
	}

	var order models.Order
	result := initializers.DB.First(&order, id)

	if result.Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Error while retrieving the order",
		})
		return
	}

	result = initializers.DB.Model(&order).Updates(body)
	if result.Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Error while retrieving the order",
		})
		return
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
