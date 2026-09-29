package controllers

import (
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const refreshTokenTTLSeconds = 60 * 60 * 24 * 7 // 7d, per CLAUDE.md

// normalizeEmail lowercases and trims an email input so per-email rate
// limits/counters (Redis keys keyed on email) and lookups can't be
// sidestepped by varying case or padding (see important.todo LAUNCH BLOCKER
// 4). Applied to every email field read from a request body before it's
// used for a DB lookup or a rate-limit key.
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func setAuthCookies(c *gin.Context, user models.User) error {
	refreshToken := utils.GenerateToken(user.ID, refreshTokenTTLSeconds, user.Role)
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
	c.SetCookie("RefreshToken", refreshTokenString, refreshTokenTTLSeconds, "/", "", isProduction, true)
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
		RespondError(c, http.StatusBadRequest, "Invalid request body", err)
		return
	}
	body.Email = normalizeEmail(body.Email)

	const maxFailedLogins = 10
	const failedLoginWindow = 15 * time.Minute

	if middleware.TooManyFailedLogins(body.Email, maxFailedLogins, failedLoginWindow) {
		c.JSON(http.StatusTooManyRequests, gin.H{
			"success": false,
			"message": "Too many attempts, please try again later",
		})
		return
	}

	var user models.User
	err := initializers.DB.
		Preload("Memberships").
		Where("email = ?", body.Email).
		First(&user).Error

	if err != nil {
		middleware.RecordFailedLogin(body.Email, failedLoginWindow)
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
			"code":    "EMAIL_NOT_VERIFIED",
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
		middleware.RecordFailedLogin(body.Email, failedLoginWindow)
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

	middleware.ClearFailedLogins(body.Email)
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
		RespondError(c, http.StatusBadRequest, "Invalid request body", err)
		return
	}
	body.Email = normalizeEmail(body.Email)

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
		Role:              "owner",
		IsVerified:        false,
	}

	if err := initializers.DB.Create(&user).Error; err != nil {
		RespondError(c, http.StatusBadRequest, "User with this email may already exist", err)
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

// otpDecision is the outcome of checking a submitted OTP against a user's
// stored OTP, for an already-unverified user. It never grants a session for
// a verified user — that path is handled separately in VerifyUser before
// this is even called.
type otpDecision int

const (
	otpOK otpDecision = iota
	otpMismatch
	otpExpired
	otpAlreadyVerified
)

// verifyOTPDecision is the pure decision for VerifyUser: whether a submitted
// OTP for an unverified user matches and is still valid. A verified user is
// never otpOK here (defense in depth behind VerifyUser's own 409 guard) —
// that was the original login-without-OTP bug.
func verifyOTPDecision(isVerified bool, storedOTP, submittedOTP string, expiresAt *time.Time, now time.Time) otpDecision {
	if isVerified {
		return otpAlreadyVerified
	}
	if storedOTP == "" || storedOTP != submittedOTP {
		return otpMismatch
	}
	if expiresAt == nil || now.After(*expiresAt) {
		return otpExpired
	}
	return otpOK
}

func VerifyUser(c *gin.Context) {
	var body struct {
		Email string `json:"email" binding:"required,email"`
		OTP   string `json:"otp" binding:"required"`
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		RespondError(c, http.StatusBadRequest, "Invalid request body", err)
		return
	}
	body.Email = normalizeEmail(body.Email)

	const maxOTPAttempts = 5
	const otpAttemptWindow = 15 * time.Minute // matches the OTP's own TTL

	if middleware.TooManyOTPAttempts("verify", body.Email, maxOTPAttempts) {
		c.JSON(http.StatusTooManyRequests, gin.H{
			"success": false,
			"message": "Too many attempts, please try again later",
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
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"message": "Account already verified, please log in",
		})
		return
	}

	decision := verifyOTPDecision(user.IsVerified, user.EmailOTP, body.OTP, user.EmailOTPExpiresAt, time.Now())
	switch decision {
	case otpAlreadyVerified:
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"message": "Account already verified, please log in",
		})
		return
	case otpMismatch:
		middleware.RecordOTPAttempt("verify", body.Email, otpAttemptWindow)
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "The OTP doesn't match",
		})
		return
	case otpExpired:
		middleware.RecordOTPAttempt("verify", body.Email, otpAttemptWindow)
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "The OTP code has expired",
		})
		return
	}

	middleware.ClearOTPAttempts("verify", body.Email)

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
		"role":    user.Role,
		"user":    user,
	})
}

// ResendOTP issues a fresh EmailOTP for an existing, unverified user. Always
// responds with the same generic success message regardless of whether the
// account exists or is already verified — no account enumeration (see
// important.todo LAUNCH BLOCKER 14).
func ResendOTP(c *gin.Context) {
	var body struct {
		Email string `json:"email" binding:"required,email"`
	}

	if err := c.ShouldBindJSON(&body); err != nil {
		RespondError(c, http.StatusBadRequest, "Invalid request body", err)
		return
	}
	body.Email = normalizeEmail(body.Email)

	const genericResponse = "If an account exists and is not yet verified, a new code has been sent."

	var user models.User
	err := initializers.DB.Where("email = ?", body.Email).First(&user).Error
	if err != nil || user.IsVerified {
		c.JSON(http.StatusOK, gin.H{"success": true, "message": genericResponse})
		return
	}

	otp := utils.GenerateOTP()
	expiresAt := time.Now().Add(15 * time.Minute)
	user.EmailOTP = otp
	user.EmailOTPExpiresAt = &expiresAt

	if err := initializers.DB.Save(&user).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "message": genericResponse})
		return
	}

	middleware.ClearOTPAttempts("verify", user.Email)

	_ = utils.SendOTPEmail(user.Email, otp)

	c.JSON(http.StatusOK, gin.H{"success": true, "message": genericResponse})
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
	body.Email = normalizeEmail(body.Email)

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

	// A fresh OTP resets the attempt budget from the previous code.
	middleware.ClearOTPAttempts("reset", user.Email)

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
		RespondError(c, http.StatusBadRequest, "Données invalides", err)
		return
	}
	body.Email = normalizeEmail(body.Email)

	const maxOTPAttempts = 5
	const otpAttemptWindow = 15 * time.Minute // matches the OTP's own TTL

	if middleware.TooManyOTPAttempts("reset", body.Email, maxOTPAttempts) {
		c.JSON(http.StatusTooManyRequests, gin.H{"success": false, "message": "Trop de tentatives, réessayez plus tard"})
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
		middleware.RecordOTPAttempt("reset", body.Email, otpAttemptWindow)
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "Code OTP invalide"})
		return
	}

	if user.EmailOTPExpiresAt == nil || time.Now().After(*user.EmailOTPExpiresAt) {
		middleware.RecordOTPAttempt("reset", body.Email, otpAttemptWindow)
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "Code OTP expiré"})
		return
	}

	middleware.ClearOTPAttempts("reset", body.Email)

	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), 10)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Erreur lors du traitement du mot de passe"})
		return
	}

	user.Password = string(hash)
	user.EmailOTP = ""
	user.EmailOTPExpiresAt = nil
	// Proving inbox ownership via OTP is equivalent to email verification —
	// unblocks a signed-up-but-never-verified user who forgot their password
	// instead of leaving them stuck behind both gates (important.todo LAUNCH
	// BLOCKER 14).
	user.IsVerified = true

	if err := initializers.DB.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Erreur lors de la mise à jour du mot de passe"})
		return
	}

	// A leaked/stolen refresh token must stop working once the account owner
	// resets their password — otherwise the reset doesn't actually lock the
	// attacker out.
	utils.RevokeAllSessions(user.ID.String())

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Mot de passe réinitialisé avec succès"})
}

func Logout(c *gin.Context) {
	// Best-effort: denylist the refresh token's jti so a copy of it (already
	// exfiltrated, cached by a proxy, etc) stops working immediately instead
	// of remaining valid for the rest of its 7d life.
	if refreshTokenString, err := c.Cookie("RefreshToken"); err == nil {
		if _, claims, err := utils.ParseJWT(refreshTokenString); err == nil {
			utils.RevokeToken(claims)
		}
	}

	isProduction := os.Getenv("APP_ENV") == "production"
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("RefreshToken", "", -1, "/", "", isProduction, true)
	c.SetCookie("AccessToken", "", -1, "/", "", isProduction, true)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Logged out successfully",
	})
}
