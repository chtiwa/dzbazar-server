package controllers

import (
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/chtiwa/dzbazar-server/dto"
	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/services"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type experimentBody struct {
	ProductID         string   `json:"productId"`
	Name              string   `json:"name"`
	TargetConversions int      `json:"targetConversions"`
	LandingPageIDs    []string `json:"landingPageIds"`
}

// buildExperimentResponse reuses the exact same views/orders queries the
// standalone landing-page PagePerf panel uses (viewsByEntityIDs,
// countOrdersByLandingPageIDs, conversionRate in productsController.go) so
// per-set standings never drift from a landing page's own numbers.
func buildExperimentResponse(experiment models.LandingPageExperiment) (dto.ExperimentResponse, error) {
	setIDs := make([]uuid.UUID, len(experiment.Sets))
	for i, s := range experiment.Sets {
		setIDs[i] = s.ID
	}

	views, err := viewsByEntityIDs("landing_page", setIDs)
	if err != nil {
		return dto.ExperimentResponse{}, err
	}
	convs, err := countOrdersByLandingPageIDs(setIDs)
	if err != nil {
		return dto.ExperimentResponse{}, err
	}

	tallies := make([]services.SetTally, len(experiment.Sets))
	standings := make([]dto.ExperimentSetStandingResponse, len(experiment.Sets))
	for i, s := range experiment.Sets {
		v := views[s.ID]
		c := convs[s.ID]
		tallies[i] = services.SetTally{LandingPageID: s.ID, Position: s.ExperimentPosition, Conversions: int(c), Views: v}
		standings[i] = dto.ExperimentSetStandingResponse{
			LandingPageID:  s.ID.String(),
			Position:       s.ExperimentPosition,
			Title:          s.Title,
			Views:          v,
			Conversions:    c,
			ConversionRate: conversionRate(c, v),
			Active:         s.Active,
			IsWinner:       experiment.Status == models.ExperimentStatusDecided && experiment.WinnerLandingPageID != nil && *experiment.WinnerLandingPageID == s.ID,
		}
	}
	sort.Slice(standings, func(i, j int) bool { return standings[i].Position < standings[j].Position })

	resp := dto.ExperimentResponse{
		ID:                experiment.ID.String(),
		Name:              experiment.Name,
		ProductID:         experiment.ProductID.String(),
		TargetConversions: experiment.TargetConversions,
		Status:            experiment.Status,
		Standings:         standings,
		CreatedAt:         experiment.CreatedAt,
	}
	if experiment.WinnerLandingPageID != nil {
		w := experiment.WinnerLandingPageID.String()
		resp.WinnerLandingPageID = &w
	}
	// Leader/p-value are only meaningful while still running — once decided,
	// WinnerLandingPageID + each set's isWinner already say everything.
	if experiment.Status == models.ExperimentStatusRunning && len(tallies) > 0 {
		eval := services.EvaluateExperiment(tallies, experiment.TargetConversions)
		leader := eval.LeaderID.String()
		resp.LeadingLandingPageID = &leader
		p := eval.PValue
		resp.PValue = &p
		resp.IsSignificant = eval.IsSignificant
	}

	return resp, nil
}

func loadExperimentWithSets(db *gorm.DB, shopID, experimentID uuid.UUID, experiment *models.LandingPageExperiment) error {
	return db.
		Where("id = ? AND shop_id = ?", experimentID, shopID).
		Preload("Sets", func(db *gorm.DB) *gorm.DB {
			return db.Order("experiment_position ASC")
		}).
		First(experiment).Error
}

func CreateExperimentByShop(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID", "error": err.Error()})
		return
	}

	var body experimentBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid request body", "error": err.Error()})
		return
	}

	name := strings.TrimSpace(body.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Name is required"})
		return
	}

	if len(body.LandingPageIDs) < 2 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "An experiment needs at least 2 landing pages"})
		return
	}

	productID, err := uuid.Parse(body.ProductID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid product ID", "error": err.Error()})
		return
	}

	var product models.Product
	if err := initializers.DB.Where("id = ? AND shop_id = ?", productID, shopID).First(&product).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Product not found", "error": err.Error()})
		return
	}

	landingPageIDs := make([]uuid.UUID, len(body.LandingPageIDs))
	for i, idStr := range body.LandingPageIDs {
		id, parseErr := uuid.Parse(idStr)
		if parseErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid landing page ID", "error": parseErr.Error()})
			return
		}
		landingPageIDs[i] = id
	}

	var landingPages []models.LandingPage
	if err := initializers.DB.
		Where("id IN ? AND shop_id = ? AND product_id = ? AND experiment_id IS NULL", landingPageIDs, shopID, productID).
		Find(&landingPages).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to load landing pages", "error": err.Error()})
		return
	}
	if len(landingPages) != len(landingPageIDs) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "One or more landing pages are invalid, belong to a different product, or are already in a test"})
		return
	}

	targetConversions := body.TargetConversions
	if targetConversions <= 0 {
		targetConversions = 100
	}

	experiment := models.LandingPageExperiment{
		ShopID:            shopID,
		ProductID:         productID,
		Name:              name,
		TargetConversions: targetConversions,
		Status:            models.ExperimentStatusRunning,
	}

	err = initializers.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&experiment).Error; err != nil {
			return err
		}
		for i, id := range landingPageIDs {
			if err := tx.Model(&models.LandingPage{}).Where("id = ?", id).
				Updates(map[string]interface{}{"experiment_id": experiment.ID, "experiment_position": i}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to create experiment", "error": err.Error()})
		return
	}

	var created models.LandingPageExperiment
	if err := loadExperimentWithSets(initializers.DB, shopID, experiment.ID, &created); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to reload experiment", "error": err.Error()})
		return
	}

	response, err := buildExperimentResponse(created)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to compute experiment standings", "error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"success": true, "message": "Experiment created successfully", "data": response})
}

func GetExperimentsByShop(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID", "error": err.Error()})
		return
	}

	var experiments []models.LandingPageExperiment
	if err := initializers.DB.
		Where("shop_id = ?", shopID).
		Preload("Sets", func(db *gorm.DB) *gorm.DB {
			return db.Order("experiment_position ASC")
		}).
		Order("created_at DESC").
		Find(&experiments).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to retrieve experiments", "error": err.Error()})
		return
	}

	responses := make([]dto.ExperimentResponse, len(experiments))
	for i, e := range experiments {
		resp, err := buildExperimentResponse(e)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to compute experiment standings", "error": err.Error()})
			return
		}
		responses[i] = resp
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Experiments retrieved successfully", "data": responses})
}

func GetExperimentByShop(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID", "error": err.Error()})
		return
	}
	experimentID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid experiment ID", "error": err.Error()})
		return
	}

	var experiment models.LandingPageExperiment
	if err := loadExperimentWithSets(initializers.DB, shopID, experimentID, &experiment); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Experiment not found", "error": err.Error()})
		return
	}

	response, err := buildExperimentResponse(experiment)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to compute experiment standings", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Experiment retrieved successfully", "data": response})
}

type addExperimentSetBody struct {
	LandingPageID string `json:"landingPageId"`
}

// AddLandingPageToExperimentByShop appends one more standalone landing page
// (same product, not already in a test) as a new set on a running
// experiment. Round-robin picks it up automatically next assignment —
// AssignExperimentVariant re-reads active sets from DB on every call, no
// cursor/position rebalancing needed.
func AddLandingPageToExperimentByShop(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID", "error": err.Error()})
		return
	}
	experimentID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid experiment ID", "error": err.Error()})
		return
	}

	var experiment models.LandingPageExperiment
	if err := initializers.DB.Where("id = ? AND shop_id = ?", experimentID, shopID).First(&experiment).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Experiment not found", "error": err.Error()})
		return
	}
	if experiment.Status != models.ExperimentStatusRunning {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Landing pages can only be added to a running test"})
		return
	}

	var body addExperimentSetBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid request body", "error": err.Error()})
		return
	}
	landingPageID, err := uuid.Parse(body.LandingPageID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid landing page ID", "error": err.Error()})
		return
	}

	err = initializers.DB.Transaction(func(tx *gorm.DB) error {
		var setCount int64
		if err := tx.Model(&models.LandingPage{}).Where("experiment_id = ?", experimentID).Count(&setCount).Error; err != nil {
			return err
		}

		res := tx.Model(&models.LandingPage{}).
			Where("id = ? AND shop_id = ? AND product_id = ? AND experiment_id IS NULL", landingPageID, shopID, experiment.ProductID).
			Updates(map[string]interface{}{"experiment_id": experimentID, "experiment_position": setCount})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Landing page is invalid, belongs to a different product, or is already in a test"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to add landing page to the test", "error": err.Error()})
		return
	}

	var updated models.LandingPageExperiment
	if err := loadExperimentWithSets(initializers.DB, shopID, experimentID, &updated); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to reload experiment", "error": err.Error()})
		return
	}

	response, err := buildExperimentResponse(updated)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to compute experiment standings", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Landing page added to the test successfully", "data": response})
}

type updateExperimentBody struct {
	Name              *string `json:"name"`
	TargetConversions *int    `json:"targetConversions"`
	Status            *string `json:"status"` // "stopped", or "running" to reactivate a stopped test
}

func UpdateExperimentByShop(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID", "error": err.Error()})
		return
	}
	experimentID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid experiment ID", "error": err.Error()})
		return
	}

	var experiment models.LandingPageExperiment
	if err := initializers.DB.Where("id = ? AND shop_id = ?", experimentID, shopID).First(&experiment).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Experiment not found", "error": err.Error()})
		return
	}

	var body updateExperimentBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid request body", "error": err.Error()})
		return
	}

	updates := map[string]interface{}{}
	if body.Name != nil {
		if name := strings.TrimSpace(*body.Name); name != "" {
			updates["name"] = name
		}
	}
	if body.TargetConversions != nil && *body.TargetConversions > 0 {
		updates["target_conversions"] = *body.TargetConversions
	}
	if body.Status != nil {
		switch *body.Status {
		case models.ExperimentStatusStopped:
			updates["status"] = models.ExperimentStatusStopped
		case models.ExperimentStatusRunning:
			if experiment.Status != models.ExperimentStatusStopped {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Only a stopped test can be reactivated"})
				return
			}
			updates["status"] = models.ExperimentStatusRunning
		default:
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Status can only be set to 'stopped' or 'running' here"})
			return
		}
	}

	if len(updates) > 0 {
		if err := initializers.DB.Model(&experiment).Updates(updates).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to update experiment", "error": err.Error()})
			return
		}
	}

	var updated models.LandingPageExperiment
	if err := loadExperimentWithSets(initializers.DB, shopID, experimentID, &updated); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to reload experiment", "error": err.Error()})
		return
	}

	response, err := buildExperimentResponse(updated)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to compute experiment standings", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Experiment updated successfully", "data": response})
}

func DeleteExperimentByShop(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID", "error": err.Error()})
		return
	}
	experimentID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid experiment ID", "error": err.Error()})
		return
	}

	var experiment models.LandingPageExperiment
	if err := initializers.DB.Where("id = ? AND shop_id = ?", experimentID, shopID).First(&experiment).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Experiment not found", "error": err.Error()})
		return
	}

	err = initializers.DB.Transaction(func(tx *gorm.DB) error {
		// Soft-deleting the experiment row doesn't fire the FK's ON DELETE SET
		// NULL (that only runs on a real DELETE), so release the sets by hand —
		// they go back to being ordinary standalone landing pages.
		if err := tx.Model(&models.LandingPage{}).Where("experiment_id = ?", experimentID).
			Updates(map[string]interface{}{"experiment_id": nil, "experiment_position": 0}).Error; err != nil {
			return err
		}
		return tx.Delete(&experiment).Error
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to delete experiment", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Experiment deleted successfully"})
}

type assignExperimentBody struct {
	VisitorID string `json:"visitorId"`
}

func AssignExperimentVariantPublic(c *gin.Context) {
	experimentID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid experiment ID", "error": err.Error()})
		return
	}

	var body assignExperimentBody
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.VisitorID) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "visitorId is required"})
		return
	}

	landingPageID, err := services.AssignExperimentVariant(experimentID, body.VisitorID)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrExperimentStopped):
			c.JSON(http.StatusGone, gin.H{"success": false, "message": "This experiment is no longer running"})
		case errors.Is(err, services.ErrNoActiveSets):
			c.JSON(http.StatusConflict, gin.H{"success": false, "message": "This experiment has no active variants"})
		case errors.Is(err, gorm.ErrRecordNotFound):
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Experiment not found"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to assign a variant", "error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Variant assigned successfully", "data": gin.H{"landingPageId": landingPageID}})
}
