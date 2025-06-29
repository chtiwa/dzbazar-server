package main

import (
	"fmt"

	"github.com/chtiwa/herbs-store-client/initializers"
	"github.com/chtiwa/herbs-store-client/middleware"
	"github.com/chtiwa/herbs-store-client/migrate"
	"github.com/chtiwa/herbs-store-client/realtime"
	"github.com/chtiwa/herbs-store-client/routes"
	"github.com/gin-gonic/gin"
)

func init() {
	initializers.LoadEnvVars()
	initializers.ConnectToDB()
	initializers.InitAWS()
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
	routes.CategoriesRoutes(router)
	routes.WebSocketRoutes(router)

	go realtime.StartHub()

	fmt.Println("The server is running successfully!")

	router.Run()
}
