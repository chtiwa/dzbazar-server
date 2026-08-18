package services

import (
	"time"

	"github.com/chtiwa/dzbazar-server/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// FlaggedClientWithShop is a FlaggedClient row plus the shop name/slug a
// cross-tenant reviewer needs to make sense of it — same join pattern as
// ClientWithShop / requestWithShop in the superadmin controllers.
type FlaggedClientWithShop struct {
	models.FlaggedClient
	ShopName string `json:"shopName"`
	ShopSlug string `json:"shopSlug"`
}

// ListFlaggedClientsFilter narrows the cross-tenant fraud review list.
// Resolved is a tri-state: nil means "both", true/false narrows to one.
type ListFlaggedClientsFilter struct {
	ShopID   string
	Platform string
	Resolved *bool
}

// ListFlaggedClients is a read-only cross-tenant view of flagged_clients,
// filterable by shop/platform/resolved-state — same batching principle as
// ListClients: one query for the page, one grouped shop lookup instead of
// N+1.
func ListFlaggedClients(db *gorm.DB, filter ListFlaggedClientsFilter, order string, page, perPage int) ([]FlaggedClientWithShop, int64, error) {
	q := db.Model(&models.FlaggedClient{})

	if filter.ShopID != "" {
		q = q.Where("shop_id = ?", filter.ShopID)
	}
	if filter.Platform != "" {
		q = q.Where("platform = ?", filter.Platform)
	}
	if filter.Resolved != nil {
		if *filter.Resolved {
			q = q.Where("resolved_at IS NOT NULL")
		} else {
			q = q.Where("resolved_at IS NULL")
		}
	}

	var totalRows int64
	if err := q.Count(&totalRows).Error; err != nil {
		return nil, 0, err
	}

	var flags []models.FlaggedClient
	if err := q.Order(order).Offset((page - 1) * perPage).Limit(perPage).Find(&flags).Error; err != nil {
		return nil, 0, err
	}

	shopIDs := make([]string, 0, len(flags))
	seen := map[string]bool{}
	for _, f := range flags {
		idStr := f.ShopID.String()
		if !seen[idStr] {
			seen[idStr] = true
			shopIDs = append(shopIDs, idStr)
		}
	}

	shopByID := map[string]models.Shop{}
	if len(shopIDs) > 0 {
		var shops []models.Shop
		db.Select("id", "name", "slug").Where("id IN ?", shopIDs).Find(&shops)
		for _, s := range shops {
			shopByID[s.ID.String()] = s
		}
	}

	result := make([]FlaggedClientWithShop, 0, len(flags))
	for _, f := range flags {
		shop := shopByID[f.ShopID.String()]
		result = append(result, FlaggedClientWithShop{FlaggedClient: f, ShopName: shop.Name, ShopSlug: shop.Slug})
	}

	return result, totalRows, nil
}

// ResolveFlaggedClient marks a flag dismissed by the acting operator. It does
// NOT delete the row — the flag stays as history — but a resolved flag no
// longer blocks that client's future orders (ordersController.go's
// fbp/ttp short-circuit filters on resolved_at IS NULL).
func ResolveFlaggedClient(db *gorm.DB, id uuid.UUID, actorID uuid.UUID) (models.FlaggedClient, error) {
	var flag models.FlaggedClient
	if err := db.First(&flag, "id = ?", id).Error; err != nil {
		return flag, err
	}

	now := time.Now()
	if err := db.Model(&flag).Updates(map[string]any{
		"resolved_at": now,
		"resolved_by": actorID,
	}).Error; err != nil {
		return flag, err
	}

	flag.ResolvedAt = &now
	flag.ResolvedBy = &actorID
	return flag, nil
}
