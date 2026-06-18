package migrate

import (
	"log"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
)

func Migrate() {
	initializers.DB.Exec(`CREATE EXTENSION IF NOT EXISTS "uuid-ossp"`)
	initializers.DB.Exec(`CREATE EXTENSION IF NOT EXISTS "pg_trgm";`)

	// 1. Temporarily disable Foreign Key creation on the GORM instance
	initializers.DB.Config.DisableForeignKeyConstraintWhenMigrating = true

	log.Println("🔄 Phase 1: Creating base tables without constraints...")
	err := initializers.DB.AutoMigrate(
		&models.User{},
		&models.Shop{},
	)
	if err != nil {
		log.Fatalf("Phase 1 migration failed: %v", err)
	}

	// 2. Re-enable Foreign Key creation so all subsequent tables link correctly
	initializers.DB.Config.DisableForeignKeyConstraintWhenMigrating = false

	log.Println("🔄 Phase 2: Building dependent tables and establishing relationships...")
	err = initializers.DB.AutoMigrate(
		&models.User{}, // Passing them again forces GORM to append the skipped foreign keys
		&models.Shop{},
		&models.ShopLogoImage{},
		&models.ShopMember{},
		&models.DeliveryRate{},
		&models.Client{},
		&models.Pixel{},
		&models.Product{},
		&models.ProductImage{},
		&models.LandingPage{},
		&models.LandingPageImage{},
		&models.Variant{},
		&models.VariantItem{},
		&models.ProductVariantCombination{},
		&models.Order{},
		&models.OrderItem{},
		&models.AvailableDeliveryCompany{},
		&models.AvailableDeliveryCompanyImage{},
		&models.DeliveryCompany{},
		&models.Plan{},
		&models.ShopSubscription{},
		&models.AbandonedLead{},
		&models.AuditLog{},
		&models.FeatureFlag{},
		&models.GlobalSetting{},
		&models.SupportTicket{},
		&models.SupportTicketMessage{},
	)
	if err != nil {
		log.Fatalf("Phase 2 migration failed: %v", err)
	}

	// Drop legacy NOT NULL constraints on columns removed from the DeliveryCompany struct.
	// AutoMigrate never drops columns, so old constraints must be patched manually.
	// DB.Exec errors are silently ignored — safe if column is already nullable or absent.
	initializers.DB.Exec(`ALTER TABLE delivery_companies ALTER COLUMN name DROP NOT NULL`)
	initializers.DB.Exec(`ALTER TABLE delivery_companies ALTER COLUMN url DROP NOT NULL`)

	// Enforce "only one super_admin" at the DB level — not just app convention —
	// so it holds no matter what code path ever writes platform_role.
	initializers.DB.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_one_super_admin
		ON users (platform_role)
		WHERE platform_role = 'super_admin' AND deleted_at IS NULL
	`)

	log.Println("🚀 Database schema migrated perfectly with all relations intact!")
}
