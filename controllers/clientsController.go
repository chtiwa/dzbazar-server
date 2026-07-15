package controllers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

type CreateClientInput struct {
	FullName      string `json:"fullName"`
	PhoneNumber   string `json:"phoneNumber" binding:"required"`
	PhoneNumber2  string `json:"phoneNumber2"`
	State         string `json:"state"`
	StateCode     string `json:"stateCode"`
	City          string `json:"city"`
	StopdeskPoint string `json:"stopdeskPoint"`
}

type UpdateClientInput struct {
	FullName      *string `json:"fullName"`
	PhoneNumber   *string `json:"phoneNumber"`
	PhoneNumber2  *string `json:"phoneNumber2"`
	State         *string `json:"state"`
	StateCode     *string `json:"stateCode"`
	City          *string `json:"city"`
	StopdeskPoint *string `json:"stopdeskPoint"`
}

// Update your normalization helper
func normalizeClientInput(fullName, phoneNumber, phoneNumber2, state, stateCode, city, stopdeskPoint string) (string, string, string, string, string, string, string) {
	return strings.TrimSpace(fullName),
		strings.TrimSpace(phoneNumber),
		strings.TrimSpace(phoneNumber2),
		strings.TrimSpace(state),
		strings.TrimSpace(stateCode),
		strings.TrimSpace(city),
		strings.TrimSpace(stopdeskPoint)
}

func GetClientsByShopID(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid shop ID",
		})
		return
	}

	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}

	limit, err := strconv.Atoi(c.DefaultQuery("perPage", "20"))
	if err != nil || limit < 1 {
		limit = 20
	}

	offset := (page - 1) * limit

	var clients []models.Client
	var total int64

	if err := initializers.DB.Model(&models.Client{}).
		Where("shop_id = ?", shopID).
		Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to count clients",
			"error":   err.Error(),
		})
		return
	}

	if err := initializers.DB.
		Where("shop_id = ?", shopID).
		Preload("Orders", func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at DESC")
		}).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&clients).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to retrieve clients",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    clients,
		"page":    page,
		"limit":   limit,
		"count":   len(clients),
		"total":   total,
	})
}

func GetClientsBySearch(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid shop ID",
		})
		return
	}

	search := strings.TrimSpace(c.Query("search"))
	if search == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Search query is required",
		})
		return
	}

	like := "%" + strings.ToLower(search) + "%"

	var clients []models.Client
	if err := initializers.DB.
		Where("shop_id = ?", shopID).
		Where(`
			LOWER(full_name) LIKE ?
			OR LOWER(phone_number) LIKE ?
			OR LOWER(phone_number2) LIKE ?
		`, like, like, like).
		Preload("Orders", func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at DESC")
		}).
		Order("created_at DESC").
		Find(&clients).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to search clients",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"count":   len(clients),
		"data":    clients,
	})
}

func IndexClientByShopID(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid shop ID",
		})
		return
	}

	clientID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid client ID",
		})
		return
	}

	var client models.Client
	err = initializers.DB.
		Where("id = ? AND shop_id = ?", clientID, shopID).
		Preload("Orders", func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at DESC")
		}).
		First(&client).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "Client not found",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to retrieve client",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    client,
	})
}

func CreateClientByShopID(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid shop ID",
		})
		return
	}

	var body CreateClientInput
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request body",
			"error":   err.Error(),
		})
		return
	}

	fullName, phoneNumber, phoneNumber2, state, stateCode, city, stopdeskPoint := normalizeClientInput(
		body.FullName,
		body.PhoneNumber,
		body.PhoneNumber2,
		body.State,
		body.StateCode,
		body.City,
		body.StopdeskPoint,
	)

	if phoneNumber == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Phone number is required",
		})
		return
	}

	client := models.Client{
		ShopID:        shopID,
		FullName:      fullName,
		PhoneNumber:   phoneNumber,
		PhoneNumber2:  phoneNumber2,
		State:         state,
		StateCode:     stateCode,
		City:          city,
		StopdeskPoint: stopdeskPoint,
	}

	if err := initializers.DB.Create(&client).Error; err != nil {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"message": "Failed to create client. Phone number may already exist for this shop.",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "Client created successfully",
		"data":    client,
	})
}

func UpdateClientByShopID(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid shop ID",
		})
		return
	}

	clientID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid client ID",
		})
		return
	}

	var input UpdateClientInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request body",
			"error":   err.Error(),
		})
		return
	}

	var client models.Client
	err = initializers.DB.
		Where("id = ? AND shop_id = ?", clientID, shopID).
		First(&client).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "Client not found",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to load client",
			"error":   err.Error(),
		})
		return
	}

	updateData := map[string]interface{}{}

	if input.FullName != nil {
		updateData["full_name"] = strings.TrimSpace(*input.FullName)
	}
	if input.PhoneNumber != nil {
		cleanPhone := strings.TrimSpace(*input.PhoneNumber)
		if cleanPhone == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "Phone number cannot be empty",
			})
			return
		}
		updateData["phone_number"] = cleanPhone
	}
	if input.PhoneNumber2 != nil {
		updateData["phone_number2"] = strings.TrimSpace(*input.PhoneNumber2)
	}
	if input.State != nil {
		updateData["state"] = strings.TrimSpace(*input.State)
	}
	if input.StateCode != nil {
		updateData["state_code"] = strings.TrimSpace(*input.StateCode)
	}
	if input.City != nil {
		updateData["city"] = strings.TrimSpace(*input.City)
	}

	if input.StopdeskPoint != nil {
		updateData["stopdesk_point"] = strings.TrimSpace(*input.StopdeskPoint)
	}

	if len(updateData) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "No changes provided",
			"data":    client,
		})
		return
	}

	if err := initializers.DB.Model(&client).Updates(updateData).Error; err != nil {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"message": "Failed to update client. Phone number may already exist for this shop.",
			"error":   err.Error(),
		})
		return
	}

	if err := initializers.DB.
		Where("id = ? AND shop_id = ?", clientID, shopID).
		Preload("Orders", func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at DESC")
		}).
		First(&client).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Client updated but failed to reload record",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Client updated successfully",
		"data":    client,
	})
}

func DeleteClientByShopID(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid shop ID",
		})
		return
	}

	clientID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid client ID",
		})
		return
	}

	var client models.Client
	err = initializers.DB.
		Where("id = ? AND shop_id = ?", clientID, shopID).
		First(&client).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"success": false,
				"message": "Client not found",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to load client",
			"error":   err.Error(),
		})
		return
	}

	if err := initializers.DB.Delete(&client).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to delete client",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Client deleted successfully",
	})
}

func UploadExcelClients(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid shop ID",
		})
		return
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Excel file is required",
			"error":   err.Error(),
		})
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to open uploaded file",
			"error":   err.Error(),
		})
		return
	}
	defer file.Close()

	xl, err := excelize.OpenReader(file)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Failed to parse Excel file",
			"error":   err.Error(),
		})
		return
	}
	defer func() {
		_ = xl.Close()
	}()

	sheets := xl.GetSheetList()
	if len(sheets) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Excel file contains no sheets",
		})
		return
	}

	rows, err := xl.GetRows(sheets[0])
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Failed to read Excel rows",
			"error":   err.Error(),
		})
		return
	}

	if len(rows) < 2 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Excel file contains no client data",
		})
		return
	}

	type excelRow struct {
		FullName     string
		PhoneNumber  string
		PhoneNumber2 string
		State        string
		StateCode    string
		City         string
	}

	var clientsToInsert []models.Client
	var skipped []string

	for i, row := range rows[1:] {
		getCell := func(index int) string {
			if index >= len(row) {
				return ""
			}
			return strings.TrimSpace(row[index])
		}

		item := excelRow{
			FullName:     getCell(0),
			PhoneNumber:  getCell(1),
			PhoneNumber2: getCell(2),
			State:        getCell(3),
			StateCode:    getCell(4),
			City:         getCell(5),
		}

		if item.PhoneNumber == "" {
			skipped = append(skipped, fmt.Sprintf("Row %d skipped: phone number is required", i+2))
			continue
		}

		clientsToInsert = append(clientsToInsert, models.Client{
			ShopID:       shopID,
			FullName:     item.FullName,
			PhoneNumber:  item.PhoneNumber,
			PhoneNumber2: item.PhoneNumber2,
			State:        item.State,
			StateCode:    item.StateCode,
			City:         item.City,
		})
	}

	if len(clientsToInsert) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "No valid clients found in Excel file",
			"skipped": skipped,
		})
		return
	}

	result := initializers.DB.
		Session(&gorm.Session{CreateBatchSize: 100}).
		Clauses().
		Create(&clientsToInsert)

	if result.Error != nil {
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"message": "Failed to import clients. Some phone numbers may already exist for this shop.",
			"error":   result.Error.Error(),
			"skipped": skipped,
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "Clients imported successfully",
		"count":   len(clientsToInsert),
		"data":    clientsToInsert,
		"skipped": skipped,
	})
}
