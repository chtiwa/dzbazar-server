package initializers

import (
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
)

func LoadEnvVars() {
	err := godotenv.Load()
	if os.Getenv("RAILWAY_ENVIRONMENT") == "" {
		if err != nil {
			fmt.Println(err.Error())
			log.Fatal("Error while loading the environment variables!")
		}
	}
}
