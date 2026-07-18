package initializers

import (
	"fmt"
	"log"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

func ConnectToDB() {
	var err error
	dsn := os.Getenv("DB_URI")
	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})

	if err != nil {
		log.Fatal("Error while connecting to the database!")
	}

	sqlDB, err := DB.DB()
	if err != nil {
		log.Fatal("Error while accessing underlying sql.DB for pool tuning: ", err)
	}
	// ponytail: hardcoded for a single small Railway instance; bump if
	// scaling out to multiple instances against the same connection cap.
	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)
	sqlDB.SetConnMaxIdleTime(5 * time.Minute)

	fmt.Println("Connected to the database successfully!")
}
