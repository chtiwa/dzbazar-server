package migrate

import (
	"fmt"
	"log"

	"github.com/chtiwa/lk-parfumo-server/initializers"
	"github.com/chtiwa/lk-parfumo-server/models"
)

func Migrate() {
	initializers.DB.Exec(`CREATE EXTENSION IF NOT EXISTS "uuid-ossp"`)
	initializers.DB.Exec(`CREATE EXTENSION IF NOT EXISTS "pg_trgm";`)

	err := initializers.DB.AutoMigrate(&models.BaseModel{}, &models.Order{}, &models.User{}, &models.Client{}, &models.Product{}, &models.ProductImage{}, &models.Variant{}, &models.VariantItem{}, &models.Tag{}, &models.LandingPage{}, &models.LandingPageImage{})
	if err != nil {
		log.Fatal("Something went wrong while migrating")
	}

	status := initializers.RClient.Set(initializers.Ctx, "promo:pack3:remaining", 87, 0)

	fmt.Println(status)

	SeedUsers()
	SeedTags()
	fmt.Println("Migration was successful!")
}
