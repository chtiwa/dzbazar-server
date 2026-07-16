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
		&models.PermissionAction{},
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
		&models.ShopMemberPermission{},
		&models.RoleActionDefault{},
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
		&models.ShopVisit{},
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

	// offer_type was a merchant-facing label redundant with (action, placement) and
	// never read by the eval pipeline — dropped in favor of deriving the label client-side.
	initializers.DB.Exec(`ALTER TABLE offers DROP COLUMN IF EXISTS offer_type`)

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

	// Seed/extend grantable permission actions (idempotent) — grows one row per
	// action gated via RequireShopPermission. DO UPDATE backfills resource/label
	// on pre-existing rows as the vocabulary grows.
	initializers.DB.Exec(`
		INSERT INTO permission_actions (name, resource, label) VALUES
			('orders.view','orders','View orders'),
			('orders.edit','orders','Edit orders & status'),
			('orders.export','orders','Export to Excel'),
			('orders.status_history','orders','View status history'),
			('orders.delete','orders','Delete orders'),
			('orders.ban_client','orders','Ban client'),
			('orders.ship','orders','Ship to delivery company'),
			('orders.track','orders','Track shipments'),
			('users.view','users','View members'),
			('users.create','users','Add members'),
			('users.edit','users','Edit members'),
			('users.delete','users','Delete members'),
			('products.create','products','Add products'),
			('products.edit','products','Edit products'),
			('products.delete','products','Delete products'),
			('landing_pages.create','landing_pages','Add landing pages'),
			('landing_pages.edit','landing_pages','Edit landing pages'),
			('landing_pages.delete','landing_pages','Delete landing pages'),
			('coupons.create','coupons','Add coupons'),
			('coupons.edit','coupons','Edit coupons'),
			('coupons.delete','coupons','Delete coupons'),
			('clients.edit','clients','Edit clients'),
			('clients.delete','clients','Delete clients'),
			('pixels.view','pixels','View pixels'),
			('pixels.edit','pixels','Edit pixels'),
			('pixels.delete','pixels','Delete pixels'),
			('delivery_rates.edit','delivery_rates','Edit delivery rates'),
			('delivery_companies.view','delivery_companies','View courier integrations'),
			('delivery_companies.edit','delivery_companies','Edit courier integrations'),
			('subscription.view','subscription','View subscription'),
			('subscription.edit','subscription','Edit subscription'),
			('offers.create','offers','Add offers'),
			('offers.edit','offers','Edit offers'),
			('offers.archive','offers','Publish & archive offers'),
			('offers.delete','offers','Delete offers'),
			('settings.view','settings','View shop settings'),
			('settings.edit','settings','Edit shop settings'),
			('dashboard.view','dashboard','View dashboard')
		ON CONFLICT (name) DO UPDATE SET resource = EXCLUDED.resource, label = EXCLUDED.label
	`)

	// Rename the original bare 'delete' action (users-only at the time) to
	// 'users.delete' now that action names are resource-namespaced (idempotent).
	initializers.DB.Exec(`UPDATE shop_member_permissions SET action = 'users.delete' WHERE action = 'delete'`)
	initializers.DB.Exec(`DELETE FROM permission_actions WHERE name = 'delete'`)

	// Trim the action list down to the client's curated set: drop 'view' as a
	// checklist item on resources where it's not meant to be overridable
	// (reverted to a fixed role check in the route), and merge two pairs of
	// actions into one now that their routes share a single permission.
	// ponytail: these tables are days old with ~no real overrides set yet, so a
	// plain rename (not a conflict-safe merge) is fine — re-grant by hand if a
	// merged pair ever collides on a live member.
	initializers.DB.Exec(`UPDATE shop_member_permissions SET action = 'offers.archive' WHERE action = 'offers.publish'`)
	initializers.DB.Exec(`UPDATE shop_member_permissions SET action = 'delivery_companies.edit' WHERE action = 'delivery_companies.connect'`)
	initializers.DB.Exec(`UPDATE shop_member_permissions SET action = 'subscription.edit' WHERE action = 'subscription.subscribe'`)
	initializers.DB.Exec(`
		DELETE FROM shop_member_permissions WHERE action IN (
			'products.view','landing_pages.view','coupons.view','offers.view',
			'clients.view','clients.create','clients.import','pixels.create',
			'delivery_rates.view','delivery_companies.disconnect','subscription.cancel'
		)
	`)
	initializers.DB.Exec(`
		DELETE FROM permission_actions WHERE name IN (
			'products.view','landing_pages.view','coupons.view','offers.view','offers.publish',
			'clients.view','clients.create','clients.import','pixels.create',
			'delivery_rates.view','delivery_companies.connect','delivery_companies.disconnect',
			'subscription.subscribe','subscription.cancel'
		)
	`)

	// Seed role defaults (idempotent; DO NOTHING so manual tuning is never
	// clobbered). Values are behavior-preserving with the gates each action
	// replaces — migrating from hardcoded switch to DB changes no one's access.
	initializers.DB.Exec(`
		INSERT INTO role_action_defaults (role, action, allow) VALUES
			('owner','orders.view',true),('moderator','orders.view',true),('confirmation','orders.view',true),
			('owner','orders.edit',true),('moderator','orders.edit',true),('confirmation','orders.edit',true),
			('owner','orders.export',true),('moderator','orders.export',true),('confirmation','orders.export',true),
			('owner','orders.status_history',true),('moderator','orders.status_history',false),('confirmation','orders.status_history',false),
			('owner','orders.delete',true),('moderator','orders.delete',false),('confirmation','orders.delete',false),
			('owner','orders.ban_client',true),('moderator','orders.ban_client',false),('confirmation','orders.ban_client',false),
			('owner','orders.ship',true),('moderator','orders.ship',true),('confirmation','orders.ship',true),
			('owner','orders.track',true),('moderator','orders.track',true),('confirmation','orders.track',true),
			('owner','users.view',true),('moderator','users.view',true),('confirmation','users.view',false),
			('owner','users.create',true),('moderator','users.create',true),('confirmation','users.create',false),
			('owner','users.edit',true),('moderator','users.edit',true),('confirmation','users.edit',false),
			('owner','users.delete',true),('moderator','users.delete',false),('confirmation','users.delete',false),
			('owner','products.create',true),('moderator','products.create',true),('confirmation','products.create',false),
			('owner','products.edit',true),('moderator','products.edit',true),('confirmation','products.edit',false),
			('owner','products.delete',true),('moderator','products.delete',false),('confirmation','products.delete',false),
			('owner','landing_pages.create',true),('moderator','landing_pages.create',true),('confirmation','landing_pages.create',false),
			('owner','landing_pages.edit',true),('moderator','landing_pages.edit',true),('confirmation','landing_pages.edit',false),
			('owner','landing_pages.delete',true),('moderator','landing_pages.delete',false),('confirmation','landing_pages.delete',false),
			('owner','coupons.create',true),('moderator','coupons.create',true),('confirmation','coupons.create',false),
			('owner','coupons.edit',true),('moderator','coupons.edit',true),('confirmation','coupons.edit',false),
			('owner','coupons.delete',true),('moderator','coupons.delete',false),('confirmation','coupons.delete',false),
			('owner','clients.edit',true),('moderator','clients.edit',true),('confirmation','clients.edit',true),
			('owner','clients.delete',true),('moderator','clients.delete',false),('confirmation','clients.delete',false),
			('owner','pixels.view',true),('moderator','pixels.view',true),('confirmation','pixels.view',false),
			('owner','pixels.edit',true),('moderator','pixels.edit',true),('confirmation','pixels.edit',false),
			('owner','pixels.delete',true),('moderator','pixels.delete',false),('confirmation','pixels.delete',false),
			('owner','delivery_rates.edit',true),('moderator','delivery_rates.edit',true),('confirmation','delivery_rates.edit',false),
			('owner','delivery_companies.view',true),('moderator','delivery_companies.view',true),('confirmation','delivery_companies.view',true),
			('owner','delivery_companies.edit',true),('moderator','delivery_companies.edit',true),('confirmation','delivery_companies.edit',false),
			('owner','subscription.view',true),('moderator','subscription.view',true),('confirmation','subscription.view',true),
			('owner','subscription.edit',true),('moderator','subscription.edit',false),('confirmation','subscription.edit',false),
			('owner','offers.create',true),('moderator','offers.create',true),('confirmation','offers.create',false),
			('owner','offers.edit',true),('moderator','offers.edit',true),('confirmation','offers.edit',false),
			('owner','offers.archive',true),('moderator','offers.archive',true),('confirmation','offers.archive',false),
			('owner','offers.delete',true),('moderator','offers.delete',false),('confirmation','offers.delete',false),
			('owner','settings.view',true),('moderator','settings.view',true),('confirmation','settings.view',false),
			('owner','settings.edit',true),('moderator','settings.edit',true),('confirmation','settings.edit',false),
			('owner','dashboard.view',true),('moderator','dashboard.view',true),('confirmation','dashboard.view',true)
		ON CONFLICT (role, action) DO NOTHING
	`)

	// Backfill legacy role names → new role names (idempotent — no-op once migrated)
	initializers.DB.Exec(`UPDATE shop_members SET role='owner'        WHERE role='Owner'`)
	initializers.DB.Exec(`UPDATE shop_members SET role='moderator'    WHERE role='Staff'`)
	initializers.DB.Exec(`UPDATE shop_members SET role='confirmation' WHERE role='Logistics'`)
	initializers.DB.Exec(`UPDATE users        SET role='owner'        WHERE role='Owner'`)
	initializers.DB.Exec(`UPDATE users        SET role='moderator'    WHERE role='Staff'`)
	initializers.DB.Exec(`UPDATE users        SET role='confirmation' WHERE role='Logistics'`)

	log.Println("🚀 Database schema migrated perfectly with all relations intact!")
}
