// Dev seed: go run ./cmd/seed
// Creates a demo owner/staff, one shop, products w/ variants, clients and orders.
// Idempotent (skips if seed user exists) and refuses to touch a DB that already
// holds non-seed users, unless SEED_FORCE=1.
package main

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/migrate"
	"github.com/chtiwa/dzbazar-server/models"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const (
	password   = "Password123!"
	seedDomain = "@seed.test"
	shopSlug   = "demo-shop"
)

func main() {
	initializers.LoadEnvVars()
	initializers.ConnectToDB()
	migrate.Migrate()
	db := initializers.DB

	var foreign int64
	db.Model(&models.User{}).Where("email NOT LIKE ?", "%"+seedDomain).Count(&foreign)
	if foreign > 0 && os.Getenv("SEED_FORCE") != "1" {
		log.Fatalf("DB has %d non-seed users; refusing to seed (SEED_FORCE=1 to override)", foreign)
	}
	var exists int64
	db.Model(&models.User{}).Where("email = ?", "owner"+seedDomain).Count(&exists)
	if exists > 0 {
		log.Println("already seeded, nothing to do")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), 10)
	must(err)

	must(db.Transaction(func(tx *gorm.DB) error {
		mkUser := func(first, last, email, role string) models.User {
			u := models.User{FirstName: first, LastName: last, PhoneNumber: "0550000000", Email: email + seedDomain,
				Password: string(hash), Role: role, IsVerified: true}
			must(tx.Omit("Memberships").Create(&u).Error)
			return u
		}
		owner := mkUser("Demo", "Owner", "owner", "owner")
		mod := mkUser("Demo", "Moderator", "moderator", "moderator")
		conf := mkUser("Demo", "Confirmation", "confirmation", "confirmation")

		shop := models.Shop{Name: "Demo Shop", Slug: shopSlug, Description: "Seeded dev shop", OwnerID: owner.ID, IsActive: true, IsVerified: true}
		must(tx.Omit("Owner", "Members", "Products", "Orders", "Clients", "LogoImage").Create(&shop).Error)

		members := map[string]*models.ShopMember{}
		for role, u := range map[string]models.User{"owner": owner, "moderator": mod, "confirmation": conf} {
			m := models.ShopMember{ShopID: shop.ID, UserID: u.ID, Role: role}
			must(tx.Omit("User", "Shop").Create(&m).Error)
			members[role] = &m
		}

		var wilayas []models.Wilaya
		must(tx.Order("id").Find(&wilayas).Error)
		for _, w := range wilayas {
			must(tx.Create(&models.DeliveryRate{ShopID: shop.ID, WilayaID: w.ID, WilayaName: w.Name, IsActive: w.IsActive,
				HasDoorstep: w.HasDoorstep, DoorstepRate: w.DoorstepRate, HasStopdesk: w.HasStopdesk, StopdeskRate: w.StopdeskRate}).Error)
			must(tx.Create(&models.Bureau{ShopID: shop.ID, WilayaID: w.ID, Name: "Bureau " + w.Name}).Error)
		}

		// Pro tier = highest caps, so nothing blocks dev work.
		var plan models.Plan
		must(tx.Where("name = ?", "Pro").First(&plan).Error)
		must(tx.Create(&models.ShopSubscription{ShopID: shop.ID, PlanID: plan.ID, StartedAt: time.Now()}).Error)

		// Products: first has Color x Size variants, the rest are single-SKU.
		type prod struct {
			title string
			price float64
		}
		prods := []prod{{"T-shirt coton premium", 2500}, {"Montre connectée", 6900}, {"Écouteurs sans fil", 3900},
			{"Sac à dos étanche", 4500}, {"Lampe LED rechargeable", 1800}, {"Coque iPhone 15", 900}}
		var combos []models.ProductVariantCombination
		var products []models.Product
		for i, p := range prods {
			old := p.price * 1.3
			pr := models.Product{ShopID: shop.ID, Title: p.title, Description: "Produit de démonstration : " + p.title,
				Price: p.price, OldPrice: &old, Active: true}
			must(tx.Omit("Images", "Variants", "Combinations").Create(&pr).Error)
			must(tx.Create(&models.ProductImage{ProductID: pr.ID, URL: fmt.Sprintf("https://picsum.photos/seed/dz%d/600/600", i)}).Error)
			products = append(products, pr)

			if i != 0 {
				c := models.ProductVariantCombination{ProductID: pr.ID, ShopID: shop.ID, SKU: fmt.Sprintf("P%d-DEF", i),
					Price: p.price, Quantity: 50, CombinationString: "Default"}
				must(tx.Omit("Product").Create(&c).Error)
				combos = append(combos, c)
				continue
			}
			colorV := models.Variant{ProductID: pr.ID, Title: "Couleur"}
			sizeV := models.Variant{ProductID: pr.ID, Title: "Taille"}
			must(tx.Omit("Product", "VariantItems").Create(&colorV).Error)
			must(tx.Omit("Product", "VariantItems").Create(&sizeV).Error)
			for _, col := range []string{"Noir", "Blanc"} {
				ci := models.VariantItem{VariantID: colorV.ID, Value: col}
				must(tx.Omit("Variant").Create(&ci).Error)
				for _, sz := range []string{"M", "L", "XL"} {
					si := models.VariantItem{VariantID: sizeV.ID, Value: sz}
					must(tx.Omit("Variant").Create(&si).Error)
					c := models.ProductVariantCombination{ProductID: pr.ID, ShopID: shop.ID, SKU: fmt.Sprintf("TS-%s-%s", col, sz),
						Price: p.price, Quantity: 30, Option1ID: &ci.ID, Option2ID: &si.ID, CombinationString: col + " / " + sz}
					must(tx.Omit("Product", "Option1", "Option2", "Option3").Create(&c).Error)
					combos = append(combos, c)
				}
			}
		}

		// Clients + orders spread over the last 30 days.
		names := []string{"Karim Benali", "Amina Haddad", "Yacine Messaoudi", "Sara Boudiaf", "Walid Cherif", "Lina Mansouri", "Omar Saidi", "Nadia Khelifi"}
		cities := []struct {
			wilaya int
			state  string
			city   string
		}{{16, "Alger", "Bab Ezzouar"}, {31, "Oran", "Es Senia"}, {25, "Constantine", "El Khroub"}, {9, "Blida", "Blida"}}
		statuses := []string{"En attente", "En attente", "Confirmé", "Confirmé", "Livré", "Livré", "Livré", "Annulé", "Retour", "Ne répond pas 1"}
		rng := rand.New(rand.NewSource(42))
		var clients []models.Client
		for i, n := range names {
			ct := cities[i%len(cities)]
			c := models.Client{ShopID: shop.ID, FullName: n, PhoneNumber: fmt.Sprintf("055%07d", 1000000+i*7919),
				State: ct.state, StateCode: fmt.Sprint(ct.wilaya), City: ct.city}
			must(tx.Create(&c).Error)
			clients = append(clients, c)
		}
		for i := 0; i < 40; i++ {
			cl := clients[rng.Intn(len(clients))]
			cb := combos[rng.Intn(len(combos))]
			qty := uint(1 + rng.Intn(3))
			ship := 400.0 + float64(rng.Intn(6))*100
			created := time.Now().Add(-time.Duration(rng.Intn(30*24)) * time.Hour)
			status := statuses[rng.Intn(len(statuses))]
			o := models.Order{ShopID: shop.ID, ClientID: cl.ID, ShippingMethod: []string{"doorstep", "stopdesk"}[rng.Intn(2)],
				ShippingPrice: ship, TotalPrice: cb.Price*float64(qty) + ship, Status: status, State: cl.State,
				IsShipped: status == "Livré" || status == "Retour"}
			o.CreatedAt, o.UpdatedAt = created, created
			must(tx.Omit("Client", "Items", "ShippedVia", "AssignedMember").Create(&o).Error)
			must(tx.Omit("Product", "ProductVariantCombination").Create(&models.OrderItem{OrderID: o.ID, ProductID: cb.ProductID,
				ProductVariantCombinationID: cb.ID, Quantity: qty, Price: cb.Price}).Error)
		}
		return nil
	}))

	log.Printf("seeded. shop slug=%q; logins owner/moderator/confirmation%s, password %q", shopSlug, seedDomain, password)
}

func must(err error) {
	if err != nil {
		log.Fatalf("seed: %v", err)
	}
}
