package controllers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/utils"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// Available delivery companies (global, admin-managed)
// ---------------------------------------------------------------------------

type CreateAvailableDeliveryCompanyInput struct {
	Name string `form:"name" binding:"required"`
	URL  string `form:"url" binding:"required"`
}

type UpdateAvailableDeliveryCompanyInput struct {
	Name     *string `form:"name"`
	URL      *string `form:"url"`
	IsActive *bool   `form:"isActive"`
}

func GetAvailableDeliveryCompanies(c *gin.Context) {
	var companies []models.AvailableDeliveryCompany
	if err := initializers.DB.Preload("Image").Where("is_active = ?", true).Find(&companies).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to fetch available delivery companies",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": companies})
}

// ListAllAvailableDeliveryCompanies is the super-admin variant — returns both
// active and inactive entries so operators can re-enable a disabled carrier.
func ListAllAvailableDeliveryCompanies(c *gin.Context) {
	var companies []models.AvailableDeliveryCompany
	if err := initializers.DB.Preload("Image").Find(&companies).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to fetch available delivery companies",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": companies})
}

func CreateAvailableDeliveryCompany(c *gin.Context) {
	var body CreateAvailableDeliveryCompanyInput
	if err := c.ShouldBindWith(&body, binding.FormMultipart); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Validation failed", "error": err.Error()})
		return
	}

	uploadedImageURL, uploadErr := uploadAvailableDeliveryCompanyImage(c)
	if uploadErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": uploadErr.Error()})
		return
	}

	var company models.AvailableDeliveryCompany

	err := initializers.DB.Transaction(func(tx *gorm.DB) error {
		company = models.AvailableDeliveryCompany{
			Name: strings.TrimSpace(body.Name),
			URL:  strings.TrimSpace(body.URL),
		}

		if err := tx.Omit("Image").Create(&company).Error; err != nil {
			return err
		}

		if uploadedImageURL != "" {
			img := models.AvailableDeliveryCompanyImage{
				AvailableDeliveryCompanyID: company.ID,
				URL:                        uploadedImageURL,
			}
			if err := tx.Create(&img).Error; err != nil {
				return err
			}
			company.Image = &img
		}

		return nil
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to create available delivery company",
			"error":   err.Error(),
		})
		return
	}

	utils.LogAudit(c, "available_delivery_company.create", "AvailableDeliveryCompany", &company.ID, gin.H{
		"name":     company.Name,
		"url":      company.URL,
		"hasImage": company.Image != nil,
	})

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "Available delivery company created successfully",
		"data":    company,
	})
}

func UpdateAvailableDeliveryCompany(c *gin.Context) {
	companyID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid delivery company ID"})
		return
	}

	var body UpdateAvailableDeliveryCompanyInput
	if err := c.ShouldBindWith(&body, binding.FormMultipart); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Validation failed", "error": err.Error()})
		return
	}

	var company models.AvailableDeliveryCompany
	if err := initializers.DB.Preload("Image").First(&company, "id = ?", companyID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Delivery company not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Database error", "error": err.Error()})
		return
	}

	uploadedImageURL, uploadErr := uploadAvailableDeliveryCompanyImage(c)
	if uploadErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": uploadErr.Error()})
		return
	}

	updates := map[string]any{}
	if body.Name != nil {
		updates["name"] = strings.TrimSpace(*body.Name)
	}
	if body.URL != nil {
		updates["url"] = strings.TrimSpace(*body.URL)
	}
	if body.IsActive != nil {
		updates["is_active"] = *body.IsActive
	}

	err = initializers.DB.Transaction(func(tx *gorm.DB) error {
		if len(updates) > 0 {
			if err := tx.Model(&company).Updates(updates).Error; err != nil {
				return err
			}
		}

		if uploadedImageURL != "" {
			if company.Image != nil {
				if err := tx.Model(company.Image).Update("url", uploadedImageURL).Error; err != nil {
					return err
				}
			} else {
				img := models.AvailableDeliveryCompanyImage{
					AvailableDeliveryCompanyID: company.ID,
					URL:                        uploadedImageURL,
				}
				if err := tx.Create(&img).Error; err != nil {
					return err
				}
			}
		}

		return nil
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to update delivery company",
			"error":   err.Error(),
		})
		return
	}

	initializers.DB.Preload("Image").First(&company, "id = ?", company.ID)

	utils.LogAudit(c, "available_delivery_company.update", "AvailableDeliveryCompany", &company.ID, gin.H{
		"updates":      updates,
		"imageChanged": uploadedImageURL != "",
	})

	if body.IsActive != nil {
		utils.LogAudit(c, "available_delivery_company.toggle", "AvailableDeliveryCompany", &company.ID, gin.H{
			"name":     company.Name,
			"isActive": *body.IsActive,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Available delivery company updated successfully",
		"data":    company,
	})
}

func DeleteAvailableDeliveryCompany(c *gin.Context) {
	companyID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid delivery company ID"})
		return
	}

	var company models.AvailableDeliveryCompany
	if err := initializers.DB.First(&company, "id = ?", companyID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Delivery company not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Database error", "error": err.Error()})
		return
	}

	if err := initializers.DB.Delete(&company).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to delete delivery company",
			"error":   err.Error(),
		})
		return
	}

	utils.LogAudit(c, "available_delivery_company.delete", "AvailableDeliveryCompany", &company.ID, gin.H{
		"name": company.Name,
		"url":  company.URL,
	})

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Delivery company deleted successfully"})
}

// ---------------------------------------------------------------------------
// Shop delivery company integrations (per-shop credentials)
// ---------------------------------------------------------------------------

type ConnectDeliveryCompanyInput struct {
	AvailableDeliveryCompanyID string `json:"availableDeliveryCompanyId" binding:"required"`
	Token                      string `json:"token"`
	MerchantID                 string `json:"merchantId"`
}

type UpdateDeliveryCompanyCredentialsInput struct {
	Token      *string `json:"token"`
	MerchantID *string `json:"merchantId"`
}

func GetShopDeliveryCompanies(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	var integrations []models.DeliveryCompany
	if err := initializers.DB.
		Preload("AvailableDeliveryCompany").
		Preload("AvailableDeliveryCompany.Image").
		Where("shop_id = ?", shopID).
		Find(&integrations).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to fetch shop delivery companies",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": integrations})
}

func ConnectDeliveryCompany(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	var body ConnectDeliveryCompanyInput
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Validation failed", "error": err.Error()})
		return
	}
	body.Token = strings.TrimSpace(body.Token)
	body.MerchantID = strings.TrimSpace(body.MerchantID)

	availableID, err := uuid.Parse(body.AvailableDeliveryCompanyID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid available delivery company ID"})
		return
	}

	var available models.AvailableDeliveryCompany
	if err := initializers.DB.First(&available, "id = ?", availableID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Available delivery company not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Database error", "error": err.Error()})
		return
	}

	// Validate credentials against the carrier's API before saving.
	if strings.EqualFold(strings.TrimSpace(available.Name), "osen express") {
		valid, errMsg := validateOsenToken(body.Token)
		if !valid {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": fmt.Sprintf("Token Osen Express invalide: %s", errMsg),
			})
			return
		}
	}
	if strings.EqualFold(strings.TrimSpace(available.Name), "zr express") {
		valid, errMsg := validateZrCredentials(body.Token, body.MerchantID)
		if !valid {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": fmt.Sprintf("Identifiants ZR Express invalides: %s", errMsg),
			})
			return
		}
	}

	var existing models.DeliveryCompany
	err = initializers.DB.
		Where("shop_id = ? AND available_delivery_company_id = ?", shopID, availableID).
		First(&existing).Error
	if err == nil {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"message": fmt.Sprintf("%s is already connected to this shop", available.Name),
		})
		return
	}
	if err != gorm.ErrRecordNotFound {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Database error", "error": err.Error()})
		return
	}

	integration := models.DeliveryCompany{
		ShopID:                     shopID,
		AvailableDeliveryCompanyID: availableID,
		Token:                      body.Token,
		MerchantID:                 body.MerchantID,
	}

	if err := initializers.DB.Create(&integration).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to connect delivery company",
			"error":   err.Error(),
		})
		return
	}

	initializers.DB.
		Preload("AvailableDeliveryCompany").
		Preload("AvailableDeliveryCompany.Image").
		First(&integration, "id = ?", integration.ID)

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "Delivery company connected successfully",
		"data":    integration,
	})
}

func UpdateDeliveryCompanyCredentials(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	integrationID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid integration ID"})
		return
	}

	var body UpdateDeliveryCompanyCredentialsInput
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Validation failed", "error": err.Error()})
		return
	}

	var integration models.DeliveryCompany
	if err := initializers.DB.
		Where("id = ? AND shop_id = ?", integrationID, shopID).
		First(&integration).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Integration not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Database error", "error": err.Error()})
		return
	}

	updates := map[string]any{}
	if body.Token != nil {
		updates["token"] = strings.TrimSpace(*body.Token)
	}
	if body.MerchantID != nil {
		updates["merchant_id"] = strings.TrimSpace(*body.MerchantID)
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "No fields provided for update"})
		return
	}

	if err := initializers.DB.Model(&integration).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to update credentials",
			"error":   err.Error(),
		})
		return
	}

	initializers.DB.
		Preload("AvailableDeliveryCompany").
		Preload("AvailableDeliveryCompany.Image").
		First(&integration, "id = ?", integration.ID)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Credentials updated successfully",
		"data":    integration,
	})
}

func DisconnectDeliveryCompany(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	integrationID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid integration ID"})
		return
	}

	var integration models.DeliveryCompany
	if err := initializers.DB.
		Where("id = ? AND shop_id = ?", integrationID, shopID).
		First(&integration).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Integration not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Database error", "error": err.Error()})
		return
	}

	if err := initializers.DB.Delete(&integration).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to disconnect delivery company",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Delivery company disconnected successfully"})
}

// ---------------------------------------------------------------------------
// Image upload helper
// ---------------------------------------------------------------------------

func uploadAvailableDeliveryCompanyImage(c *gin.Context) (string, error) {
	file, err := c.FormFile("image")
	if err != nil || file == nil {
		return "", nil
	}

	src, err := file.Open()
	if err != nil {
		return "", fmt.Errorf("failed to open uploaded image")
	}
	defer src.Close()

	buf := make([]byte, 512)
	n, err := src.Read(buf)
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("failed to read uploaded image")
	}

	contentType := http.DetectContentType(buf[:n])
	if !strings.HasPrefix(contentType, "image/") {
		return "", fmt.Errorf("only image files are allowed")
	}

	seeker, ok := src.(io.Seeker)
	if !ok {
		return "", fmt.Errorf("failed to process image stream")
	}
	if _, err := seeker.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("failed to reset image stream")
	}

	cleanFileName := filepath.Base(file.Filename)
	key := fmt.Sprintf("uploads/delivery-companies/%d_%s", time.Now().UnixNano(), cleanFileName)

	bucketName := os.Getenv("B2_BUCKET_NAME")
	b2Region := os.Getenv("B2_REGION")
	b2PublicBaseURL := strings.TrimSpace(os.Getenv("B2_PUBLIC_BASE_URL"))

	_, putErr := initializers.S3Client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket:        aws.String(bucketName),
		Key:           aws.String(key),
		Body:          src,
		ContentType:   aws.String(contentType),
		ContentLength: aws.Int64(file.Size),
	})
	if putErr != nil {
		return "", fmt.Errorf("failed to upload image: %w", putErr)
	}

	if b2PublicBaseURL != "" {
		return fmt.Sprintf("%s/%s", strings.TrimRight(b2PublicBaseURL, "/"), key), nil
	}
	return fmt.Sprintf("https://%s.s3.%s.backblazeb2.com/%s", bucketName, b2Region, key), nil
}
