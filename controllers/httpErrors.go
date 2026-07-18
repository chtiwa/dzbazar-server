package controllers

import "github.com/gin-gonic/gin"

// respondError emits the shape actually used across every handler in this
// package ({success, message, error?}) — not the {error, code} shape
// CLAUDE.md described, which was never implemented. Use this for new code
// so error responses stay consistent going forward; existing call sites are
// left as-is (a ~700-site migration is its own separate pass).
func respondError(c *gin.Context, status int, message string, err error) {
	h := gin.H{"success": false, "message": message}
	if err != nil {
		h["error"] = err.Error()
	}
	c.JSON(status, h)
}
