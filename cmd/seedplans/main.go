// Command seedplans upserts the platform's 3 standard subscription plans
// (Basic, Standard, Premium) by Name — idempotent, safe to re-run to update
// caps/pricing on existing plans rather than duplicating them.
//
// Usage (from the server/ directory):
//
//	go run ./cmd/seedplans
package main

import (
	"fmt"
	"os"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/migrate"
	"github.com/chtiwa/dzbazar-server/models"
)

func init() {
	initializers.LoadEnvVars()
	initializers.ConnectToDB()
	migrate.Migrate()
}

func main() {
	plans := []models.Plan{
		{
			Name:                  "Basic",
			Price:                 2000,
			IsActive:              true,
			MaxShops:              1,
			MaxProducts:           30,
			MaxOrders:             500,
			MaxLandingPages:       3,
			MaxUsers:              2,
			MaxFacebookPixels:     1,
			MaxTikTokPixels:       1,
			HasConfirmationOrders: true,
			HasAbandonedOrders:    false,
			HasOrderTracking:      false,
			HasClientTracking:     false,
		},
		{
			Name:                  "Standard",
			Price:                 4000,
			IsActive:              true,
			MaxShops:              1,
			MaxProducts:           150,
			MaxOrders:             3000,
			MaxLandingPages:       15,
			MaxUsers:              5,
			MaxFacebookPixels:     3,
			MaxTikTokPixels:       3,
			HasConfirmationOrders: true,
			HasAbandonedOrders:    true,
			HasOrderTracking:      true,
			HasClientTracking:     true,
		},
		{
			Name:                  "Premium",
			Price:                 9000,
			IsActive:              true,
			MaxShops:              1,
			MaxProducts:           -1,
			MaxOrders:             -1,
			MaxLandingPages:       -1,
			MaxUsers:              -1,
			MaxFacebookPixels:     -1,
			MaxTikTokPixels:       -1,
			HasConfirmationOrders: true,
			HasAbandonedOrders:    true,
			HasOrderTracking:      true,
			HasClientTracking:     true,
		},
	}

	for _, p := range plans {
		var plan models.Plan
		if err := initializers.DB.Where("name = ?", p.Name).Assign(p).FirstOrCreate(&plan).Error; err != nil {
			fmt.Printf("failed to upsert plan %s: %v\n", p.Name, err)
			os.Exit(1)
		}
		fmt.Printf("plan %q upserted (id=%s, price=%.0f)\n", plan.Name, plan.ID, plan.Price)
	}
}
