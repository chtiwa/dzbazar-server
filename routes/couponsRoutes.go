package routes

import (
	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/gin-gonic/gin"
)

func CouponsRoutes(router *gin.Engine) {
	adminCoupons := router.Group("/v1/shops/:shopId/coupons")
	{
		adminCoupons.GET("", middleware.RequireAuthentication, middleware.RequireShopAccess("owner", "moderator"), controllers.GetCouponsByShop)
		adminCoupons.POST("", middleware.RequireAuthentication, middleware.RequireShopAccess("owner", "moderator"), controllers.CreateCoupon)
		adminCoupons.PATCH("/:id", middleware.RequireAuthentication, middleware.RequireShopAccess("owner", "moderator"), controllers.UpdateCoupon)
		adminCoupons.DELETE("/:id", middleware.RequireAuthentication, middleware.RequireShopAccess("owner"), controllers.DeleteCoupon)
	}

	router.POST("/v1/shops/:shopId/coupons/validate", controllers.ValidateCouponPublic)
	router.GET("/v1/shops/:shopId/coupons/available", controllers.CouponAvailableForProduct)
}
