package controllers

import (
	"fmt"
	"net/http"

	"github.com/chtiwa/herbs-store-client/initializers"
	"github.com/chtiwa/herbs-store-client/models"
	"github.com/chtiwa/herbs-store-client/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func GetOrders(c *gin.Context) {
	var orders []models.Order
	result := initializers.DB.Find(&orders)

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
		FullName       string
		PhoneNumber    string
		State          string
		StateNumber    uint
		City           string
		ProductName    string
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

	order := models.Order{Client: models.Client{FullName: body.FullName, PhoneNumber: body.PhoneNumber, State: body.State, StateNumber: body.StateNumber, City: body.City}, ProductName: body.ProductName, Price: body.Price, ShippingMethod: body.ShippingMethod, ShippingPrice: body.ShippingPrice, Quantity: body.Quantity, TotalPrice: body.TotalPrice, Status: body.Status}

	result := initializers.DB.Create(&order)

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Error while creating the order",
		})
		return
	}

	go func() {
		err = utils.SendEmail(order.FullName, order.PhoneNumber, order.State, order.StateNumber, order.City, order.Price, order.ShippingMethod, order.ShippingPrice, order.Quantity, order.TotalPrice)

		if err != nil {
			fmt.Println(err)
		}
	}()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    order,
	})
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
