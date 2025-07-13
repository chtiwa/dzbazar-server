package migrate

import (
	"log"

	"github.com/chtiwa/herbs-store-client/initializers"
	"github.com/chtiwa/herbs-store-client/models"
	"golang.org/x/crypto/bcrypt"
)

func SeedUsers() {
	var count int64
	initializers.DB.Model(&models.User{}).Count(&count)

	if count == 0 {
		hash1, _ := bcrypt.GenerateFromPassword([]byte("sigma"), 10)
		hash2, _ := bcrypt.GenerateFromPassword([]byte("sabrine29052003"), 10)
		users := []models.User{
			{Username: "chtiwa", Password: string(hash1), Role: "Admin"},
			{Username: "zinou", Password: string(hash2), Role: "Admin"},
		}

		result := initializers.DB.Create(&users)

		if result.Error != nil {
			log.Fatal("Failed to seed the users.")
		}
	} else {
		log.Println("Seeding skipped - users already exist.")
	}
}
