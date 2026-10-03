package services

import (
	"github.com/chtiwa/dzbazar-server/dto"
	"github.com/chtiwa/dzbazar-server/initializers"
)

// TrackRecordsByPhones returns platform-wide (all shops) Livré/Retour counts
// per phone. Same terminal statuses as dashboardController's resolved*,
// minus is_shipped (manual-shipping shops never set it).
// ponytail: exact phone match — CreateOrderByShopID enforces ^0[567]\d{8}$, so stored phones are canonical; normalize here only if client CRUD/Excel imports diverge.
func TrackRecordsByPhones(phones []string) (map[string]dto.ClientTrackRecord, error) {
	out := map[string]dto.ClientTrackRecord{}
	if len(phones) == 0 {
		return out, nil
	}
	var rows []struct {
		Phone     string
		Delivered int64
		Returned  int64
	}
	err := initializers.DB.Raw(`
		SELECT c.phone_number AS phone,
			COUNT(*) FILTER (WHERE o.status = 'Livré')  AS delivered,
			COUNT(*) FILTER (WHERE o.status = 'Retour') AS returned
		FROM orders o
		JOIN clients c ON c.id = o.client_id
		WHERE c.phone_number IN ?
			AND o.status IN ('Livré', 'Retour')
			AND o.deleted_at IS NULL AND c.deleted_at IS NULL
		GROUP BY c.phone_number`, phones).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.Phone] = dto.ClientTrackRecord{Delivered: r.Delivered, Returned: r.Returned}
	}
	return out, nil
}
