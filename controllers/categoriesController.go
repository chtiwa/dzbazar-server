package controllers

import (
	"net/http"

	"github.com/chtiwa/herbs-store-client/initializers"
	"github.com/chtiwa/herbs-store-client/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func GetCategories(c *gin.Context) {
	var categories []models.Category

	result := initializers.DB.Find(&categories)

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Error while retrieving the categories",
			"error":   result.Error.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    categories,
	})
}

func GetProductsByCategoryId(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "error while parsing the id",
			"error":   err.Error(),
		})
		return
	}

	var category models.Category
	result := initializers.DB.Preload("Products").Preload("Products.Images").First(&category, id)
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
		"message": "products retrieved successfully",
		"data":    category.Products,
	})
}
