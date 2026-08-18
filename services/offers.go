package services

import (
	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
)

// ForceDisableOffer archives an offer platform-side (super-admin shutting
// down an abusive/deceptive promotion across tenants). Same "archived"
// status a merchant reaches via ArchiveOffer — no separate disabled state —
// so every other code path (evaluation, listing) already treats it as off.
func ForceDisableOffer(offer *models.Offer) error {
	offer.Status = "archived"
	return initializers.DB.Model(offer).Select("Status").Updates(offer).Error
}
