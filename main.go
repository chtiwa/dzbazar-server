package main

import (
	"fmt"

	"github.com/chtiwa/herbs-store-client/initializers"
	"github.com/chtiwa/herbs-store-client/migrate"
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

	routes.OrdersRoutes(router)
	// routes.UsersRoutes(router)

	fmt.Println("The server is running successfully!")
	router.Run()
}
