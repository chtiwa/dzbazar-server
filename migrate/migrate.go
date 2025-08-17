package migrate

import (
	"fmt"
	"log"

	"github.com/chtiwa/lk-parfumo-server/initializers"
	"github.com/chtiwa/lk-parfumo-server/models"
)

func Migrate() {
	initializers.DB.Exec(`CREATE EXTENSION IF NOT EXISTS "uuid-ossp"`)

	err := initializers.DB.AutoMigrate(&models.BaseModel{}, &models.Order{}, &models.User{}, &models.Client{}, &models.Product{}, &models.ProductImage{}, &models.Variant{}, &models.VariantItem{}, &models.Tag{})
	if err != nil {
		log.Fatal("Something went wrong while migrating")
	}

	SeedUsers()
	SeedTags()
	fmt.Println("Migration was successful!")
}
