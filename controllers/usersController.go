package controllers

import (
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func GetUsersByShop(c *gin.Context) {
	var users []models.User

	// 1. Extract the shopId from the URL parameters
	shopID := c.Param("shopId")

	// Optional but recommended: check if shopId is empty
	if shopID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "shopId parameter is required",
		})
		return
	}

	// 2. Add the Where clause to filter by shopId
	// Note: ensure "shop_id" matches your actual database column name
	result := initializers.DB.Where("shop_id = ?", shopID).Find(&users)

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Error while retrieving the users",
		})
		return // 3. Added the missing return statement here
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    users,
	})
}

func IndexUserByShop(c *gin.Context) {
	// 1. Extract both parameters from the URL
	shopID := c.Param("shopId")
	userIDParam := c.Param("id")

	if shopID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "shopId parameter is required",
		})
		return
	}

	id, err := uuid.Parse(userIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid user ID format",
		})
		return
	}

	var user models.User

	// 2. Query for BOTH the user ID and the shop ID simultaneously.
	// Note: Ensure "shop_id" matches your exact database column name.
	result := initializers.DB.Where("id = ? AND shop_id = ?", id, shopID).First(&user)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			// 3. Return a generic 404. This hides whether the user doesn't exist at all,
			// or if they just belong to a different shop (good security practice).
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "User not found in this shop",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Error while retrieving the user",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "The user was retrieved successfully",
		"data":    user,
	})
}

func CreateUserByShop(c *gin.Context) {
	// 1. Added missing fields and strict validation tags
	var body struct {
		ShopID      string `json:"shopId" binding:"required,uuid"` // Assuming admin must assign them to a shop
		FirstName   string `json:"firstName" binding:"required"`
		LastName    string `json:"lastName" binding:"required"`
		PhoneNumber string `json:"phoneNumber" binding:"required"`
		Email       string `json:"email" binding:"required,email"`
		Password    string `json:"password" binding:"required,min=6"`
		Role        string `json:"role" binding:"omitempty,oneof=Admin Moderator User"`
	}

	err := c.ShouldBindJSON(&body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request body",
			"error":   err.Error(), // Helpful to see exactly which validation failed
		})
		return // CRITICAL: Added missing return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), 10)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to hash the password",
		})
		return
	}

	if body.Role == "" {
		body.Role = "User"
	}

	// Parse the ShopID from the body string to a UUID
	// shopUUID, _ := uuid.Parse(body.ShopID)

	// 2. Added all the fields required by your GORM 'not null' constraints
	user := models.User{
		// ShopID:      &shopUUID,
		FirstName:   body.FirstName,
		LastName:    body.LastName,
		PhoneNumber: body.PhoneNumber,
		Email:       body.Email,
		Password:    string(hash),
		Role:        body.Role,
		// Since an admin is creating this user, you might want to bypass OTP and auto-verify them:
		IsVerified: true,
	}

	result := initializers.DB.Create(&user)
	if result.Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Failed to create the user (Email may already exist)",
		})
		return
	}

	// 3. Clear the password hash before sending the user data back to the client
	user.Password = ""

	c.JSON(http.StatusCreated, gin.H{ // Changed to 201 Created
		"success": true,
		"message": "User was created successfully",
		"data":    user,
	})
}

func UpdateUserByShop(c *gin.Context) {
	// 1. Secure the endpoint by enforcing the Shop ID scope
	shopID := c.Param("shopId")
	userIDParam := c.Param("id")

	id, err := uuid.Parse(userIDParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid user ID format",
		})
		return
	}

	// 2. Use pointers to detect exactly what the client wants to update
	var body struct {
		FirstName *string `json:"firstName"`
		LastName  *string `json:"lastName"`
		Email     *string `json:"email"`
		Password  *string `json:"password" binding:"omitempty,min=6"`
		Role      *string `json:"role" binding:"omitempty,oneof=Admin Moderator User"`
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

	// 3. Find the user, ensuring they belong to this specific shop
	result := initializers.DB.Where("id = ? AND shop_id = ?", id, shopID).First(&user)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
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

	// 4. Safely apply only the fields that were provided in the JSON body
	if body.FirstName != nil {
		user.FirstName = *body.FirstName
	}
	if body.LastName != nil {
		user.LastName = *body.LastName
	}
	if body.Email != nil {
		// TODO : add email re-verification flow post-MVP
		user.Email = *body.Email
	}
	if body.Password != nil {
		hash, err := bcrypt.GenerateFromPassword([]byte(*body.Password), 10)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Failed to hash the password",
			})
			return
		}
		user.Password = string(hash)
	}
	if body.Role != nil {
		user.Role = *body.Role
	}

	// Save the explicitly updated fields back to the database
	if err := initializers.DB.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to update user",
		})
		return
	}

	// 5. Clear sensitive data before sending the response
	user.Password = ""
	user.EmailOTP = ""

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "User was updated successfully",
		"data":    user,
	})
}

func Login(c *gin.Context) {
	// 1. Added validation tags
	var body struct {
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request body",
		})
		return
	}

	var user models.User
	// 2. Used .Where() for consistency, though your string approach was also fine
	result := initializers.DB.Where("email = ?", body.Email).First(&user)
	if result.Error != nil {
		c.JSON(http.StatusUnauthorized, gin.H{ // Changed to 401
			"success": false,
			"message": "Invalid email or password",
		})
		return
	}

	// 3. CRITICAL: Prevent login if the user hasn't verified their OTP
	if !user.IsVerified {
		c.JSON(http.StatusForbidden, gin.H{ // 403 Forbidden
			"success": false,
			"message": "Please verify your email address before logging in",
		})
		return
	}

	err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(body.Password))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{ // Changed to 401
			"success": false,
			"message": "Invalid email or password",
		})
		return
	}

	// generate the access and refresh token
	refreshToken := utils.GenerateToken(user.ID, 60*60*24*30, user.Role)
	accessToken := utils.GenerateToken(user.ID, 60*15, user.Role)

	refreshTokenString, err := refreshToken.SignedString([]byte(os.Getenv("JWT_SECRET")))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{ // Changed to 500
			"success": false,
			"message": "Failed to create the refresh token",
		})
		return
	}

	accessTokenString, err := accessToken.SignedString([]byte(os.Getenv("JWT_SECRET")))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{ // Changed to 500
			"success": false,
			"message": "Failed to create the access token",
		})
		return
	}

	// 4. Make the Secure flag dynamic based on your environment
	// Assuming you have an env var like APP_ENV=production
	isProduction := os.Getenv("APP_ENV") == "production"

	c.SetSameSite(http.SameSiteLaxMode)

	// c.SetCookie(name, value, maxAge, path, domain, secure, httpOnly)
	c.SetCookie("RefreshToken", refreshTokenString, 60*60*24*30, "/", "", isProduction, true)
	c.SetCookie("AccessToken", accessTokenString, 60*15, "/", "", isProduction, true)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"role":    user.Role,
	})
}

func Validate(c *gin.Context) {
	role, ok := c.Get("role")
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Error while fetching the role from context",
		})
		return
	}

	userInterface, ok := c.Get("user")
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Error while fetching the user from context",
		})
		return
	}

	// Safest practice: Cast the interface{} to your User model
	// and clear sensitive fields before sending it to the client
	user, isUser := userInterface.(models.User)
	if isUser {
		user.Password = ""
		user.EmailOTP = ""
		// Clear any other sensitive fields here
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"role":    role,
		"user":    user, // Now safe to return
	})
}

func Logout(c *gin.Context) {
	// 1. Match the Secure flag logic from your Login function
	isProduction := os.Getenv("APP_ENV") == "production"

	// 2. Use -1 for maxAge to force immediate deletion.
	// 3. Best Practice: Set the path to "/" (Make sure to update your Login func to use "/" too!)
	c.SetCookie("RefreshToken", "", -1, "/", "", isProduction, true)
	c.SetCookie("AccessToken", "", -1, "/", "", isProduction, true)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Logged out successfully", // Fixed typo
	})
}

func DeleteUserByShop(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Error while parsing the id",
		})
		return
	}

	result := initializers.DB.Delete(&models.User{}, id)
	if result.Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Error while deleting the user",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "User was deleted",
	})
}

func SignUp(c *gin.Context) {
	// 1. Added validation tags
	var body struct {
		FirstName   string `json:"firstName" binding:"required"`
		LastName    string `json:"lastName" binding:"required"`
		PhoneNumber string `json:"phoneNumber" binding:"required"`
		Email       string `json:"email" binding:"required,email"`
		Password    string `json:"password" binding:"required,min=6"`
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{ // Changed to 400
			"success": false,
			"message": "Invalid request body",
			"error":   err.Error(),
		})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), 10)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to process password",
		})
		return
	}

	otp := utils.GenerateOTP()
	expiresAt := time.Now().Add(15 * time.Minute)
	user := models.User{
		FirstName:         body.FirstName,
		LastName:          body.LastName,
		PhoneNumber:       body.PhoneNumber,
		Email:             body.Email,
		Password:          string(hash),
		EmailOTP:          otp,
		EmailOTPExpiresAt: &expiresAt,
		Role:              "Admin",
	}

	// 2. Consider using a DB transaction here if email sending is prone to failure
	result := initializers.DB.Create(&user)
	if result.Error != nil {
		// Note: You might want to check if this is a duplicate email error specifically
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "User with this email may already exist",
			"error":   result.Error.Error(),
		})
		return
	}

	err = utils.SendOTPEmail(user.Email, otp)
	if err != nil {
		// Ideally, rollback the user creation here or flag them for OTP resend
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Account created, but failed to send verification email. Please request a new OTP.",
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{ // 201 Created is better for POST requests
		"success": true,
		"message": "User created successfully. Please check your email for the OTP.",
	})
}

func VerifyUser(c *gin.Context) {
	var body struct {
		Email string `json:"email" binding:"required,email"`
		OTP   string `json:"otp" binding:"required"`
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{ // Changed to 400
			"success": false,
			"message": "Invalid request body",
		})
		return
	}

	var user models.User
	// 3. Fixed the GORM query and added error handling
	err := initializers.DB.Where("email = ?", body.Email).First(&user).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "User not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Database error",
		})
		return
	}

	if user.EmailOTP != body.OTP {
		c.JSON(http.StatusUnauthorized, gin.H{ // Changed to 401
			"success": false,
			"message": "The OTP doesn't match",
		})
		return
	}

	if user.EmailOTPExpiresAt == nil || time.Now().After(*user.EmailOTPExpiresAt) {
		c.JSON(http.StatusUnauthorized, gin.H{ // Changed to 401
			"success": false,
			"message": "The OTP code has expired",
		})
		return
	}

	// 4. Update status AND clear the OTP so it can't be reused
	user.IsVerified = true
	user.EmailOTP = ""
	user.EmailOTPExpiresAt = nil
	initializers.DB.Save(&user)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "User verified successfully",
	})
}
