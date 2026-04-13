package controllers

import (
	"net/http"
	"os"

	"github.com/chtiwa/lk-parfumo-server/initializers"
	"github.com/chtiwa/lk-parfumo-server/models"
	"github.com/chtiwa/lk-parfumo-server/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func GetUsers(c *gin.Context) {
	var users []models.User

	result := initializers.DB.Find(&users)

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Error while retrieving the users",
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    users,
	})
}

func GetUser(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Error while parsing the id",
		})
		return
	}

	var user models.User

	result := initializers.DB.First(&user, id)

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Error while retrieving the users",
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "The user was retrieved successfully",
		"data":    user,
	})
}

func CreateUser(c *gin.Context) {
	var body struct {
		Username string
		Password string
		Role     string
	}

	err := c.ShouldBindJSON(&body)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Failed to parse the body",
		})
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), 10)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Failed to hash the password",
		})
		return
	}

	if body.Role == "" {
		body.Role = "User"
	}

	user := models.User{Username: body.Username, Password: string(hash), Role: body.Role}
	result := initializers.DB.Create(&user)

	if result.Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Failed to create the user",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "User was created successfuly",
		"data":    user,
	})
}

func UpdateUser(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Error while parsing the id",
		})
		return
	}

	// TODO : add password later for admins to change
	var body struct {
		Username string `json:"username"`
		Role     string `json:"role"`
	}

	err = c.ShouldBindJSON(&body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Error while parsing the body!",
		})
		return
	}

	var user models.User
	initializers.DB.First(&user, id)

	initializers.DB.Model(&user).Updates(&body)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "User was updated successfuly",
		"data":    user,
	})

}

func Login(c *gin.Context) {
	var body struct {
		Username string
		Password string
	}

	err := c.ShouldBindJSON(&body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Failed to parse the body",
		})
		return
	}

	var user models.User
	result := initializers.DB.First(&user, "username = ?", body.Username)
	if result.Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid username or password",
		})
		return
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(body.Password))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid username or password",
		})
		return
	}

	// generate the access and refresh token
	refreshToken := utils.GenerateToken(user.ID, 60*60*24*30, user.Role)
	accessToken := utils.GenerateToken(user.ID, 60*15, user.Role)

	refreshTokenString, err := refreshToken.SignedString([]byte(os.Getenv("JWT_SECRET")))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Failed to create the refresh token",
		})
		return
	}

	accessTokenString, err := accessToken.SignedString([]byte(os.Getenv("JWT_SECRET")))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Failed to create the access token",
		})
		return
	}

	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("RefreshToken", refreshTokenString, 60*60*24*30, "", "", false, true)
	c.SetCookie("AccessToken", accessTokenString, 60*15, "", "", false, true)
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
			"message": "Error while fetching the role",
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"role":    role,
	})
}

func Logout(c *gin.Context) {
	c.SetCookie("RefreshToken", "", 0, "", "", false, true)
	c.SetCookie("AccessToken", "", 0, "", "", false, true)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Logged out successfuly",
	})
}

func DeleteUser(c *gin.Context) {
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
