package routes

import (
	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// HealthRoutes is deliberately unauthenticated and unversioned (not under
// /v1) — it's an infra probe, not an API resource. /metrics lives here too
// for the same reason: an infra scrape target, not an API resource.
func HealthRoutes(router *gin.Engine) {
	router.GET("/health", controllers.Health)
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))
}
