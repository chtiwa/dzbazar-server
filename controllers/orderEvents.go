package controllers

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/realtime"
	"github.com/chtiwa/dzbazar-server/utils"
	"github.com/google/uuid"
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
		fmt.Println("order events queue full, dropping side-effects for order", orderID)
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

	// Meta Purchase fires here, at creation. Firing it on confirmation instead
	// tanked ad delivery — Meta's optimization needs the signal close to the
	// click, not hours later once an admin gets to it.
	if fullOrder.ConversionSource == "facebook" {
		var px models.Pixel
		pixelErr := initializers.DB.
			Where("shop_id = ? AND platform = ? AND is_active = ?", fullOrder.ShopID, "facebook", true).
			First(&px).Error

		if pixelErr == nil && px.HasAccessToken && px.AccessToken != "" {
			testCode := ""
			if strings.Contains(strings.ToLower(fullOrder.Client.FullName), "test") {
				testCode = os.Getenv("FACEBOOK_TEST_CODE")
			}

			fbErr := utils.SendFacebookPurchase(
				px.PixelID,
				px.AccessToken,
				fullOrder.ID.String(),
				fullOrder.Client.FullName,
				fullOrder.Client.PhoneNumber,
				fullOrder.TotalPrice,
				"DZD",
				fullOrder.FBc,
				fullOrder.FBp,
				fullOrder.CreatedAt,
				fullOrder.ClientUserAgent,
				fullOrder.ClientIP,
				testCode,
			)
			if fbErr != nil {
				fmt.Println("Meta CAPI purchase send failed:", fbErr)
			} else {
				initializers.DB.Model(&models.Order{}).Where("id = ?", orderID).
					Update("meta_purchase_sent_at", time.Now())
			}
		}
	}
}
