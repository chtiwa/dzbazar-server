package main

import (
	"log"

	"github.com/chtiwa/dzbazar-server/models"
	"gorm.io/gorm"
)

// topUp adds the extras to an already-seeded demo shop. Additive only: skips if WELCOME10 exists.
func topUp(db *gorm.DB) {
	var shop models.Shop
	must(db.Where("slug = ?", shopSlug).First(&shop).Error)
	var n int64
	db.Model(&models.Coupon{}).Where("shop_id = ? AND code = ?", shop.ID, "WELCOME10").Count(&n)
	if n > 0 {
		log.Println("already seeded + extras present, nothing to do")
		return
	}
	var owner models.User
	must(db.Where("email = ?", "owner"+seedDomain).First(&owner).Error)
	members := map[string]*models.ShopMember{}
	for _, r := range []string{"owner", "moderator", "confirmation"} {
		var m models.ShopMember
		must(db.Where("shop_id = ? AND role = ?", shop.ID, r).First(&m).Error)
		members[r] = &m
	}
	var products []models.Product
	must(db.Where("shop_id = ?", shop.ID).Order("created_at").Find(&products).Error)
	var combos []models.ProductVariantCombination
	must(db.Where("shop_id = ?", shop.ID).Order("created_at").Find(&combos).Error)
	var clients []models.Client
	must(db.Where("shop_id = ? AND phone_number LIKE '055%'", shop.ID).Order("created_at").Find(&clients).Error)

	var sa int64
	db.Model(&models.User{}).Where("email = ?", "superadmin"+seedDomain).Count(&sa)
	must(db.Transaction(func(tx *gorm.DB) error {
		if sa == 0 {
			u := models.User{FirstName: "Demo", LastName: "Super", PhoneNumber: "0550000000", Email: "superadmin" + seedDomain,
				Password: owner.Password, Role: "owner", IsVerified: true, PlatformRole: "super_admin"}
			must(tx.Omit("Memberships").Create(&u).Error)
		}
		seedExtras(tx, shop, owner, members, products, combos, clients)
		return nil
	}))
	log.Println("extras added to", shopSlug)
}
