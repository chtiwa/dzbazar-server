package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/chtiwa/dzbazar-server/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// seedExtras adds the non-order-flow data: coupons, landing pages, A/B test,
// offers, stock edge cases, tracked/shipped orders, hidden+banned clients,
// abandoned leads and visit analytics. Runs inside the main seed transaction.
func seedExtras(tx *gorm.DB, shop models.Shop, owner models.User, members map[string]*models.ShopMember,
	products []models.Product, combos []models.ProductVariantCombination, clients []models.Client) {

	ago := func(d time.Duration) time.Time { return time.Now().Add(-d) }
	day := 24 * time.Hour

	// --- Stock edge cases: tracked products with low / zero stock.
	// combos[0..5] belong to product 0 (T-shirt, 6 variants); combos[6..] are the single-SKU products.
	must(tx.Model(&products[1]).Update("track_stock", true).Error)
	must(tx.Model(&products[2]).Update("track_stock", true).Error)
	must(tx.Model(&products[3]).Update("track_stock", true).Error)
	must(tx.Model(&models.ProductVariantCombination{}).Where("id = ?", combos[6].ID).Update("quantity", 3).Error)  // low
	must(tx.Model(&models.ProductVariantCombination{}).Where("id = ?", combos[7].ID).Update("quantity", 0).Error)  // out
	must(tx.Model(&models.ProductVariantCombination{}).Where("id = ?", combos[8].ID).Update("quantity", 12).Error) // ok

	// --- Coupons: shop-wide, product-scoped, inactive.
	must(tx.Create(&models.Coupon{ShopID: shop.ID, Code: "WELCOME10", Percent: 10, Active: true}).Error)
	scoped := models.Coupon{ShopID: shop.ID, Code: "MONTRE20", Percent: 20, Active: true}
	must(tx.Omit("Products", "LandingPages").Create(&scoped).Error)
	must(tx.Model(&scoped).Association("Products").Append(&products[1]))
	must(tx.Create(&models.Coupon{ShopID: shop.ID, Code: "EXPIRED50", Percent: 50, Active: false}).Error)

	// --- Landing pages (product 0 has 2 sets for the A/B test, product 1 has a standalone one).
	mkLP := func(p models.Product, title string, exp *uuid.UUID, pos int) models.LandingPage {
		lp := models.LandingPage{ShopID: shop.ID, ProductID: p.ID, Title: title, Active: true, ExperimentID: exp, ExperimentPosition: pos}
		must(tx.Omit("Shop", "Product", "Images").Create(&lp).Error)
		must(tx.Create(&models.LandingPageImage{LandingPageID: lp.ID, URL: "https://picsum.photos/seed/" + lp.ID.String()[:6] + "/800/1200"}).Error)
		return lp
	}
	solo := mkLP(products[1], "Montre — page promo", nil, 0)
	exp := models.LandingPageExperiment{ShopID: shop.ID, ProductID: products[0].ID, Name: "T-shirt: héro A vs B",
		TargetConversions: 30, Status: models.ExperimentStatusRunning, AssignmentCursor: 40}
	must(tx.Omit("Sets").Create(&exp).Error)
	setA := mkLP(products[0], "T-shirt — Set A", &exp.ID, 0)
	setB := mkLP(products[0], "T-shirt — Set B", &exp.ID, 1)
	// Finished experiment with a winner, for the "decided" UI state.
	done := models.LandingPageExperiment{ShopID: shop.ID, ProductID: products[2].ID, Name: "Écouteurs: ancien test",
		TargetConversions: 20, Status: models.ExperimentStatusDecided}
	must(tx.Omit("Sets").Create(&done).Error)
	dA := mkLP(products[2], "Écouteurs — Set A", &done.ID, 0)
	dB := mkLP(products[2], "Écouteurs — Set B", &done.ID, 1)
	must(tx.Model(&done).Update("winner_landing_page_id", dB.ID).Error)

	// --- Offers (promotions): one per OfferType.
	qtyType, varType, crossType := "quantity_upsell", "variant_upsell", "cross_sell"
	offers := []models.Offer{
		{InternalName: "2 pour le prix de 1.8", Status: "published", Action: "mutate_qty", OfferType: &qtyType,
			TriggerProductID: products[1].ID, OfferProductID: products[1].ID, Placement: "under_variant",
			QuantityPackages: []models.OfferQuantityPackage{{Quantity: 1, TotalPrice: 6900}, {Quantity: 2, TotalPrice: 12000, Description: "-13%"}},
			Headline:         "Achetez 2, économisez", ButtonText: "Choisir"},
		{InternalName: "Upsell écouteurs", Status: "published", Action: "append", OfferType: &crossType,
			TriggerProductID: products[1].ID, OfferProductID: products[2].ID, Placement: "above_submit",
			DiscountType: "percent", DiscountValue: 15, Headline: "Ajoutez des écouteurs -15%", ButtonText: "Ajouter"},
		{InternalName: "Coque en cadeau (brouillon)", Status: "draft", Action: "append", OfferType: &varType,
			TriggerProductID: products[0].ID, OfferProductID: products[5].ID, Placement: "pre_submit_modal",
			DiscountType: "override_price", DiscountValue: 0, Headline: "Une coque offerte", ButtonText: "Oui !"},
	}
	for i := range offers {
		offers[i].ShopID = shop.ID
		offers[i].QuantityRule = 1
		offers[i].Priority = 100 + i
		offers[i].InventoryBehavior = "skip_when_oos"
		if offers[i].DiscountType == "" {
			offers[i].DiscountType = "percent"
		}
		offers[i].Conditions = models.OfferConditions{Match: "all", Predicates: []models.OfferPredicate{}}
		must(tx.Omit("TriggerProduct", "LandingPage", "OfferProduct").Create(&offers[i]).Error)
		for k := 0; k < 30; k++ {
			ev := "impression"
			if k%5 == 0 {
				ev = "accept"
			} else if k%3 == 0 {
				ev = "click"
			}
			must(tx.Omit("Offer").Create(&models.OfferEvent{ShopID: shop.ID, OfferID: offers[i].ID, Event: ev}).Error)
		}
	}

	// --- Visits: unique visitors per page (drives landing-page conversion rates).
	visit := func(pageType string, entity uuid.UUID, n int) {
		for i := 0; i < n; i++ {
			must(tx.Exec(`INSERT INTO page_visits (shop_id, page_type, entity_id, day, visitor_id, created_at)
				VALUES (?,?,?,?,?,now()) ON CONFLICT DO NOTHING`,
				shop.ID, pageType, entity, time.Now().Add(-time.Duration(i%14)*day).Format("2006-01-02"), fmt.Sprintf("v-%s-%d", entity.String()[:6], i)).Error)
		}
	}
	visit("landing_page", setA.ID, 120)
	visit("landing_page", setB.ID, 118)
	visit("landing_page", solo.ID, 80)
	visit("landing_page", dA.ID, 60)
	visit("landing_page", dB.ID, 60)
	for _, p := range products {
		visit("product", p.ID, 50)
	}
	for i := 0; i < 14; i++ {
		for v := 0; v < 20+i*3; v++ {
			must(tx.Create(&models.ShopVisit{ShopID: shop.ID, Day: ago(time.Duration(i) * day), VisitorID: fmt.Sprintf("sv-%d-%d", i, v)}).Error)
		}
	}

	// --- Carrier + tracked ("suivi") orders.
	var carrier models.AvailableDeliveryCompany
	hasCarrier := tx.First(&carrier).Error == nil
	if hasCarrier {
		must(tx.Omit("AvailableDeliveryCompany").Create(&models.DeliveryCompany{ShopID: shop.ID, AvailableDeliveryCompanyID: carrier.ID,
			Token: "seed-fake-token", MerchantID: "seed", IsActive: true}).Error)
	}

	mkOrder := func(cl models.Client, cb models.ProductVariantCombination, qty uint, status string, age time.Duration, mut func(*models.Order)) models.Order {
		ship := 600.0
		o := models.Order{ShopID: shop.ID, ClientID: cl.ID, ShippingMethod: "doorstep", ShippingPrice: ship,
			TotalPrice: cb.Price*float64(qty) + ship, Status: status, State: cl.State,
			AssignedMemberID: &members["confirmation"].ID, AssignedAt: ptr(ago(age))}
		o.CreatedAt, o.UpdatedAt = ago(age), ago(age)
		if mut != nil {
			mut(&o)
		}
		must(tx.Omit("Client", "Items", "ShippedVia", "AssignedMember").Create(&o).Error)
		must(tx.Omit("Product", "ProductVariantCombination").Create(&models.OrderItem{OrderID: o.ID, ProductID: cb.ProductID,
			ProductVariantCombinationID: cb.ID, Quantity: qty, Price: cb.Price}).Error)
		return o
	}
	audit := func(o models.Order, from, to string, at time.Duration) {
		meta, _ := json.Marshal(map[string]string{"from": from, "to": to})
		al := models.AuditLog{ActorID: owner.ID, ActorEmail: owner.Email, Action: "order.status_changed", TargetType: "Order", TargetID: &o.ID, Metadata: string(meta)}
		al.CreatedAt = ago(at)
		must(tx.Create(&al).Error)
	}
	// Shipped orders at each tracking stage. Shipped-state strings mirror the ones the seed already uses.
	for i, st := range []string{"Expédié", "En livraison", "Livré", "Retour"} {
		o := mkOrder(clients[i], combos[(6+i)%len(combos)], 1, st, time.Duration(10-i)*day, func(o *models.Order) {
			o.IsShipped = true
			o.TrackingNumber = fmt.Sprintf("SEED-TRK-%05d", 1000+i)
			o.ShippedAt = ptr(ago(time.Duration(9-i) * day))
			if hasCarrier {
				o.ShippedViaID = &carrier.ID
			}
		})
		audit(o, "En attente", "Confirmé", time.Duration(10-i)*day)
		audit(o, "Confirmé", st, time.Duration(9-i)*day)
	}

	// --- Coupon + landing-page attributed orders (feeds A/B conversion counts).
	var wc models.Coupon
	must(tx.Where("shop_id = ? AND code = ?", shop.ID, "WELCOME10").First(&wc).Error)
	for i := 0; i < 12; i++ {
		lp := setA
		if i%3 == 0 {
			lp = setB
		}
		mkOrder(clients[i%len(clients)], combos[i%6], 1, "Confirmé", time.Duration(i+1)*day, func(o *models.Order) {
			o.LandingPageID = &lp.ID
			if i%2 == 0 {
				o.CouponID = &wc.ID
				o.DiscountAmount = 250
				o.TotalPrice -= 250
			}
		})
	}
	// Rescheduled + no-answer states.
	mkOrder(clients[0], combos[8], 1, "Reporté", 2*day, func(o *models.Order) { o.ReportedDate = ptr(time.Now().Add(3 * day)) })
	mkOrder(clients[1], combos[9], 1, "Ne répond pas 2", 1*day, nil)

	// --- Banned / hidden: flagged by each reason, plus a banned client and an FB-flagged browser.
	for i, reason := range []string{"cussword", "banned_client", "incognito", "vpn", "datacenter"} {
		cl := models.Client{ShopID: shop.ID, FullName: "Suspect " + reason, PhoneNumber: fmt.Sprintf("067000%04d", i),
			State: "Alger", StateCode: "16", City: "Alger", Banned: reason == "banned_client"}
		must(tx.Create(&cl).Error)
		mkOrder(cl, combos[6], 1, "En attente", time.Duration(i+1)*day, func(o *models.Order) {
			o.IsHidden, o.HiddenReason = true, reason
			o.ClientIP = fmt.Sprintf("41.200.0.%d", i+1)
			o.FBp = fmt.Sprintf("fb.1.seed.%d", i)
		})
	}
	must(tx.Create(&models.FlaggedClient{ShopID: shop.ID, Platform: "facebook", ClientID: "fb.1.seed.0"}).Error)
	resolved := models.FlaggedClient{ShopID: shop.ID, Platform: "facebook", ClientID: "fb.1.seed.1", ResolvedAt: ptr(ago(day)), ResolvedBy: &owner.ID}
	must(tx.Create(&resolved).Error)

	// --- Abandoned leads.
	for i := 0; i < 8; i++ {
		p := products[i%len(products)]
		must(tx.Create(&models.AbandonedLead{ShopID: shop.ID, ProductID: p.ID, ProductTitle: p.Title, Price: p.Price,
			CombinationStr: "Default", State: "Oran", City: "Es Senia", ShippingMethod: "doorstep", Quantity: 1 + i%2,
			FullName: fmt.Sprintf("Prospect %d", i+1), PhoneNumber: fmt.Sprintf("077000%04d", i), ConversionSource: "facebook"}).Error)
	}
}

func ptr[T any](v T) *T { return &v }
