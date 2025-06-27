package controllers

import (
	"net/http"

	"github.com/chtiwa/herbs-store-client/initializers"
	"github.com/chtiwa/herbs-store-client/models"
	"github.com/gin-gonic/gin"
)

// check admin
func CreateProduct(c *gin.Context) {
	var products []models.Product

	result := initializers.DB.Find(&products)

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "error while retrieving the products",
			"error":   result.Error.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    products,
	})
}
func GetProducts(c *gin.Context)   {}
func GetProduct(c *gin.Context)    {}
func UpdateProduct(c *gin.Context) {}
func DeleteProduct(c *gin.Context) {}
