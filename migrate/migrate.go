package migrate

import (
	"fmt"
	"log"

	"github.com/chtiwa/herbs-store-client/initializers"
	"github.com/chtiwa/herbs-store-client/models"
)

func Migrate() {
	initializers.DB.Exec(`CREATE EXTENSION IF NOT EXISTS "uuid-ossp"`)
	// if err := fixProductImagesConstraint(); err != nil {
	// 	log.Fatalf("failed to fix constraint: %v", err)
	// }
	// initializers.DB.Migrator().DropTable(&models.Product{}, &models.ProductImage{}, &models.Variant{}, models.VariantItem{})
	err := initializers.DB.AutoMigrate(&models.BaseModel{}, &models.Order{}, &models.User{}, &models.Client{}, &models.Product{}, &models.ProductImage{}, &models.Variant{}, &models.VariantItem{}, &models.Category{})
	if err != nil {
		log.Fatal("Something went wrong while migrating")
	}
	SeedUsers()
	SeedCategories()
	fmt.Println("Migration was successful!")
}

// func fixProductImagesConstraint() error {
// 	// Drop existing constraint if it exists
// 	result := initializers.DB.Exec(`
// 	ALTER TABLE variants
// 		DROP CONSTRAINT IF EXISTS fk_variants_product;
// 	`)
// 	if result.Error != nil {
// 		return result.Error
// 	}

// 	// Add new constraint with ON DELETE CASCADE
// 	result = initializers.DB.Exec(`
// 		ALTER TABLE variants
// 		ADD CONSTRAINT fk_variants_product
// 		FOREIGN KEY (product_id) REFERENCES products(id)
// 		ON DELETE CASCADE;
// 	`)
// 	return result.Error
// }
