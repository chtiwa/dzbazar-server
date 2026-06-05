package main

import (
	"fmt"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/chtiwa/dzbazar-server/migrate"

	// "github.com/chtiwa/dzbazar-server/migrate"
	"github.com/chtiwa/dzbazar-server/realtime"
	"github.com/chtiwa/dzbazar-server/routes"
	"github.com/gin-gonic/gin"
)

func init() {
	initializers.LoadEnvVars()
	initializers.InitStaticData()
	initializers.ConnectToDB()
	initializers.InitB2()
	initializers.InitRedis()
	migrate.Migrate()
}

func main() {
	router := gin.Default()

	router.Use(middleware.CORSMiddleware())

	// setting a lower memory limit for multipart forms
	router.MaxMultipartMemory = 20 << 20 //20 MiB
	// routes
	routes.OrdersRoutes(router)
	routes.UsersRoutes(router)
	routes.ProductsRoutes(router)
	routes.LandingPagesRoutes(router)
	routes.ShopsRoutes(router)
	routes.PixelsRoutes(router)
	routes.DashboardRoutes(router)
	routes.DeliveryRatesRoutes(router)
	routes.DeliveryCompaniesRoutes(router)
	routes.ClientsRoutes(router)
	routes.PlansRoutes(router)
	// routes.WebSocketRoutes(router)

	go realtime.StartHub()

	fmt.Println("The server is running successfully!")

	router.Run()
}
