package middleware

import (
	"log/slog"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Logger is the process-wide structured (JSON) logger. Replaces the default
// gin.Default() text logger — see RequestLogger below.
var Logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))

// RequestID stamps every request with an X-Request-ID (client-supplied if
// present, else generated), stored in gin context under "request_id" so
// handlers/logs can correlate a single request across log lines.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if id == "" {
			id = uuid.NewString()
		}
		c.Set("request_id", id)
		c.Header("X-Request-ID", id)
		c.Next()
	}
}

// RequestLogger replaces gin's default text access log with structured JSON
// (method, path, status, latency, request_id) — needed to grep/alert on
// production logs during an incident instead of scrolling raw text.
func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}

		c.Next()

		Logger.Info("request",
			"method", c.Request.Method,
			"path", path,
			"status", c.Writer.Status(),
			"latency_ms", time.Since(start).Milliseconds(),
			"request_id", c.GetString("request_id"),
			"client_ip", c.ClientIP(),
		)
	}
}

