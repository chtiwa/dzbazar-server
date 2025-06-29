package migrate

import (
	"fmt"
	"log"

	"github.com/chtiwa/herbs-store-client/initializers"
	"github.com/chtiwa/herbs-store-client/models"
)

func Migrate() {
	initializers.DB.Exec(`CREATE EXTENSION IF NOT EXISTS "uuid-ossp"`)

	err := initializers.DB.AutoMigrate(&models.BaseModel{}, &models.Order{}, &models.User{}, &models.Client{}, &models.Product{}, &models.ProductImage{}, &models.Variant{}, &models.VariantItem{}, &models.Category{})
	if err != nil {
		log.Fatal("Something went wrong while migrating")
	}

	// 	// Add the ALTER TABLE statement here to update price and quantity types:
	// 	if err := initializers.DB.Exec(`
	// 	-- Drop existing defaults on price and quantity
	// 	ALTER TABLE variant_items
	// 	ALTER COLUMN price DROP DEFAULT,
	// 	ALTER COLUMN quantity DROP DEFAULT;

	// 	-- Change types from text to integer
	// 	ALTER TABLE variant_items
	// 	ALTER COLUMN price TYPE INTEGER USING price::integer,
	// 	ALTER COLUMN quantity TYPE INTEGER USING quantity::integer;

	// 	-- (Optional) Re-add integer defaults
	// 	ALTER TABLE variant_items
	// 	ALTER COLUMN price SET DEFAULT 0,
	// 	ALTER COLUMN quantity SET DEFAULT 0;
	// `).Error; err != nil {
	// 		log.Fatalf("Failed to alter column types on variant_items: %v", err)
	// 	}

	SeedUsers()
	SeedCategories()
	fmt.Println("Migration was successful!")
}
