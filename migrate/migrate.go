package migrate

import (
	"fmt"
	"log"

	"github.com/chtiwa/herbs-store-client/initializers"
	"github.com/chtiwa/herbs-store-client/models"
)

func Migrate() {
	initializers.DB.Exec(`CREATE EXTENSION IF NOT EXISTS "uuid-ossp"`)

	// initializers.DB.Migrator().DropTable(&models.VariantItem{}, &models.Variant{}, &models.Product{}, &models.ProductImage{})
	err := initializers.DB.AutoMigrate(&models.BaseModel{}, &models.Order{}, &models.User{}, &models.Client{}, &models.Product{}, &models.ProductImage{}, &models.Variant{}, &models.VariantItem{}, &models.Category{})
	if err != nil {
		log.Fatal("Something went wrong while migrating")
	}

	SeedUsers()
	SeedCategories()
	fmt.Println("Migration was successful!")
}
