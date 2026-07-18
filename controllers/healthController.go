package controllers

import (
	"context"
	"net/http"
	"time"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/gin-gonic/gin"
)

// Health pings DB and Redis with a short timeout so Railway/uptime checks
// can tell "booting" apart from "a dependency is actually gone", and so the
// check itself can never hang the probe.
func Health(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	sqlDB, dbErr := initializers.DB.DB()
	if dbErr == nil {
		dbErr = sqlDB.PingContext(ctx)
	}
	redisErr := initializers.RClient.Ping(ctx).Err()

	if dbErr != nil {
		respondError(c, http.StatusServiceUnavailable, "database unreachable", dbErr)
		return
	}
	if redisErr != nil {
		respondError(c, http.StatusServiceUnavailable, "redis unreachable", redisErr)
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
