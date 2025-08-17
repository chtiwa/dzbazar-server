package main

import (
	"fmt"

	"github.com/chtiwa/lk-parfumo-server/initializers"
	"github.com/chtiwa/lk-parfumo-server/middleware"
	"github.com/chtiwa/lk-parfumo-server/migrate"
	"github.com/chtiwa/lk-parfumo-server/realtime"
	"github.com/chtiwa/lk-parfumo-server/routes"
	"github.com/gin-gonic/gin"
)

func init() {
	initializers.LoadEnvVars()
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
	routes.WebSocketRoutes(router)

	go realtime.StartHub()

	fmt.Println("The server is running successfully!")

	router.Run()
}
