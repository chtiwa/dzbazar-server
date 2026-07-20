package services

import (
	"fmt"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/google/uuid"
)

func LandingPageCacheKeyByID(id uuid.UUID) string {
	return fmt.Sprintf("landing-page:id=%s", id.String())
}

func LandingPagesCacheKeyByShop(shopID uuid.UUID) string {
	return fmt.Sprintf("landing-pages:shop=%s", shopID.String())
}

// InvalidateLandingPageCaches must be called after any write that changes a
// landing page's cached fields (active, title, images, ...) — including
// automated writes like DecideExperimentIfReady's deactivation of losing
// sets, not just the admin CRUD handlers.
func InvalidateLandingPageCaches(shopID uuid.UUID, landingPageID uuid.UUID) {
	keys := []string{
		LandingPageCacheKeyByID(landingPageID),
		LandingPagesCacheKeyByShop(shopID),
	}
	if err := initializers.RClient.Del(initializers.Ctx, keys...).Err(); err != nil {
		fmt.Println("Failed to delete landing page cache keys:", err)
	}
}
