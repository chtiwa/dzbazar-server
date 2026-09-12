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
	"github.com/chtiwa/dzbazar-server/services"
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

// Google Sheets order export retry sweep — see StartSheetsExportRetrySweep.
const (
	sheetsExportRetryAge     = 15 * time.Minute
	sheetsExportSweepEvery   = 15 * time.Minute
	sheetsExportMaxAttempts  = 5
	sheetsExportSweepLockKey = "lock:tick:sheets_export_sweep"
)

// orderEventPayload is what CreateOrderByShopID hands off to the worker
// pool. IsStaffOrder rides along because processOrderEvent runs with no
// *gin.Context — it can't re-derive middleware.IsStaffOrder(c) itself.
type orderEventPayload struct {
	OrderID      uuid.UUID
	IsStaffOrder bool
}

// Order side-effects (confirmation email, Meta CAPI purchase event, live
// dashboard broadcast) run on a small bounded worker pool instead of one raw
// goroutine per order — a traffic spike can no longer fan out unbounded
// goroutines, and DrainOrderEvents lets graceful shutdown wait for in-flight
// work instead of losing it mid-send when the process exits.
var (
	orderEvents  chan orderEventPayload
	orderEventWG sync.WaitGroup
)

// StartOrderEventWorkers must be called once at boot, before any order can
// be created, otherwise enqueueOrderEvent has nothing to send to.
func StartOrderEventWorkers(n int) {
	orderEvents = make(chan orderEventPayload, 256)
	for i := 0; i < n; i++ {
		go orderEventWorker()
	}
}

func orderEventWorker() {
	for evt := range orderEvents {
		runOrderEvent(evt)
	}
}

func runOrderEvent(evt orderEventPayload) {
	defer orderEventWG.Done()
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("Recovered from panic inside order event worker: %v\n", r)
		}
	}()
	processOrderEvent(evt.OrderID, evt.IsStaffOrder)
}

// enqueueOrderEvent hands an order off to the worker pool. If the queue is
// full (a sustained spike outrunning the workers), the event is dropped
// rather than blocking the checkout request — checkout must stay fast even
// if that means an occasional missed pixel/email under extreme load.
func enqueueOrderEvent(orderID uuid.UUID, isStaffOrder bool) {
	orderEventWG.Add(1)
	select {
	case orderEvents <- orderEventPayload{OrderID: orderID, IsStaffOrder: isStaffOrder}:
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
// isStaffOrder skips the merchant notification email — a staff member
// placing the order from the dashboard already knows about it; the email
// exists to alert staff to orders they don't yet know about.
func processOrderEvent(orderID uuid.UUID, isStaffOrder bool) {
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
	// Staff-created orders also skip the email: the person creating it from
	// the dashboard already knows about it, the email exists to alert staff
	// to orders they don't yet know about.
	if isProduction && !isStaffOrder && fullOrder.Status != "Confirmé" && !fullOrder.IsHidden {
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

		emailItems := make([]utils.OrderEmailItem, 0, len(fullOrder.Items))
		for _, item := range fullOrder.Items {
			name := item.Product.Title
			if name == "" {
				name = "Produit"
			}
			emailItems = append(emailItems, utils.OrderEmailItem{
				ProductName: name,
				Variant:     item.ProductVariantCombination.CombinationString,
				Quantity:    item.Quantity,
				UnitPrice:   item.Price,
				LineTotal:   item.Price * float64(item.Quantity),
			})
		}

		platform := fullOrder.ConversionSource
		if platform == "" {
			platform = "Organique"
		}

		if emailErr := utils.SendOrderEmail(
			shop.Name,
			recipients,
			fullOrder.Client.FullName,
			fullOrder.Client.PhoneNumber,
			fullOrder.Client.State,
			fullOrder.Client.City,
			platform,
			fullOrder.ShippingMethod,
			emailItems,
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
			"orderId":     fullOrder.ID.String(),
			"productName": mainProductName,
			"clientName":  fullOrder.Client.FullName,
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

	sendSheetsExportIfEligible(&fullOrder)
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

	accessToken, decErr := services.DecryptField(px.AccessToken)
	if decErr != nil {
		log.Printf("meta capi: skip order=%s shop=%s: failed to decrypt pixel access token: %v", order.ID, order.ShopID, decErr)
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
		accessToken,
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

// sendSheetsExportIfEligible appends one row to the shop's connected Google
// Sheet for this order, if an active integration exists. Shared by
// processOrderEvent (first attempt, at creation) and
// retryPendingSheetsExports (the reconciliation sweep), same idempotency
// shape as sendMetaPurchaseIfEligible: SheetsExportSentAt is the claim.
func sendSheetsExportIfEligible(order *models.Order) {
	var integ models.GoogleSheetsIntegration
	integErr := initializers.DB.
		Where("shop_id = ? AND is_active = ?", order.ShopID, true).
		First(&integ).Error

	if integErr != nil {
		return
	}

	initializers.DB.Model(&models.Order{}).Where("id = ?", order.ID).
		UpdateColumn("sheets_export_attempts", gorm.Expr("sheets_export_attempts + 1"))

	products := make([]string, 0, len(order.Items))
	for _, item := range order.Items {
		name := item.Product.Title
		if name == "" {
			name = "Produit"
		}
		combo := item.ProductVariantCombination.CombinationString
		if combo != "" {
			products = append(products, fmt.Sprintf("%s (%s) x%d", name, combo, item.Quantity))
		} else {
			products = append(products, fmt.Sprintf("%s x%d", name, item.Quantity))
		}
	}

	row := []interface{}{
		order.ID.String(),
		order.CreatedAt.Format("2006-01-02 15:04:05"),
		order.Client.FullName,
		order.Client.PhoneNumber,
		order.Client.State,
		order.Client.City,
		order.Client.StopdeskPoint,
		strings.Join(products, "; "),
		order.TotalPrice,
		order.Status,
	}

	svc, err := services.NewSheetsClient(integ.ServiceAccountJSON)
	if err == nil {
		err = services.AppendOrderRow(svc, integ.SpreadsheetID, integ.SheetName, row)
	}

	if err != nil {
		log.Printf("sheets export: append failed order=%s shop=%s: %v", order.ID, order.ShopID, err)
		integ.LastError = err.Error()
		initializers.DB.Save(&integ)
		return
	}

	now := time.Now()
	if updErr := initializers.DB.Model(&models.Order{}).Where("id = ?", order.ID).
		Update("sheets_export_sent_at", now).Error; updErr != nil {
		log.Printf("sheets export: sent but failed to stamp sheets_export_sent_at order=%s shop=%s: %v", order.ID, order.ShopID, updErr)
	}

	integ.LastSyncedAt = &now
	integ.LastError = ""
	initializers.DB.Save(&integ)
}

// StartSheetsExportRetrySweep periodically retries the Google Sheets export
// for orders whose shop has an active integration but the export never
// succeeded. Mirrors StartMetaPurchaseRetrySweep exactly. Intended to run in
// its own goroutine (see main.go).
func StartSheetsExportRetrySweep() {
	ticker := time.NewTicker(sheetsExportSweepEvery)
	defer ticker.Stop()

	for range ticker.C {
		if !utils.TryAcquireTickLock(sheetsExportSweepLockKey, sheetsExportSweepEvery-time.Minute) {
			continue
		}
		retryPendingSheetsExports()
	}
}

// retryPendingSheetsExports finds non-hidden, non-abandoned orders older
// than sheetsExportRetryAge whose shop currently has an active Sheets
// integration and never got a successful export, and retries them. Once an
// integration's pending orders exhaust sheetsExportMaxAttempts without a
// single success, it's deactivated below — a per-order cap alone would
// retry every new order forever against a permanently-broken credential.
func retryPendingSheetsExports() {
	var orders []models.Order
	cutoff := time.Now().Add(-sheetsExportRetryAge)

	eligibleShops := initializers.DB.Model(&models.GoogleSheetsIntegration{}).
		Select("shop_id").
		Where("is_active = ?", true)

	err := initializers.DB.
		Preload("Client").
		Preload("Items").
		Preload("Items.Product").
		Preload("Items.ProductVariantCombination").
		Where("sheets_export_sent_at IS NULL").
		Where("sheets_export_attempts < ?", sheetsExportMaxAttempts).
		Where("is_hidden = ?", false).
		Where("status <> ?", "Abandonné").
		Where("created_at < ?", cutoff).
		Where("shop_id IN (?)", eligibleShops).
		Find(&orders).Error

	if err != nil {
		log.Printf("sheets export retry sweep: query failed: %v", err)
		return
	}

	for i := range orders {
		sendSheetsExportIfEligible(&orders[i])
	}

	// Deactivate any integration whose pending orders have now exhausted
	// their attempts without a single success — prevents infinite retry
	// against a permanently-broken credential (revoked key, unshared sheet).
	initializers.DB.Exec(`
		UPDATE google_sheets_integrations
		SET is_active = false
		WHERE is_active = true
		AND shop_id IN (
			SELECT shop_id FROM orders
			WHERE sheets_export_sent_at IS NULL
			AND sheets_export_attempts >= ?
		)
	`, sheetsExportMaxAttempts)
}

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
