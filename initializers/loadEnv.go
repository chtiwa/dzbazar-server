package initializers

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

func LoadEnvVars() {
	err := godotenv.Load()
	if os.Getenv("RAILWAY_ENVIRONMENT") == "" {
		if err != nil {
			log.Fatal("Error while loading the environment variables!")
		}
	}
}
