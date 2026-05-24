package controllers

import (
	"errors"
	"net/http"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func GetUsersByShop(c *gin.Context) {
	shopIDParam := c.Param("shopId")
	shopID, err := uuid.Parse(shopIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid shop ID format",
		})
		return
	}

	var users []models.User

	err = initializers.DB.
		Joins("JOIN shop_members ON shop_members.user_id = users.id").
		Where("shop_members.shop_id = ?", shopID).
		Preload("Memberships", "shop_id = ?", shopID).
		Find(&users).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Error while retrieving users",
		})
		return
	}

	for i := range users {
		users[i].Password = ""
		users[i].EmailOTP = ""
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    users,
	})
}

func IndexUserByShop(c *gin.Context) {
	shopIDParam := c.Param("shopId")
	userIDParam := c.Param("id")

	shopID, err := uuid.Parse(shopIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid shop ID format",
		})
		return
	}

	userID, err := uuid.Parse(userIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid user ID format",
		})
		return
	}

	var user models.User

	err = initializers.DB.
		Joins("JOIN shop_members ON shop_members.user_id = users.id").
		Where("users.id = ? AND shop_members.shop_id = ?", userID, shopID).
		Preload("Memberships", "shop_id = ?", shopID).
		First(&user).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "User not found in this shop",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Error while retrieving user",
		})
		return
	}

	user.Password = ""
	user.EmailOTP = ""

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "The user was retrieved successfully",
		"data":    user,
	})
}

func CreateUserByShop(c *gin.Context) {
	shopIDParam := c.Param("shopId")
	shopID, err := uuid.Parse(shopIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid shop ID format",
		})
		return
	}

	var body struct {
		FirstName   string `json:"firstName" binding:"required"`
		LastName    string `json:"lastName" binding:"required"`
		PhoneNumber string `json:"phoneNumber" binding:"required"`
		Email       string `json:"email" binding:"required,email"`
		Password    string `json:"password" binding:"required,min=6"`
		Role        string `json:"role" binding:"omitempty,oneof=Owner Admin Moderator Staff User"`
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request body",
			"error":   err.Error(),
		})
		return
	}

	if body.Role == "" {
		body.Role = "Staff"
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), 10)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to hash the password",
		})
		return
	}

	tx := initializers.DB.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to start transaction",
		})
		return
	}

	user := models.User{
		FirstName:   body.FirstName,
		LastName:    body.LastName,
		PhoneNumber: body.PhoneNumber,
		Email:       body.Email,
		Password:    string(hash),
		Role:        body.Role,
		IsVerified:  true,
	}

	if err := tx.Create(&user).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Failed to create user (email may already exist)",
		})
		return
	}

	member := models.ShopMember{
		ShopID: shopID,
		UserID: user.ID,
		Role:   body.Role,
	}

	if err := tx.Create(&member).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Failed to attach user to shop",
		})
		return
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to commit user creation",
		})
		return
	}

	user.Password = ""
	user.EmailOTP = ""

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "User was created successfully",
		"data":    user,
	})
}

func UpdateUserByShop(c *gin.Context) {
	shopIDParam := c.Param("shopId")
	userIDParam := c.Param("id")

	shopID, err := uuid.Parse(shopIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid shop ID format",
		})
		return
	}

	userID, err := uuid.Parse(userIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid user ID format",
		})
		return
	}

	var body struct {
		FirstName   *string `json:"firstName"`
		LastName    *string `json:"lastName"`
		PhoneNumber *string `json:"phoneNumber"`
		Email       *string `json:"email" binding:"omitempty,email"`
		Password    *string `json:"password" binding:"omitempty,min=6"`
		Role        *string `json:"role" binding:"omitempty,oneof=Owner Admin Moderator Staff User"`
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request body",
			"error":   err.Error(),
		})
		return
	}

	var user models.User
	err = initializers.DB.
		Joins("JOIN shop_members ON shop_members.user_id = users.id").
		Where("users.id = ? AND shop_members.shop_id = ?", userID, shopID).
		First(&user).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "User not found in this shop",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Database error while retrieving user",
		})
		return
	}

	tx := initializers.DB.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to start transaction",
		})
		return
	}

	if body.FirstName != nil {
		user.FirstName = *body.FirstName
	}
	if body.LastName != nil {
		user.LastName = *body.LastName
	}
	if body.PhoneNumber != nil {
		user.PhoneNumber = *body.PhoneNumber
	}
	if body.Email != nil {
		user.Email = *body.Email
	}
	if body.Password != nil {
		hash, err := bcrypt.GenerateFromPassword([]byte(*body.Password), 10)
		if err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Failed to hash the password",
			})
			return
		}
		user.Password = string(hash)
	}

	if err := tx.Save(&user).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to update user",
		})
		return
	}

	if body.Role != nil {
		if err := tx.Model(&models.ShopMember{}).
			Where("shop_id = ? AND user_id = ?", shopID, userID).
			Update("role", *body.Role).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Failed to update membership role",
			})
			return
		}

		user.Role = *body.Role
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to commit user update",
		})
		return
	}

	user.Password = ""
	user.EmailOTP = ""

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "User was updated successfully",
		"data":    user,
	})
}

func DeleteUserByShop(c *gin.Context) {
	shopIDParam := c.Param("shopId")
	userIDParam := c.Param("id")

	shopID, err := uuid.Parse(shopIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid shop ID format",
		})
		return
	}

	userID, err := uuid.Parse(userIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid user ID format",
		})
		return
	}

	var member models.ShopMember
	err = initializers.DB.Where("shop_id = ? AND user_id = ?", shopID, userID).First(&member).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "User not found in this shop",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Error while retrieving membership",
		})
		return
	}

	if err := initializers.DB.Delete(&member).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Error while deleting the user from shop",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "User was removed from the shop",
	})
}
