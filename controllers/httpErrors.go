package controllers

import (
	"errors"
	"log"
	"net/http"

	"github.com/chtiwa/dzbazar-server/services"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RespondError emits the shape used across every handler in this codebase
// ({success, message, error?} — not the {error, code} shape CLAUDE.md
// described, which was never implemented). The raw error is logged
// server-side only; the client never sees err.Error(), so SQL/internal
// error strings don't leak to the UI.
func RespondError(c *gin.Context, status int, message string, err error) {
	if err != nil {
		log.Printf("%s %s: %s: %v", c.Request.Method, c.Request.URL.Path, message, err)
	}
	c.JSON(status, gin.H{"success": false, "message": message})
}

// reserveAICredits locks + budget-checks an AI call, writing the error response itself.
// Caller must defer release() when ok.
func reserveAICredits(c *gin.Context, shopID uuid.UUID, need int) (release func(), ok bool) {
	release, err := services.ReserveCredits(shopID, need)
	switch {
	case err == nil:
		return release, true
	case errors.Is(err, services.ErrAIBusy):
		c.JSON(http.StatusTooManyRequests, gin.H{"success": false, "message": "Another AI request is still running. Try again in a moment.", "code": "AI_BUSY"})
	case errors.Is(err, services.ErrPlanLimitReached):
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "Not enough AI credits. Upgrade your plan for more.", "code": "PLAN_LIMIT_REACHED"})
	default:
		RespondError(c, http.StatusInternalServerError, "Failed to verify plan limits", err)
	}
	return nil, false
}
