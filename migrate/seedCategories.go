package migrate

import (
	"log"

	"github.com/chtiwa/herbs-store-client/initializers"
	"github.com/chtiwa/herbs-store-client/models"
)

func SeedCategories() {
	var count int64
	initializers.DB.Model(&models.Category{}).Count(&count)

	if count == 0 {

		categories := []models.Category{
			{Title: "homme"},
			{Title: "femme"},
			{Title: "unisex"},
		}

		result := initializers.DB.Create(&categories)
		if result.Error != nil {
			log.Fatal("Failed to seed the categories")
		}
	} else {
		log.Println("Seeding skipped - categories already exist.")
	}
}
