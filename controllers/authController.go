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

func setAuthCookies(c *gin.Context, user models.User) error {
	refreshToken := utils.GenerateToken(user.ID, 60*60*24*30, user.Role)
	accessToken := utils.GenerateToken(user.ID, 60*15, user.Role)

	refreshTokenString, err := refreshToken.SignedString([]byte(os.Getenv("JWT_SECRET")))
	if err != nil {
		return err
	}

	accessTokenString, err := accessToken.SignedString([]byte(os.Getenv("JWT_SECRET")))
	if err != nil {
		return err
	}

	isProduction := os.Getenv("APP_ENV") == "production"
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("RefreshToken", refreshTokenString, 60*60*24*30, "/", "", isProduction, true)
	c.SetCookie("AccessToken", accessTokenString, 60*15, "/", "", isProduction, true)

	return nil
}

func sanitizeUser(user *models.User) {
	user.Password = ""
	user.EmailOTP = ""
	user.EmailOTPExpiresAt = nil
}

func Login(c *gin.Context) {
	var body struct {
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password" binding:"required"`
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
	err := initializers.DB.
		Preload("Memberships").
		Where("email = ?", body.Email).
		First(&user).Error

	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Invalid email or password",
		})
		return
	}

	if !user.IsVerified {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "Please verify your email address before logging in",
		})
		return
	}

	if user.IsSuspended {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "This account has been suspended",
		})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(body.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Invalid email or password",
		})
		return
	}

	if err := setAuthCookies(c, user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to create authentication cookies",
		})
		return
	}

	sanitizeUser(&user)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Logged in successfully",
		"role":    user.Role,
		"user":    user,
	})
}

func SignUp(c *gin.Context) {
	var body struct {
		FirstName   string `json:"firstName" binding:"required"`
		LastName    string `json:"lastName" binding:"required"`
		PhoneNumber string `json:"phoneNumber" binding:"required"`
		Email       string `json:"email" binding:"required,email"`
		Password    string `json:"password" binding:"required,min=6"`
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
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
		Role:              "Owner",
		IsVerified:        false,
	}

	if err := initializers.DB.Create(&user).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "User with this email may already exist",
			"error":   err.Error(),
		})
		return
	}

	if err := utils.SendOTPEmail(user.Email, otp); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Account created, but failed to send verification email. Please request a new OTP.",
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
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
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request body",
			"error":   err.Error(),
		})
		return
	}

	var user models.User
	err := initializers.DB.Where("email = ?", body.Email).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
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

	if user.IsVerified {
		if err := setAuthCookies(c, user); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "Failed to create authentication cookies",
			})
			return
		}

		sanitizeUser(&user)

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "User already verified",
			"user":    user,
		})
		return
	}

	if user.EmailOTP != body.OTP {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "The OTP doesn't match",
		})
		return
	}

	if user.EmailOTPExpiresAt == nil || time.Now().After(*user.EmailOTPExpiresAt) {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "The OTP code has expired",
		})
		return
	}

	user.IsVerified = true
	user.EmailOTP = ""
	user.EmailOTPExpiresAt = nil

	if err := initializers.DB.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to update verification status",
		})
		return
	}

	if err := setAuthCookies(c, user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to create authentication cookies",
		})
		return
	}

	sanitizeUser(&user)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "User verified successfully",
		"user":    user,
	})
}

func Validate(c *gin.Context) {
	userInterface, ok := c.Get("user")
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Unauthenticated user",
		})
		return
	}

	user, ok := userInterface.(models.User)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Invalid user in request context",
		})
		return
	}

	var freshUser models.User
	err := initializers.DB.
		Preload("Memberships").
		Where("id = ?", user.ID).
		First(&freshUser).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "User no longer exists",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Error while validating session",
		})
		return
	}

	sanitizeUser(&freshUser)

	response := gin.H{
		"success": true,
		"role":    freshUser.Role,
		"user":    freshUser,
	}

	// Surface the impersonation grant (set by middleware.RequireAuthentication
	// when the access token carries an "impersonating" claim) so the tenant
	// admin app can show the banner and auto-select the impersonated shop —
	// without needing to know anything about the super-admin app.
	if isImpersonating, _ := c.Get("isImpersonating"); isImpersonating == true {
		if shopIDStr, ok := c.Get("impersonatedShopID"); ok {
			if shopID, err := uuid.Parse(shopIDStr.(string)); err == nil {
				var shop models.Shop
				if err := initializers.DB.Select("id", "name", "slug").First(&shop, "id = ?", shopID).Error; err == nil {
					response["isImpersonating"] = true
					response["impersonatedShop"] = gin.H{
						"id":   shop.ID,
						"name": shop.Name,
						"slug": shop.Slug,
					}
				}
			}
		}
	}

	c.JSON(http.StatusOK, response)
}

func ForgotPassword(c *gin.Context) {
	var body struct {
		Email string `json:"email" binding:"required,email"`
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Email invalide"})
		return
	}

	var user models.User
	if err := initializers.DB.Where("email = ?", body.Email).First(&user).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "Si un compte existe avec cet email, un code a été envoyé."})
		return
	}

	otp := utils.GenerateOTP()
	expiresAt := time.Now().Add(15 * time.Minute)
	user.EmailOTP = otp
	user.EmailOTPExpiresAt = &expiresAt

	if err := initializers.DB.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Erreur lors de la génération du code"})
		return
	}

	if err := utils.SendPasswordResetEmail(user.Email, otp); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Impossible d'envoyer l'email"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Code de réinitialisation envoyé"})
}

func ResetPassword(c *gin.Context) {
	var body struct {
		Email    string `json:"email" binding:"required,email"`
		OTP      string `json:"otp" binding:"required"`
		Password string `json:"password" binding:"required,min=6"`
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Données invalides", "error": err.Error()})
		return
	}

	var user models.User
	if err := initializers.DB.Where("email = ?", body.Email).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Compte introuvable"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Erreur base de données"})
		return
	}

	if user.EmailOTP == "" || user.EmailOTP != body.OTP {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "Code OTP invalide"})
		return
	}

	if user.EmailOTPExpiresAt == nil || time.Now().After(*user.EmailOTPExpiresAt) {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "Code OTP expiré"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), 10)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Erreur lors du traitement du mot de passe"})
		return
	}

	user.Password = string(hash)
	user.EmailOTP = ""
	user.EmailOTPExpiresAt = nil

	if err := initializers.DB.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Erreur lors de la mise à jour du mot de passe"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Mot de passe réinitialisé avec succès"})
}

func Logout(c *gin.Context) {
	isProduction := os.Getenv("APP_ENV") == "production"
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("RefreshToken", "", -1, "/", "", isProduction, true)
	c.SetCookie("AccessToken", "", -1, "/", "", isProduction, true)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Logged out successfully",
	})
}
