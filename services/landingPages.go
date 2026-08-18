package services

import (
	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
)

// ForceUnpublishLandingPage takes a live landing page down platform-side
// (super-admin shutting down a deceptive/abusive page across tenants). Sets
// the same Active=false a merchant reaches via UpdateLandingPageByShop — no
// separate unpublished state — so every other code path (IndexLandingPage)
// already treats it as off.
//
// Caller must also call InvalidateLandingPageCaches after this, same as
// UpdateLandingPageByShop does — Active is a cached field (see
// landingPageCache.go), so skipping it would let IndexLandingPage keep
// serving the page as live for up to the cache TTL.
func ForceUnpublishLandingPage(landingPage *models.LandingPage) error {
	landingPage.Active = false
	return initializers.DB.Model(landingPage).Select("Active").Updates(landingPage).Error
}
