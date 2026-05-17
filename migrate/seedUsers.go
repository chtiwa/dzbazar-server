package migrate

import (
	"log"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"golang.org/x/crypto/bcrypt"
)

func SeedUsers() {
	var count int64
	initializers.DB.Model(&models.User{}).Count(&count)

	if count == 0 {
		hash1, _ := bcrypt.GenerateFromPassword([]byte("sigma"), 10)
		hash2, _ := bcrypt.GenerateFromPassword([]byte("zinou2003"), 10)
		users := []models.User{
			{FirstName: "chtiwa", Password: string(hash1), Role: "Admin"},
			{LastName: "zinou", Password: string(hash2), Role: "Admin"},
		}

		result := initializers.DB.Create(&users)

		if result.Error != nil {
			log.Fatal("Failed to seed the users.")
		}
	} else {
		log.Println("Seeding skipped - users already exist.")
	}
}
