package controllers

import (
	"log"

	"github.com/gin-gonic/gin"
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
