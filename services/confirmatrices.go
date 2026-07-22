package services

import (
	"time"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// EligibleConfirmatrices returns the confirmation-role shop members scoped to
// at least one of productIDs (union, not intersection — an order touching
// several products is eligible for anyone covering any one of them). A
// member with no confirmatrice_products rows is never eligible: scope is an
// allow-list, not a wildcard. A member with active=false (out sick, on
// leave) is skipped even if otherwise scoped — she stays in the round-robin
// pool the moment she's flipped back on, nothing else to reconfigure.
func EligibleConfirmatrices(tx *gorm.DB, shopID uuid.UUID, productIDs []uuid.UUID) ([]models.ShopMember, error) {
	var members []models.ShopMember
	if len(productIDs) == 0 {
		return members, nil
	}
	err := tx.
		Distinct("shop_members.*").
		Joins("JOIN confirmatrice_products cp ON cp.shop_member_id = shop_members.id").
		Where("shop_members.shop_id = ? AND shop_members.role = ? AND shop_members.active = ? AND cp.product_id IN ?", shopID, "confirmation", true, productIDs).
		Order("shop_members.id ASC").
		Find(&members).Error
	return members, err
}

// AutoAssignOrder picks a confirmatrice via strict round-robin among those
// eligible for the order's products and stamps the assignment. Best-effort:
// no eligible confirmatrice just leaves the order unassigned for an admin to
// assign manually — it must never fail the checkout it's called from.
//
// ponytail: one shop-level cursor gives rough fairness across a changing
// eligible set; upgrade to per-product cursors only if rotation skew is
// actually reported.
func AutoAssignOrder(tx *gorm.DB, shopID uuid.UUID, order *models.Order) error {
	productIDs := make([]uuid.UUID, 0, len(order.Items))
	seen := make(map[uuid.UUID]bool, len(order.Items))
	for _, item := range order.Items {
		if !seen[item.ProductID] {
			seen[item.ProductID] = true
			productIDs = append(productIDs, item.ProductID)
		}
	}

	eligible, err := EligibleConfirmatrices(tx, shopID, productIDs)
	if err != nil || len(eligible) == 0 {
		return err
	}

	var cursor int64
	if err := tx.Raw(
		"UPDATE shops SET confirmatrice_cursor = confirmatrice_cursor + 1 WHERE id = ? RETURNING confirmatrice_cursor",
		shopID,
	).Scan(&cursor).Error; err != nil {
		return err
	}

	chosen := eligible[(cursor-1)%int64(len(eligible))]
	now := time.Now()
	order.AssignedMemberID = &chosen.ID
	order.AssignedAt = &now

	return tx.Model(&models.Order{}).
		Where("id = ?", order.ID).
		Updates(map[string]any{"assigned_member_id": chosen.ID, "assigned_at": now}).Error
}

// AssignOrder is the manual assign/reassign/clear path. memberID nil clears
// the assignment. A non-nil memberID must resolve to a confirmation-role
// member of the same shop, or the order.
func AssignOrder(shopID, orderID uuid.UUID, memberID *uuid.UUID) error {
	updates := map[string]any{"assigned_member_id": nil, "assigned_at": nil}

	if memberID != nil {
		var member models.ShopMember
		if err := initializers.DB.
			Where("id = ? AND shop_id = ? AND role = ?", *memberID, shopID, "confirmation").
			First(&member).Error; err != nil {
			return err
		}
		updates["assigned_member_id"] = *memberID
		updates["assigned_at"] = time.Now()
	}

	result := initializers.DB.Model(&models.Order{}).
		Where("id = ? AND shop_id = ?", orderID, shopID).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// BulkAssignOrders assigns/reassigns/clears a batch of orders in one shot —
// same rules as AssignOrder (memberID nil clears, non-nil must be a
// confirmation-role member of the shop). Returns how many orders matched.
func BulkAssignOrders(shopID uuid.UUID, orderIDs []uuid.UUID, memberID *uuid.UUID) (int64, error) {
	if len(orderIDs) == 0 {
		return 0, nil
	}

	updates := map[string]any{"assigned_member_id": nil, "assigned_at": nil}

	if memberID != nil {
		var member models.ShopMember
		if err := initializers.DB.
			Where("id = ? AND shop_id = ? AND role = ?", *memberID, shopID, "confirmation").
			First(&member).Error; err != nil {
			return 0, err
		}
		updates["assigned_member_id"] = *memberID
		updates["assigned_at"] = time.Now()
	}

	result := initializers.DB.Model(&models.Order{}).
		Where("id IN ? AND shop_id = ?", orderIDs, shopID).
		Updates(updates)
	return result.RowsAffected, result.Error
}

// ConfirmatriceRate is one confirmatrice's assigned/confirmed tally over an
// optional date window.
type ConfirmatriceRate struct {
	MemberID  uuid.UUID `json:"memberId"`
	FirstName string    `json:"firstName"`
	LastName  string    `json:"lastName"`
	Email     string    `json:"email"`
	Total     int64     `json:"total"`
	Confirmed int64     `json:"confirmed"`
	Rate      *float64  `json:"rate"`
}

// ConfirmationRates returns the "taux de confirmation" leaderboard: per
// confirmation-role member, how many assigned orders (excluding still-pending
// "En attente") reached a confirmed-or-downstream status. "Confirmed" is read
// from the order.status_changed audit trail already written by
// UpdateOrderByShopID — same "was it ever confirmed" philosophy as
// confirmationRatesByProductIDs in productsController.go — not a separate
// stamp column, the audit log already has it.
func ConfirmationRates(shopID uuid.UUID, from, to *time.Time) ([]ConfirmatriceRate, error) {
	db := initializers.DB.
		Table("orders o").
		Joins("JOIN shop_members sm ON sm.id = o.assigned_member_id").
		Joins("JOIN users u ON u.id = sm.user_id").
		Where("o.shop_id = ? AND o.deleted_at IS NULL AND o.assigned_member_id IS NOT NULL", shopID)

	if from != nil {
		db = db.Where("o.created_at >= ?", *from)
	}
	if to != nil {
		db = db.Where("o.created_at < ?", *to)
	}

	const wasConfirmedOrBeyond = `EXISTS (
		SELECT 1 FROM audit_logs al
		WHERE al.target_type = 'Order' AND al.target_id = o.id
			AND al.action = 'order.status_changed'
			AND al.metadata::json->>'to' IN ('Confirmé', 'Livré', 'Expedié')
	)`

	var rows []ConfirmatriceRate
	err := db.
		Select(`sm.id AS member_id, u.first_name, u.last_name, u.email,
			COUNT(*) FILTER (WHERE o.status <> 'En attente') AS total,
			COUNT(*) FILTER (WHERE o.status <> 'En attente' AND `+wasConfirmedOrBeyond+`) AS confirmed`).
		Group("sm.id, u.first_name, u.last_name, u.email").
		Order("confirmed DESC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	for i := range rows {
		if rows[i].Total > 0 {
			rate := float64(rows[i].Confirmed) * 100 / float64(rows[i].Total)
			rows[i].Rate = &rate
		}
	}
	return rows, nil
}
