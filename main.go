package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/chtiwa/dzbazar-server/controllers"
	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/middleware"
	"github.com/chtiwa/dzbazar-server/migrate"
	"github.com/chtiwa/dzbazar-server/realtime"
	"github.com/chtiwa/dzbazar-server/routes"
	"github.com/chtiwa/dzbazar-server/services"
	"github.com/gin-gonic/gin"
)

func init() {
	initializers.LoadEnvVars()

	// JWT_SECRET signs every access/refresh token (authController.go,
	// requireAuth.go, utils/generateTokenString.go, superadmin/
	// impersonationController.go) — read raw via os.Getenv with no prior
	// validation anywhere, so an unset/weak secret would only surface as a
	// confusing auth failure at request time. Fail fast at boot instead.
	if secret := os.Getenv("JWT_SECRET"); len(secret) < 32 {
		log.Fatalf("JWT_SECRET is not set or too short (must be at least 32 characters)")
	}

	initializers.InitStaticData()
	initializers.ConnectToDB()
	initializers.InitB2()
	initializers.InitRedis()
	migrate.Migrate()

	// Wilayas now live in Postgres (see services/wilayas.go), seeded by
	// migrate/migrations/00019_wilayas_table.sql — so warming the cache must
	// happen after migrate.Migrate() above, once the table is guaranteed to
	// exist, not alongside the other InitStaticData() checks earlier.
	if _, err := services.GetWilayas(); err != nil {
		log.Fatalf("failed to initialize wilayas: %v", err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// configureTrustedClientIP makes c.ClientIP() resolve to the real caller
// instead of an attacker-controlled header. gin.New()'s defaults trust every
// proxy (0.0.0.0/0), so a client can set X-Forwarded-For itself and forge
// any IP — this breaks every IP-keyed rate limit, fraud check and CAPI
// client-IP field (see important.todo LAUNCH BLOCKER 4).
//
//   - TRUSTED_PROXIES set (comma-separated CIDRs): SetTrustedProxies to that
//     list, so ClientIP() walks X-Forwarded-For from the right-most trusted
//     hop backwards to the first untrusted (real client) hop.
//   - TRUSTED_PROXIES unset in production: SetTrustedProxies(nil) (trust no
//     proxy hop) and set TrustedPlatform to the header Railway's own edge
//     sets with the real client IP (CLIENT_IP_HEADER, default "X-Real-IP")
//     — Railway's edge is the only hop that can reach this process directly,
//     so a client-supplied header can't reach it under that name.
//   - dev (APP_ENV != production) with no TRUSTED_PROXIES: leave Gin's
//     default alone so local requests keep working unchanged.
//
// Verify on Railway after deploy: temporarily log c.ClientIP() next to
// c.Request.Header for a few live requests and confirm ClientIP() matches
// the real caller, not something the request itself could set.
func configureTrustedClientIP(router *gin.Engine) {
	isProduction := os.Getenv("APP_ENV") == "production"
	trustedProxies := os.Getenv("TRUSTED_PROXIES")

	if trustedProxies != "" {
		cidrs := strings.Split(trustedProxies, ",")
		for i := range cidrs {
			cidrs[i] = strings.TrimSpace(cidrs[i])
		}
		if err := router.SetTrustedProxies(cidrs); err != nil {
			log.Fatalf("invalid TRUSTED_PROXIES: %v", err)
		}
		return
	}

	if isProduction {
		if err := router.SetTrustedProxies(nil); err != nil {
			log.Fatalf("failed to clear trusted proxies: %v", err)
		}
		header := envOr("CLIENT_IP_HEADER", "X-Real-IP")
		router.TrustedPlatform = header
		// If the edge doesn't actually send this header, ClientIP() silently
		// falls back to the proxy's IP and EVERY customer shares one rate-limit
		// bucket (orders get dropped platform-wide). Shout once so it's caught
		// on the first request after deploy, not as lost orders.
		var warnOnce sync.Once
		router.Use(func(c *gin.Context) {
			if c.GetHeader(header) == "" {
				warnOnce.Do(func() {
					log.Printf("WARNING: CLIENT_IP_HEADER %q missing on incoming request — all clients will share the proxy IP for rate limits. Fix CLIENT_IP_HEADER/TRUSTED_PROXIES.", header)
				})
			}
			c.Next()
		})
	}
}

func main() {
	router := gin.New()
	router.Use(gin.Recovery(), middleware.RequestID(), middleware.RequestLogger())

	configureTrustedClientIP(router)

	router.Use(middleware.CORSMiddleware())

	// cap non-multipart bodies (multipart uploads are bounded by their handlers)
	router.Use(func(c *gin.Context) {
		if !strings.HasPrefix(c.ContentType(), "multipart/") {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2<<20)
		}
		c.Next()
	})

	// setting a lower memory limit for multipart forms
	router.MaxMultipartMemory = 20 << 20 //20 MiB
	// routes
	routes.HealthRoutes(router)
	routes.OrdersRoutes(router)
	routes.UsersRoutes(router)
	routes.ProductsRoutes(router)
	routes.StockRoutes(router)
	routes.LandingPagesRoutes(router)
	routes.AIImageToolRoutes(router)
	routes.LandingPageExperimentsRoutes(router)
	routes.CouponsRoutes(router)
	routes.FeatureFlagsRoutes(router)
	routes.ShopsRoutes(router)
	routes.PixelsRoutes(router)
	routes.GoogleSheetsRoutes(router)
	routes.VisitsRoutes(router)
	routes.DashboardRoutes(router)
	routes.DeliveryRatesRoutes(router)
	routes.DeliveryCompaniesRoutes(router)
	routes.BureauxRoutes(router)
	routes.CommunesRoutes(router)
	routes.ClientsRoutes(router)
	routes.PlansRoutes(router)
	routes.OsenRoutes(router)
	routes.LeopardRoutes(router)
	routes.ZrRoutes(router)
	routes.AndersonRoutes(router)
	routes.OffersRoutes(router)
	routes.AbandonedLeadsRoutes(router)
	routes.NotificationsRoutes(router)
	routes.ConfirmatricesRoutes(router)
	routes.HelpChatRoutes(router)
	routes.SuperAdminRoutes(router)
	routes.WebSocketRoutes(router)

	// Order side-effects (email/pixel/broadcast) run on a bounded worker
	// pool — must start before any order can be created.
	controllers.StartOrderEventWorkers(4)
	go supervise("meta-purchase-sweep", controllers.StartMetaPurchaseRetrySweep)
	go supervise("sheets-export-sweep", controllers.StartSheetsExportRetrySweep)

	go supervise("ws-hub", realtime.StartHub)
	go supervise("ws-subscriber", realtime.StartSubscriber)
	go supervise("osen-sync", controllers.StartOsenStatusSync)
	go supervise("zr-sync", controllers.StartZrStatusSync)
	go supervise("anderson-sync", controllers.StartAndersonStatusSync)
	go supervise("subscription-reminders", controllers.StartSubscriptionExpiryReminders)

	srv := &http.Server{
		Addr:    ":" + envOr("PORT", "8080"),
		Handler: router,
		// Slowloris guard. No WriteTimeout on purpose: WS + AI-generation
		// handlers legitimately hold the response open for a long time.
		ReadHeaderTimeout: 10 * time.Second,
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

// supervise runs a long-lived background loop and restarts it if it panics
// or returns. gin.Recovery only covers HTTP handlers — an unrecovered panic in
// a bare goroutine (e.g. a carrier sync choking on a malformed API response)
// kills the whole process, taking every shop's storefront down with it.
func supervise(name string, fn func()) {
	for {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("background %s panicked: %v\n%s", name, r, debug.Stack())
				}
			}()
			fn()
		}()
		log.Printf("background %s exited, restarting in 5s", name)
		time.Sleep(5 * time.Second)
	}
}
