package initializers

import (
	"log"

	"github.com/joho/godotenv"
)

func LoadEnvVars() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error while loading the environemnt variables!")
	}
}
