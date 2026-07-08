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
		&models.ShopRole{},
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
		&models.Coupon{},
		&models.Variant{},
		&models.VariantItem{},
		&models.ProductVariantCombination{},
		&models.AvailableDeliveryCompany{},
		&models.AvailableDeliveryCompanyImage{},
		&models.Order{},
		&models.OrderItem{},
		&models.FlaggedClient{},
		&models.DeliveryCompany{},
		&models.Plan{},
		&models.ShopSubscription{},
		&models.AbandonedLead{},
		&models.AuditLog{},
		&models.FeatureFlag{},
		&models.GlobalSetting{},
		&models.SupportTicket{},
		&models.SupportTicketMessage{},
		&models.Offer{},
		&models.OfferPageOverride{},
		&models.OfferEvent{},
	)
	if err != nil {
		log.Fatalf("Phase 2 migration failed: %v", err)
	}

	// Drop legacy NOT NULL constraints on columns removed from the DeliveryCompany struct.
	// AutoMigrate never drops columns, so old constraints must be patched manually.
	// DB.Exec errors are silently ignored — safe if column is already nullable or absent.
	initializers.DB.Exec(`ALTER TABLE delivery_companies ALTER COLUMN name DROP NOT NULL`)
	initializers.DB.Exec(`ALTER TABLE delivery_companies ALTER COLUMN url DROP NOT NULL`)

	// FlaggedClient was reshaped from a Facebook-only {ShopID, FBp} row to a
	// platform-agnostic {ShopID, Platform, ClientID} row. Drop the orphaned,
	// still-NOT-NULL f_bp column left behind by the earlier AutoMigrate run.
	initializers.DB.Exec(`ALTER TABLE flagged_clients DROP COLUMN IF EXISTS f_bp`)

	// orders.client_id -> clients.id was originally created as ON DELETE SET
	// NULL, which is impossible since client_id is NOT NULL — deleting a
	// client with orders violated that not-null constraint instead of
	// deleting the orders. AutoMigrate never edits an existing FK's ON DELETE
	// action, so it's replaced here with CASCADE. Looked up by table pair
	// rather than a hardcoded constraint name, since GORM's auto-generated
	// name isn't guaranteed.
	initializers.DB.Exec(`
		DO $$
		DECLARE
			existing_constraint text;
		BEGIN
			SELECT conname INTO existing_constraint
			FROM pg_constraint
			WHERE conrelid = 'orders'::regclass
				AND confrelid = 'clients'::regclass
				AND contype = 'f';

			IF existing_constraint IS NOT NULL THEN
				EXECUTE format('ALTER TABLE orders DROP CONSTRAINT %I', existing_constraint);
			END IF;

			ALTER TABLE orders
				ADD CONSTRAINT fk_orders_client
				FOREIGN KEY (client_id) REFERENCES clients(id)
				ON DELETE CASCADE;
		END $$;
	`)

	// Enforce "only one super_admin" at the DB level — not just app convention —
	// so it holds no matter what code path ever writes platform_role.
	initializers.DB.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_one_super_admin
		ON users (platform_role)
		WHERE platform_role = 'super_admin' AND deleted_at IS NULL
	`)

	// Seed shop roles (idempotent)
	initializers.DB.Exec(`INSERT INTO shop_roles (name) VALUES ('owner'),('moderator'),('confirmation') ON CONFLICT (name) DO NOTHING`)

	// Backfill legacy role names → new role names (idempotent — no-op once migrated)
	initializers.DB.Exec(`UPDATE shop_members SET role='owner'        WHERE role='Owner'`)
	initializers.DB.Exec(`UPDATE shop_members SET role='moderator'    WHERE role='Staff'`)
	initializers.DB.Exec(`UPDATE shop_members SET role='confirmation' WHERE role='Logistics'`)
	initializers.DB.Exec(`UPDATE users        SET role='owner'        WHERE role='Owner'`)
	initializers.DB.Exec(`UPDATE users        SET role='moderator'    WHERE role='Staff'`)
	initializers.DB.Exec(`UPDATE users        SET role='confirmation' WHERE role='Logistics'`)

	log.Println("🚀 Database schema migrated perfectly with all relations intact!")
}
