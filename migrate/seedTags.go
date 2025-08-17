package migrate

import (
	"log"

	"github.com/chtiwa/lk-parfumo-server/initializers"
	"github.com/chtiwa/lk-parfumo-server/models"
)

func SeedTags() {
	var count int64
	initializers.DB.Model(&models.Tag{}).Count(&count)

	if count == 0 {
		tags := []models.Tag{
			{Name: "homme"},
			{Name: "femme"},
			{Name: "Unisex"},
			{Name: "Collection"},
		}

		result := initializers.DB.Create(&tags)
		if result.Error != nil {
			log.Fatal("Failed to seed the categories")
		}
	} else {
		log.Println("Seeding skipped - tags already exist.")
	}
}
