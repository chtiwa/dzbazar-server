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
	migrate.Migrate()
}

func main() {
	router := gin.Default()

	router.Use(middleware.CORSMiddleware())

	// routes
	routes.OrdersRoutes(router)
	routes.UsersRoutes(router)
	routes.WebSocketRoutes(router)

	go realtime.StartHub()

	fmt.Println("The server is running successfully!")

	router.Run()
}
