package controllers

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	maxHelpChatMessages = 12
	maxHelpChatBytes    = 6000 // per message; also bounds tool results fed back by the browser
)

type helpChatMessage struct {
	Role       string                `json:"role"`
	Content    string                `json:"content"`
	ToolCalls  []services.AiToolCall `json:"tool_calls"`
	ToolCallID string                `json:"tool_call_id"`
}

// AskHelpChat runs one model turn of the merchant help chat. The browser owns
// the conversation: it sends the full message list (user / assistant / tool)
// and, when the answer carries toolCalls, runs them with the merchant's own
// session and calls again with the results. Every call costs CreditCostChat.
func AskHelpChat(c *gin.Context) {
	var body struct {
		Messages []helpChatMessage `json:"messages"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || len(body.Messages) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "messages are required"})
		return
	}

	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid shop ID"})
		return
	}

	// Tools are Pro-only and further limited by this member's permissions.
	var tools []services.AgentTool
	allowed, err := services.HasAssistantActions(shopID)
	if err != nil {
		RespondError(c, http.StatusInternalServerError, "Failed to verify plan limits", err)
		return
	}
	if allowed {
		role, _ := c.Get("userShopRole")
		var memberID *uuid.UUID
		if v, ok := c.Get("shopMemberID"); ok {
			id := v.(uuid.UUID)
			memberID = &id
		}
		roleStr, _ := role.(string)
		tools = services.AgentToolsFor(memberID, roleStr)
	}
	toolNames := map[string]bool{}
	for _, t := range tools {
		toolNames[t.Name] = true
	}

	msgs := body.Messages
	if len(msgs) > maxHelpChatMessages {
		msgs = msgs[len(msgs)-maxHelpChatMessages:]
	}
	// Trimming can leave an orphaned tool result at the front, which the
	// provider rejects; the window must start at a user message.
	for len(msgs) > 0 && msgs[0].Role != "user" {
		msgs = msgs[1:]
	}
	if len(msgs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "messages are required"})
		return
	}

	history := make([]services.AiChatMessage, 0, len(msgs))
	for _, m := range msgs {
		if len(m.Content) > maxHelpChatBytes {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "message too long"})
			return
		}
		switch m.Role {
		case "user", "assistant", "tool":
		default:
			continue
		}
		for _, tc := range m.ToolCalls {
			if !toolNames[tc.Function.Name] {
				c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "tool not available"})
				return
			}
		}
		history = append(history, services.AiChatMessage{
			Role: m.Role, Content: m.Content, ToolCalls: m.ToolCalls, ToolCallID: m.ToolCallID,
		})
	}

	// Chat is no longer free on any tier: pre-flight one call's credits so a
	// shop out of budget (trial plans have none) fails before the provider call.
	if err := services.CheckCreditBudget(shopID, services.CreditCostChat); err != nil {
		if errors.Is(err, services.ErrPlanLimitReached) {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "Not enough AI credits. Upgrade your plan for more.",
				"code":    "PLAN_LIMIT_REACHED",
			})
			return
		}
		RespondError(c, http.StatusInternalServerError, "Failed to verify plan limits", err)
		return
	}

	answer, toolCalls, usage, err := services.AskHelpChat(history, tools)
	if err != nil {
		if errors.Is(err, services.ErrAIUnconfigured) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "Help chat is not configured"})
			return
		}
		RespondError(c, http.StatusBadGateway, "Failed to get an answer", err)
		return
	}

	// The model may only call tools it was offered.
	for _, tc := range toolCalls {
		if !toolNames[tc.Function.Name] {
			RespondError(c, http.StatusBadGateway, "Failed to get an answer", fmt.Errorf("model requested unknown tool %q", tc.Function.Name))
			return
		}
	}

	var userID *uuid.UUID
	if u, ok := c.Get("user"); ok {
		if userData, ok := u.(models.User); ok {
			userID = &userData.ID
		}
	}
	if err := services.RecordAiChatUsage(shopID, userID, usage); err != nil {
		// Non-fatal: the merchant already got their answer.
		fmt.Printf("failed to record AI chat usage for shop %s: %v\n", shopID, err)
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "answer": strings.TrimSpace(answer), "toolCalls": toolCalls})
}
