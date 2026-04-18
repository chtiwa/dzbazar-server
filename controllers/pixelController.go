package controllers

// import (
// 	"net/http"

// 	"github.com/chtiwa/lk-parfumo-server/initializers"
// 	"github.com/chtiwa/lk-parfumo-server/models"
// 	"github.com/gin-gonic/gin"
// 	"github.com/google/uuid"
// )

// func GetPixels(c *gin.Context) {
// 	// get the user from the context
// 	user, ok := c.Get("user")
// 	if !ok {
// 		c.JSON(http.StatusInternalServerError, gin.H{
// 			"success": false,
// 			"message": "Error while fetching the user",
// 		})
// 		return
// 	}

// 	userData, ok := user.(models.User)
// 	if !ok || userData.Role != "Admin" {
// 		c.AbortWithStatus(http.StatusUnauthorized)
// 		return
// 	}
// 	// get the pixels related to that shop id
// 	// var pixels []models.Pixel
// 	// result := initializers.DB.Find(&pixels).Where("shop_id = ?", userData.ShopID)
// 	// if result.Error != nil {

// 	// 	return
// 	// }
// }

// func GetPixel(c *gin.Context) {
// 	// get the user from context
// 	// get the shop id
// 	// get the pixel and verify that the shop ids match
// }

// func CreatePixel(c *gin.Context) {
// 	// get the user
// 	user, ok := c.Get("user")
// 	if !ok {
// 		c.JSON(http.StatusInternalServerError, gin.H{
// 			"success": false,
// 			"message": "Error while fetching the user",
// 		})
// 		return
// 	}

// 	userData, ok := user.(models.User)
// 	if !ok || userData.Role != "Admin" {
// 		c.AbortWithStatus(http.StatusUnauthorized)
// 		return
// 	}

// 	var body struct {
// 		PlatformID string `json:"platformId"`
// 		Name       string `json:"name"`
// 		PixelID    string `json:"pixelId"`
// 		Token      string `json:"token"`
// 	}

// 	if err := c.ShouldBindJSON(&body); err != nil {
// 		c.JSON(http.StatusBadRequest, gin.H{
// 			"success": false,
// 			"message": "Error while binding the body",
// 			"error":   err.Error(),
// 		})
// 		return
// 	}

// 	platformId, err := uuid.Parse(body.PlatformID)
// 	if err != nil {
// 		c.JSON(http.StatusInternalServerError, gin.H{
// 			"success": false,
// 			"message": "Error while parsing the platform id",
// 			"error":   err.Error(),
// 		})
// 		return
// 	}
// 	// pixel := models.Pixel{PlatformID: platformId, ShopID: userData.ShopID, Name: body.Name, PixelID: body.PixelID, Token: body.Token}
// 	pixel := models.Pixel{PlatformID: platformId, Name: body.Name, PixelID: body.PixelID, Token: body.Token}

// 	result := initializers.DB.Create(&pixel)
// 	if result.Error != nil {
// 		c.JSON(http.StatusInternalServerError, gin.H{
// 			"success": false,
// 			"message": "Error while creating the pixel",
// 			"error":   result.Error.Error(),
// 		})
// 		return
// 	}

// 	c.JSON(http.StatusCreated, gin.H{
// 		"success": true,
// 		"message": "Pixel created successfully",
// 		"data":    pixel,
// 	})
// }

// func UpdatePixel(c *gin.Context) {

// }

// func DeletePixel(c *gin.Context) {
// 	// get the user from the context
// 	// get the shop id
// 	// get the pixel and verify that the shop ids match
// 	// delete the pixel

// }
