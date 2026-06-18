package superadmin

import (
	"net/http"
	"strings"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/utils"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func ListSettings(c *gin.Context) {
	var settings []models.GlobalSetting
	if err := initializers.DB.Order("key ASC").Find(&settings).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to fetch settings", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": settings})
}

type UpsertSettingInput struct {
	Value       string `json:"value"`
	ValueType   string `json:"valueType"`
	Description string `json:"description"`
}

// UpsertSetting creates or updates a single key. Settings are intentionally
// managed one key at a time (not bulk) so every change is individually audited.
func UpsertSetting(c *gin.Context) {
	key := strings.TrimSpace(c.Param("key"))
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Setting key is required"})
		return
	}

	var body UpsertSettingInput
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Validation failed", "error": err.Error()})
		return
	}
	if body.ValueType == "" {
		body.ValueType = "string"
	}

	var setting models.GlobalSetting
	err := initializers.DB.Where("key = ?", key).First(&setting).Error

	if err != nil && err != gorm.ErrRecordNotFound {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Database error", "error": err.Error()})
		return
	}

	if err == gorm.ErrRecordNotFound {
		setting = models.GlobalSetting{Key: key, Value: body.Value, ValueType: body.ValueType, Description: body.Description}
		if err := initializers.DB.Create(&setting).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to create setting", "error": err.Error()})
			return
		}
	} else {
		if err := initializers.DB.Model(&setting).Updates(map[string]any{
			"value":       body.Value,
			"value_type":  body.ValueType,
			"description": body.Description,
		}).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to update setting", "error": err.Error()})
			return
		}
		setting.Value = body.Value
		setting.ValueType = body.ValueType
		setting.Description = body.Description
	}

	utils.LogAudit(c, "setting.update", "GlobalSetting", &setting.ID, gin.H{"key": key, "value": body.Value})
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Setting saved", "data": setting})
}
