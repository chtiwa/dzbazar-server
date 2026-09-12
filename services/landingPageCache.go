package services

import (
	"fmt"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/google/uuid"
)

// LandingPageCacheKeyByID is for the authenticated merchant-dashboard shape
// (models.LandingPage, full data). Kept separate from the public storefront
// key below — they used to share one key while caching two different JSON
// shapes, so whichever handler wrote last silently corrupted the other's
// read (json.Unmarshal doesn't error on a shape mismatch, it just drops/zeros
// fields), which is why an edit sometimes needed to be saved twice to "stick".
func LandingPageCacheKeyByID(id uuid.UUID) string {
	return fmt.Sprintf("landing-page:id=%s", id.String())
}

// PublicLandingPageCacheKeyByID is for the unauthenticated storefront shape
// (dto.PublicLandingPageResponse), used by IndexLandingPage only.
func PublicLandingPageCacheKeyByID(id uuid.UUID) string {
	return fmt.Sprintf("landing-page:public:id=%s", id.String())
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
		PublicLandingPageCacheKeyByID(landingPageID),
		LandingPagesCacheKeyByShop(shopID),
	}
	if err := initializers.RClient.Del(initializers.Ctx, keys...).Err(); err != nil {
		fmt.Println("Failed to delete landing page cache keys:", err)
	}
}
