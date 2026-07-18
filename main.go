package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/chtiwa/dzbazar-server/migrate"
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

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	router := gin.Default()

	router.Use(middleware.CORSMiddleware())

	// setting a lower memory limit for multipart forms
	router.MaxMultipartMemory = 20 << 20 //20 MiB
	// routes
	routes.HealthRoutes(router)
	routes.OrdersRoutes(router)
	routes.UsersRoutes(router)
	routes.ProductsRoutes(router)
	routes.StockRoutes(router)
	routes.LandingPagesRoutes(router)
	routes.CouponsRoutes(router)
	routes.ShopsRoutes(router)
	routes.PixelsRoutes(router)
	routes.VisitsRoutes(router)
	routes.DashboardRoutes(router)
	routes.DeliveryRatesRoutes(router)
	routes.DeliveryCompaniesRoutes(router)
	routes.ClientsRoutes(router)
	routes.PlansRoutes(router)
	routes.OsenRoutes(router)
	routes.LeopardRoutes(router)
	routes.ZrRoutes(router)
	routes.OffersRoutes(router)
	routes.AbandonedLeadsRoutes(router)
	routes.SuperAdminRoutes(router)
	routes.WebSocketRoutes(router)

	// Order side-effects (email/pixel/broadcast) run on a bounded worker
	// pool — must start before any order can be created.
	controllers.StartOrderEventWorkers(4)

	go realtime.StartHub()
	go realtime.StartSubscriber()
	go controllers.StartOsenStatusSync()
	go controllers.StartZrStatusSync()
	go controllers.StartSubscriptionExpiryReminders()

	srv := &http.Server{
		Addr:    ":" + envOr("PORT", "8080"),
		Handler: router,
	}

	go func() {
		fmt.Println("The server is running successfully!")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()

	// Block until Railway (or a local Ctrl-C) sends a termination signal.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	stop()

	fmt.Println("Shutting down: no new requests are being accepted...")

	// srv.Shutdown blocks until every in-flight HTTP handler returns, which
	// includes CreateOrderByShopID — so by the time it returns, no further
	// enqueueOrderEvent call can happen, and it's safe to close that queue.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		fmt.Println("Server shutdown error:", err)
	}

	controllers.DrainOrderEvents(10 * time.Second)
	fmt.Println("Shutdown complete.")
}
