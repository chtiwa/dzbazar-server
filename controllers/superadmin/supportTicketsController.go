package superadmin

import (
	"net/http"
	"strings"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func ListSupportTickets(c *gin.Context) {
	status := strings.TrimSpace(c.Query("status"))
	priority := strings.TrimSpace(c.Query("priority"))
	search := strings.TrimSpace(c.Query("search"))
	page, perPage := parsePageParams(c)

	db := initializers.DB.Model(&models.SupportTicket{}).
		Preload("Shop").Preload("Requester").Preload("AssignedTo")

	if status != "" {
		db = db.Where("support_tickets.status = ?", status)
	}
	if priority != "" {
		db = db.Where("support_tickets.priority = ?", priority)
	}
	// Left join since shop_id is nullable — a ticket can outlive/precede a shop link.
	// Every column below is table-qualified (not just the joined ones) because an
	// unqualified "created_at" would be ambiguous once shops is joined in — both
	// tables carry it via BaseModel.
	if search != "" {
		like := "%" + strings.ToLower(search) + "%"
		db = db.Joins("LEFT JOIN shops ON shops.id = support_tickets.shop_id").
			Where("LOWER(support_tickets.subject) LIKE ? OR LOWER(shops.name) LIKE ?", like, like)
	}

	order := resolveSort(c, map[string]string{
		"status":     "support_tickets.status",
		"priority":   "support_tickets.priority",
		"created_at": "support_tickets.created_at",
	}, "support_tickets.created_at DESC")

	var totalRows int64
	db.Count(&totalRows)

	var tickets []models.SupportTicket
	if err := db.Order(order).
		Offset((page - 1) * perPage).Limit(perPage).
		Find(&tickets).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to fetch tickets", err)
		return
	}

	for i := range tickets {
		sanitize(&tickets[i].Requester)
		if tickets[i].AssignedTo != nil {
			sanitize(tickets[i].AssignedTo)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"data":       tickets,
		"pagination": paginationMeta(page, perPage, totalRows),
	})
}

func GetSupportTicket(c *gin.Context) {
	ticketID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid ticket ID"})
		return
	}

	var ticket models.SupportTicket
	if err := initializers.DB.
		Preload("Shop").Preload("Requester").Preload("AssignedTo").
		Preload("Messages", func(db *gorm.DB) *gorm.DB { return db.Order("created_at ASC") }).
		Preload("Messages.Author").
		First(&ticket, "id = ?", ticketID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Ticket not found"})
			return
		}
		RespondError(c, http.StatusInternalServerError, "Database error", err)
		return
	}

	sanitize(&ticket.Requester)
	if ticket.AssignedTo != nil {
		sanitize(ticket.AssignedTo)
	}
	for i := range ticket.Messages {
		sanitize(&ticket.Messages[i].Author)
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": ticket})
}

type CreateSupportTicketInput struct {
	ShopID   string `json:"shopId" binding:"required"`
	Subject  string `json:"subject" binding:"required"`
	Priority string `json:"priority"`
	Body     string `json:"body" binding:"required"`
}

// CreateSupportTicket lets a super admin log an issue on behalf of a shop —
// there is no tenant-side "file a ticket" widget yet, so the requester is
// resolved server-side from the shop's owner.
func CreateSupportTicket(c *gin.Context) {
	var body CreateSupportTicketInput
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondError(c, http.StatusBadRequest, "Validation failed", err)
		return
	}

	shopID, err := uuid.Parse(body.ShopID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid shop ID"})
		return
	}

	var shop models.Shop
	if err := initializers.DB.First(&shop, "id = ?", shopID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Shop not found"})
		return
	}

	priority := body.Priority
	if priority == "" {
		priority = "normal"
	}

	ticket := models.SupportTicket{
		ShopID:          &shop.ID,
		RequesterUserID: shop.OwnerID,
		Subject:         strings.TrimSpace(body.Subject),
		Status:          "open",
		Priority:        priority,
	}

	err = initializers.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&ticket).Error; err != nil {
			return err
		}
		message := models.SupportTicketMessage{TicketID: ticket.ID, AuthorUserID: shop.OwnerID, Body: body.Body}
		return tx.Create(&message).Error
	})

	if err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to create ticket", err)
		return
	}

	utils.LogAudit(c, "support_ticket.create", "SupportTicket", &ticket.ID, gin.H{"shopId": shop.ID, "priority": priority})
	c.JSON(http.StatusCreated, gin.H{"success": true, "message": "Ticket created", "data": ticket})
}

type UpdateSupportTicketInput struct {
	Status           *string `json:"status"`
	Priority         *string `json:"priority"`
	AssignedToUserID *string `json:"assignedToUserId"`
}

func UpdateSupportTicket(c *gin.Context) {
	ticketID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid ticket ID"})
		return
	}

	var body UpdateSupportTicketInput
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondError(c, http.StatusBadRequest, "Validation failed", err)
		return
	}

	var ticket models.SupportTicket
	if err := initializers.DB.First(&ticket, "id = ?", ticketID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Ticket not found"})
			return
		}
		RespondError(c, http.StatusInternalServerError, "Database error", err)
		return
	}

	updates := map[string]any{}
	if body.Status != nil {
		updates["status"] = *body.Status
	}
	if body.Priority != nil {
		updates["priority"] = *body.Priority
	}
	if body.AssignedToUserID != nil {
		if *body.AssignedToUserID == "" {
			updates["assigned_to_user_id"] = nil
		} else {
			assigneeID, err := uuid.Parse(*body.AssignedToUserID)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid assignee ID"})
				return
			}
			updates["assigned_to_user_id"] = assigneeID
		}
	}

	if len(updates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "No fields provided for update"})
		return
	}

	if err := initializers.DB.Model(&ticket).Updates(updates).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to update ticket", err)
		return
	}

	utils.LogAudit(c, "support_ticket.update", "SupportTicket", &ticket.ID, updates)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Ticket updated", "data": ticket})
}

type AddTicketMessageInput struct {
	Body           string `json:"body" binding:"required"`
	IsInternalNote bool   `json:"isInternalNote"`
}

func AddTicketMessage(c *gin.Context) {
	ticketID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid ticket ID"})
		return
	}

	var body AddTicketMessageInput
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondError(c, http.StatusBadRequest, "Validation failed", err)
		return
	}

	actor, ok := c.Get("user")
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Missing session user"})
		return
	}
	actorUser := actor.(models.User)

	var ticket models.SupportTicket
	if err := initializers.DB.First(&ticket, "id = ?", ticketID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Ticket not found"})
			return
		}
		RespondError(c, http.StatusInternalServerError, "Database error", err)
		return
	}

	message := models.SupportTicketMessage{
		TicketID:       ticket.ID,
		AuthorUserID:   actorUser.ID,
		Body:           strings.TrimSpace(body.Body),
		IsInternalNote: body.IsInternalNote,
	}

	if err := initializers.DB.Create(&message).Error; err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to add message", err)
		return
	}

	utils.LogAudit(c, "support_ticket.message", "SupportTicket", &ticket.ID, gin.H{"isInternalNote": message.IsInternalNote})
	c.JSON(http.StatusCreated, gin.H{"success": true, "message": "Reply added", "data": message})
}
