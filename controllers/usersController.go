package controllers

import (
	"errors"
	"net/http"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/services"
	"github.com/chtiwa/dzbazar-server/utils"
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

	// firstName/lastName/phoneNumber/password are only used for the "create a
	// brand-new user" branch below; attaching an existing user (matched by
	// email) needs nothing but the email itself, so they can't be `required`.
	var body struct {
		FirstName   string `json:"firstName" binding:"omitempty"`
		LastName    string `json:"lastName" binding:"omitempty"`
		PhoneNumber string `json:"phoneNumber" binding:"omitempty"`
		Email       string `json:"email" binding:"required,email"`
		Password    string `json:"password" binding:"omitempty,min=6"`
		Role        string `json:"role" binding:"omitempty"`
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		RespondError(c, http.StatusBadRequest, "Invalid request body", err)
		return
	}
	body.Email = normalizeEmail(body.Email)

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
		RespondError(c, http.StatusInternalServerError, "Failed to verify plan limits", err)
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

	// An existing account (any shop, including another merchant or a
	// super-admin) must never be silently attached without consent — that
	// was an account-takeover path (see important.todo LAUNCH BLOCKER 2). A
	// proper invite/accept flow is a P1 follow-up; for now the caller has to
	// create a brand-new account for a brand-new email.
	if lookupErr == nil {
		tx.Rollback()
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"message": "This email already has an account",
		})
		return
	}

	if body.FirstName == "" || body.LastName == "" || body.PhoneNumber == "" || len(body.Password) < 6 {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "firstName, lastName, phoneNumber and a password (min 6 chars) are required to create a new user",
		})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), 10)
	if err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to hash the password",
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

// memberUpdateDecision is the pure authorization check for UpdateUserByShop:
// whether the caller (with role callerRole in this shop) may modify the
// target member (whose current shop role is targetCurrentRole and whose user
// ID is targetUserID), and whether email/password fields in the request may
// be applied. isSelf means the caller is editing their own account.
//
// Rules (see important.todo LAUNCH BLOCKER 2):
//   - email/password can only ever be changed by the account owner themself.
//   - only a shop owner may change anyone's role to/from "owner".
//   - only a shop owner may edit a member who currently holds "owner".
func memberUpdateDecision(callerRole, targetCurrentRole string, isSelf bool, newRole *string) (allowIdentityFields bool, forbidden bool, forbiddenReason string) {
	if !isSelf && targetCurrentRole == "owner" && callerRole != "owner" {
		return false, true, "You do not have permission to modify the shop owner"
	}
	if newRole != nil && (*newRole == "owner" || targetCurrentRole == "owner") && callerRole != "owner" {
		return false, true, "Only the shop owner can change ownership"
	}
	return isSelf, false, ""
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
		RespondError(c, http.StatusBadRequest, "Invalid request body", err)
		return
	}
	if body.Email != nil {
		normalized := normalizeEmail(*body.Email)
		body.Email = &normalized
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

	var targetMembership models.ShopMember
	if err := initializers.DB.Where("shop_id = ? AND user_id = ?", shopID, userID).First(&targetMembership).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Database error while retrieving membership",
		})
		return
	}

	requester := c.MustGet("user").(models.User)
	callerRole := c.MustGet("userShopRole").(string)
	isSelf := requester.ID == userID

	allowIdentityFields, forbidden, forbiddenReason := memberUpdateDecision(callerRole, targetMembership.Role, isSelf, body.Role)
	if forbidden {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": forbiddenReason,
		})
		return
	}
	if !allowIdentityFields {
		// Other members may only have first/last name, phone, role, active
		// changed — never someone else's email or password.
		body.Email = nil
		body.Password = nil
	}

	tx := initializers.DB.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to start transaction",
		})
		return
	}

	passwordChanged := false

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
		passwordChanged = true
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

	if passwordChanged && isSelf {
		utils.RevokeAllSessions(user.ID.String())
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

	requester := c.MustGet("user").(models.User)
	callerRole := c.MustGet("userShopRole").(string)
	if member.Role == "owner" && callerRole != "owner" && requester.ID != userID {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "You do not have permission to modify the shop owner",
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
		RespondError(c, http.StatusInternalServerError, "Error while retrieving product scope", err)
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
		RespondError(c, http.StatusBadRequest, "Error while binding JSON request context", err)
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
		RespondError(c, http.StatusInternalServerError, "Error while saving product scope", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Product scope updated"})
}
