package controllers

import (
	"errors"
	"net/http"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/services"
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

	query := initializers.DB.
		Joins("JOIN shop_members ON shop_members.user_id = users.id").
		Where("shop_members.shop_id = ?", shopID)

	if role := c.Query("role"); role != "" {
		query = query.Where("shop_members.role = ?", role)
	}

	err = query.
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

	requester := c.MustGet("user").(models.User)
	shopRole := c.MustGet("userShopRole").(string)
	if shopRole != "owner" && requester.ID != userID {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "You do not have permission to perform this action",
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
		Role        string `json:"role" binding:"omitempty"`
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
		body.Role = "moderator"
	} else {
		var n int64
		initializers.DB.Model(&models.ShopRole{}).Where("name = ?", body.Role).Count(&n)
		if n == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid role"})
			return
		}
	}

	if err := services.CheckUserLimit(shopID); err != nil {
		if errors.Is(err, services.ErrPlanLimitReached) {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "Staff member limit reached for your plan. Upgrade to add more users.",
				"code":    "PLAN_LIMIT_REACHED",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to verify plan limits", "error": err.Error()})
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

	// A person can belong to several shops (e.g. they already own one and are
	// being invited as Staff/Logistics into another). Email is globally unique
	// on users, so reuse the existing account instead of trying to re-create it.
	var existingUser models.User
	lookupErr := tx.Where("email = ?", body.Email).First(&existingUser).Error

	if lookupErr != nil && !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Error while checking for an existing account",
		})
		return
	}

	var user models.User

	if lookupErr == nil {
		user = existingUser

		var existingMembership models.ShopMember
		memErr := tx.Where("shop_id = ? AND user_id = ?", shopID, user.ID).First(&existingMembership).Error
		if memErr == nil {
			tx.Rollback()
			c.JSON(http.StatusConflict, gin.H{
				"success": false,
				"message": "This user is already a member of this shop",
			})
			return
		}
		if !errors.Is(memErr, gorm.ErrRecordNotFound) {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Error while checking existing membership",
			})
			return
		}
	} else {
		hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), 10)
		if err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Failed to hash the password",
			})
			return
		}

		user = models.User{
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

	message := "User was created successfully"
	if lookupErr == nil {
		message = "Existing user was added to the shop"
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": message,
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
		Role        *string `json:"role" binding:"omitempty"`
		Active      *bool   `json:"active"`
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
		var n int64
		initializers.DB.Model(&models.ShopRole{}).Where("name = ?", *body.Role).Count(&n)
		if n == 0 {
			tx.Rollback()
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid role"})
			return
		}

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

	if body.Active != nil {
		if err := tx.Model(&models.ShopMember{}).
			Where("shop_id = ? AND user_id = ?", shopID, userID).
			Update("active", *body.Active).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Failed to update membership availability",
			})
			return
		}
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

// GetConfirmatriceProducts lists the product scope for a confirmation-role
// member — an empty list means she sees nothing until scoped.
func GetConfirmatriceProducts(c *gin.Context) {
	member, ok := findShopMember(c)
	if !ok {
		return
	}

	var productIDs []uuid.UUID
	if err := initializers.DB.Model(&models.ConfirmatriceProduct{}).
		Where("shop_member_id = ?", member.ID).
		Pluck("product_id", &productIDs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Error while retrieving product scope", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": productIDs})
}

// SetConfirmatriceProducts replaces a member's product scope wholesale.
func SetConfirmatriceProducts(c *gin.Context) {
	member, ok := findShopMember(c)
	if !ok {
		return
	}

	var body struct {
		ProductIDs []string `json:"productIds"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Error while binding JSON request context", "error": err.Error()})
		return
	}

	rows := make([]models.ConfirmatriceProduct, 0, len(body.ProductIDs))
	for _, idStr := range body.ProductIDs {
		productID, parseErr := uuid.Parse(idStr)
		if parseErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid product ID: " + idStr})
			return
		}
		rows = append(rows, models.ConfirmatriceProduct{ShopMemberID: member.ID, ProductID: productID})
	}

	err := initializers.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("shop_member_id = ?", member.ID).Delete(&models.ConfirmatriceProduct{}).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.Create(&rows).Error
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Error while saving product scope", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Product scope updated"})
}
