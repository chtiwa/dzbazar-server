package routes

import (
	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/gin-gonic/gin"
)

// HealthRoutes is deliberately unauthenticated and unversioned (not under
// /v1) — it's an infra probe, not an API resource.
func HealthRoutes(router *gin.Engine) {
	router.GET("/health", controllers.Health)
}
