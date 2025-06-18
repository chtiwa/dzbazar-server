package migrate

import (
	"fmt"

	"github.com/chtiwa/herbs-store-client/initializers"
	"github.com/chtiwa/herbs-store-client/models"
)

func Migrate() {
	initializers.DB.Exec(`CREATE EXTENSION IF NOT EXISTS "uuid-ossp"`)
	initializers.DB.AutoMigrate(&models.Order{}, &models.User{}, &models.Client{})
	SeedUsers()
	fmt.Println("Migration was successful!")
}
