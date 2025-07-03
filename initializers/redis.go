package initializers

import (
	"context"
	"log"
	"net/url"
	"os"

	"github.com/redis/go-redis/v9"
)

var RClient *redis.Client
var Ctx = context.Background()

func InitRedis() {
	redisURI := os.Getenv("REDIS_URI")
	if redisURI == "" {
		log.Fatal("REDIS_URI environment variable is not set")
	}

	// Parse redis:// URI to extract host and password
	u, err := url.Parse(redisURI)
	if err != nil {
		log.Fatalf("failed to parse REDIS_URI: %v", err)
	}

	password, _ := u.User.Password() // optional password from URI
	addr := u.Host                   // host:port part

	RClient = redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       0,
	})

	if err := RClient.Ping(Ctx).Err(); err != nil {
		log.Fatalf("error while trying to connect to redis: %v", err)
	}
}
