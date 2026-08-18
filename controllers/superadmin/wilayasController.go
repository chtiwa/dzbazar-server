package superadmin

import (
	"net/http"
	"strconv"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/services"
	"github.com/chtiwa/dzbazar-server/utils"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ListWilayas returns all 58 wilayas ordered by id. No pagination — it's a
// small fixed set, same as feature flags.
func ListWilayas(c *gin.Context) {
	var wilayas []models.Wilaya
	if err := initializers.DB.Order("id ASC").Find(&wilayas).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to fetch wilayas", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": wilayas})
}

type UpdateWilayaInput struct {
	IsActive     *bool    `json:"isActive"`
	HasStopdesk  *bool    `json:"hasStopdesk"`
	StopdeskRate *float64 `json:"stopdeskRate"`
	HasDoorstep  *bool    `json:"hasDoorstep"`
	DoorstepRate *float64 `json:"doorstepRate"`
}

// UpdateWilaya edits an existing wilaya's active flag and delivery rates.
// Wilayas are a fixed set of 58 (Algeria isn't getting new provinces) — this
// is edit-only, no create/delete, same as feature flags' toggle path.
func UpdateWilaya(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid wilaya ID"})
		return
	}

	var body UpdateWilayaInput
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondError(c, http.StatusBadRequest, "Validation failed", err)
		return
	}

	var wilaya models.Wilaya
	if err := initializers.DB.First(&wilaya, "id = ?", id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Wilaya not found"})
			return
		}
		RespondError(c, http.StatusInternalServerError, "Database error", err)
		return
	}

	updates := map[string]any{}
	if body.IsActive != nil {
		updates["is_active"] = *body.IsActive
	}
	if body.HasStopdesk != nil {
		updates["has_stopdesk"] = *body.HasStopdesk
	}
	if body.StopdeskRate != nil {
		updates["stopdesk_rate"] = *body.StopdeskRate
	}
	if body.HasDoorstep != nil {
		updates["has_doorstep"] = *body.HasDoorstep
	}
	if body.DoorstepRate != nil {
		updates["doorstep_rate"] = *body.DoorstepRate
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "No fields provided for update"})
		return
	}

	if err := initializers.DB.Model(&wilaya).Updates(updates).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to update wilaya", err)
		return
	}

	services.InvalidateWilayaCache()
	utils.LogAudit(c, "wilaya.update", "Wilaya", nil, gin.H{"wilayaId": wilaya.ID, "name": wilaya.Name, "updates": updates})

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Wilaya updated", "data": wilaya})
}
