package controllers

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/realtime"
	"github.com/chtiwa/dzbazar-server/utils"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Meta CAPI Purchase retry sweep — see StartMetaPurchaseRetrySweep.
const (
	metaPurchaseRetryAge     = 15 * time.Minute
	metaPurchaseSweepEvery   = 15 * time.Minute
	metaPurchaseMaxAttempts  = 5
	metaPurchaseSweepLockKey = "lock:tick:meta_purchase_sweep"
)

// Order side-effects (confirmation email, Meta CAPI purchase event, live
// dashboard broadcast) run on a small bounded worker pool instead of one raw
// goroutine per order — a traffic spike can no longer fan out unbounded
// goroutines, and DrainOrderEvents lets graceful shutdown wait for in-flight
// work instead of losing it mid-send when the process exits.
var (
	orderEvents  chan uuid.UUID
	orderEventWG sync.WaitGroup
)

// StartOrderEventWorkers must be called once at boot, before any order can
// be created, otherwise enqueueOrderEvent has nothing to send to.
func StartOrderEventWorkers(n int) {
	orderEvents = make(chan uuid.UUID, 256)
	for i := 0; i < n; i++ {
		go orderEventWorker()
	}
}

func orderEventWorker() {
	for orderID := range orderEvents {
		runOrderEvent(orderID)
	}
}

func runOrderEvent(orderID uuid.UUID) {
	defer orderEventWG.Done()
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("Recovered from panic inside order event worker: %v\n", r)
		}
	}()
	processOrderEvent(orderID)
}

// enqueueOrderEvent hands an order off to the worker pool. If the queue is
// full (a sustained spike outrunning the workers), the event is dropped
// rather than blocking the checkout request — checkout must stay fast even
// if that means an occasional missed pixel/email under extreme load.
func enqueueOrderEvent(orderID uuid.UUID) {
	orderEventWG.Add(1)
	select {
	case orderEvents <- orderID:
	default:
		orderEventWG.Done()
		log.Printf("order events: queue full, dropping side-effects order=%s", orderID)
	}
}

// DrainOrderEvents closes the queue (no further enqueues are possible after
// this — see the shutdown-ordering note in main.go) and waits up to timeout
// for in-flight workers to finish before returning.
func DrainOrderEvents(timeout time.Duration) {
	if orderEvents == nil {
		return
	}
	close(orderEvents)

	done := make(chan struct{})
	go func() {
		orderEventWG.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(timeout):
		fmt.Println("order event drain timed out, exiting with side-effects still in flight")
	}
}

// processOrderEvent sends the post-creation notifications for one order:
// merchant email, live dashboard broadcast, and the Meta CAPI purchase
// event. Moved verbatim out of the old per-order goroutine in
// CreateOrderByShopID; behavior is unchanged, only the scheduling around it.
func processOrderEvent(orderID uuid.UUID) {
	var fullOrder models.Order
	preloadErr := initializers.DB.
		Preload("Client").
		Preload("Items").
		Preload("Items.Product").
		Preload("Items.ProductVariantCombination").
		First(&fullOrder, "id = ?", orderID).Error

	if preloadErr != nil {
		fmt.Printf("Error hydrating order details context for async tasks: %v\n", preloadErr)
		return
	}

	isProduction := os.Getenv("APP_ENV") == "production"

	mainProductName := "Multi-item Order"
	if len(fullOrder.Items) > 0 {
		item := fullOrder.Items[0]
		if item.Product.Title != "" {
			mainProductName = item.Product.Title
		} else {
			comboStr := item.ProductVariantCombination.CombinationString
			if comboStr == "" {
				comboStr = "Standard"
			}
			mainProductName = fmt.Sprintf("Product SKU Variant: %s", comboStr)
		}
	}

	// Cussword orders are silently accepted for the client but kept out of
	// sight of the admin entirely — no notification email, no live broadcast.
	if isProduction && fullOrder.Status != "Confirmé" && !fullOrder.IsHidden {
		var shop models.Shop
		initializers.DB.Select("name").First(&shop, "id = ?", fullOrder.ShopID)

		var members []models.ShopMember
		initializers.DB.Preload("User").Where("shop_id = ?", fullOrder.ShopID).Find(&members)

		recipients := make([]string, 0, len(members))
		seen := make(map[string]struct{})
		for _, m := range members {
			if m.User.Email != "" {
				if _, ok := seen[m.User.Email]; !ok {
					seen[m.User.Email] = struct{}{}
					recipients = append(recipients, m.User.Email)
				}
			}
		}

		if emailErr := utils.SendOrderEmail(
			shop.Name,
			recipients,
			fullOrder.Client.FullName,
			fullOrder.Client.PhoneNumber,
			fullOrder.Client.State,
			fullOrder.Client.City,
			mainProductName,
			"See order components explicitly",
			fullOrder.ShippingMethod,
			1,
			fullOrder.TotalPrice,
			fullOrder.ShippingPrice,
			fullOrder.TotalPrice,
		); emailErr != nil {
			fmt.Println("Error sending notification alert email flow:", emailErr)
		}
	}

	if fullOrder.Status == "Abandonné" || fullOrder.IsHidden {
		return
	}

	select {
	case realtime.Broadcast <- realtime.Message{
		Event:  "order_created",
		ShopID: fullOrder.ShopID.String(),
		Data: map[string]any{
			"productName": mainProductName,
			"totalPrice":  fullOrder.TotalPrice,
			"itemsCount":  len(fullOrder.Items),
		},
	}:
	case <-time.After(5 * time.Second):
		fmt.Println("ws broadcast dropped: hub backpressure")
	}

	// Meta/TikTok Purchase fire here, at creation. Firing on confirmation
	// instead tanked ad delivery — Meta's optimization needs the signal close
	// to the click, not hours later once an admin gets to it.
	//
	// Eligibility no longer depends on the client-derived ConversionSource
	// (see getTrackingParams in client/src/utils/tracking.ts): that value is
	// decided from fbp/fbc/fbclid cookies, which iOS ITP's 7-day cap, in-app
	// browsers, and plain cookie clearing routinely wipe out — silently
	// mislabeling real Facebook-driven orders as "organic" and skipping the
	// send. Every non-hidden order for a shop with the matching pixel
	// configured now gets the send attempt, passing along whatever
	// fbc/fbp/fbclid/ttclid happen to be present; ConversionSource itself is
	// untouched and still used for attribution/ban-matching elsewhere
	// (ordersController.go).
	sendMetaPurchaseIfEligible(&fullOrder)
}

// sendMetaPurchaseIfEligible sends the Meta CAPI Purchase event for one
// order, if this shop has an active Facebook pixel with an access token
// configured. Shared by processOrderEvent (first attempt, at creation) and
// retryPendingMetaPurchases (the reconciliation sweep) so both paths use the
// exact same eligibility check and idempotency bookkeeping.
func sendMetaPurchaseIfEligible(order *models.Order) {
	var px models.Pixel
	pixelErr := initializers.DB.
		Where("shop_id = ? AND platform = ? AND is_active = ?", order.ShopID, "facebook", true).
		First(&px).Error

	if pixelErr != nil || !px.HasAccessToken || px.AccessToken == "" {
		log.Printf("meta capi: skip order=%s shop=%s: no active facebook pixel with access token configured", order.ID, order.ShopID)
		return
	}

	testCode := ""
	if strings.Contains(strings.ToLower(order.Client.FullName), "test") {
		testCode = os.Getenv("FACEBOOK_TEST_CODE")
	}

	initializers.DB.Model(&models.Order{}).Where("id = ?", order.ID).
		UpdateColumn("meta_purchase_attempts", gorm.Expr("meta_purchase_attempts + 1"))

	fbErr := utils.SendFacebookPurchase(
		px.PixelID,
		px.AccessToken,
		order.ID.String(),
		order.Client.FullName,
		order.Client.PhoneNumber,
		order.TotalPrice,
		"DZD",
		order.FBc,
		order.FBp,
		order.CreatedAt,
		order.ClientUserAgent,
		order.ClientIP,
		testCode,
		order.PageURL,
	)
	if fbErr != nil {
		log.Printf("meta capi: purchase send failed order=%s shop=%s: %v", order.ID, order.ShopID, fbErr)
		return
	}

	if updErr := initializers.DB.Model(&models.Order{}).Where("id = ?", order.ID).
		Update("meta_purchase_sent_at", time.Now()).Error; updErr != nil {
		log.Printf("meta capi: sent but failed to stamp meta_purchase_sent_at order=%s shop=%s: %v", order.ID, order.ShopID, updErr)
	}
}

// ponytail: TikTok CAPI (utils.SendTikTokPurchase) intentionally not wired in
// here — frontend-only TikTok tracking for now, per product decision. Wire it
// the same way as sendMetaPurchaseIfEligible above if that changes.

// StartMetaPurchaseRetrySweep periodically retries the Meta CAPI Purchase
// send for orders whose shop has Facebook CAPI configured but the send never
// succeeded (network blip, Meta API hiccup, etc) — meta_purchase_sent_at is
// the same idempotency claim sendMetaPurchaseIfEligible uses on the first
// attempt, so a retry here can never double-send once one attempt lands.
// Capped at metaPurchaseMaxAttempts so a permanently-misconfigured pixel
// doesn't retry forever. Intended to run in its own goroutine, alongside the
// order-event worker pool (see main.go).
func StartMetaPurchaseRetrySweep() {
	ticker := time.NewTicker(metaPurchaseSweepEvery)
	defer ticker.Stop()

	for range ticker.C {
		if !utils.TryAcquireTickLock(metaPurchaseSweepLockKey, metaPurchaseSweepEvery-time.Minute) {
			continue
		}
		retryPendingMetaPurchases()
	}
}

// retryPendingMetaPurchases finds non-hidden, non-abandoned orders older
// than metaPurchaseRetryAge whose shop currently has an eligible Facebook
// pixel, that never got a successful Meta CAPI send, and retries them. The
// shop_id subquery keeps this from re-scanning (and re-logging a skip for)
// every organic order at every shop that has no Facebook pixel at all —
// only orders that are actually retry candidates are selected.
func retryPendingMetaPurchases() {
	var orders []models.Order
	cutoff := time.Now().Add(-metaPurchaseRetryAge)

	eligibleShops := initializers.DB.Model(&models.Pixel{}).
		Select("shop_id").
		Where("platform = ? AND is_active = ? AND has_access_token = ? AND access_token <> ?", "facebook", true, true, "")

	err := initializers.DB.
		Preload("Client").
		Where("meta_purchase_sent_at IS NULL").
		Where("meta_purchase_attempts < ?", metaPurchaseMaxAttempts).
		Where("is_hidden = ?", false).
		Where("status <> ?", "Abandonné").
		Where("created_at < ?", cutoff).
		Where("shop_id IN (?)", eligibleShops).
		Find(&orders).Error

	if err != nil {
		log.Printf("meta capi retry sweep: query failed: %v", err)
		return
	}

	for i := range orders {
		sendMetaPurchaseIfEligible(&orders[i])
	}
}
