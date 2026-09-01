package controllers

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/chtiwa/dzbazar-server/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const maxHelpChatHistory = 10
const freeTierChatQuestionsPerHour = 10

// AskHelpChat answers a merchant's "how do I do X in this dashboard" question.
func AskHelpChat(c *gin.Context) {
	var body struct {
		Question string `json:"question"`
		History  []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"history"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.Question) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "question is required"})
		return
	}

	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid shop ID"})
		return
	}

	// Chat costs no credits on any tier. Free tiers instead get an hourly
	// question cap so an idle trial can't run up an unbounded provider bill;
	// paid tiers are unmetered. Only counted once the question is known-valid,
	// so a malformed request doesn't burn the merchant's hourly allowance.
	free, err := services.IsFreeTier(shopID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to verify plan limits", "error": err.Error()})
		return
	}
	if free && !middleware.AllowShopAction("help-chat-hourly", shopID.String(), freeTierChatQuestionsPerHour, time.Hour) {
		c.JSON(http.StatusTooManyRequests, gin.H{
			"success": false,
			"message": "You've reached the free plan's limit of 10 questions per hour. Upgrade for unlimited chat.",
			"code":    "CHAT_LIMIT_REACHED",
		})
		return
	}

	if len(body.History) > maxHelpChatHistory {
		body.History = body.History[len(body.History)-maxHelpChatHistory:]
	}
	history := make([]services.AiChatMessage, 0, len(body.History))
	for _, m := range body.History {
		if m.Role != "user" && m.Role != "assistant" {
			continue
		}
		history = append(history, services.AiChatMessage{Role: m.Role, Content: m.Content})
	}

	answer, _, err := services.AskHelpChat(strings.TrimSpace(body.Question), history)
	if err != nil {
		if errors.Is(err, services.ErrAIUnconfigured) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "Help chat is not configured"})
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": "Failed to get an answer", "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "answer": answer})
}
