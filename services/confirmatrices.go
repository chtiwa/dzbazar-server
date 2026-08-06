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

// ConfirmatriceRate is one shop member's confirmation/delivery tally over an
// optional date window. Includes all roles (owner, moderator, confirmation);
// counts orders where this member is the actor on any status-change audit entry,
// not just orders formally assigned to them.
type ConfirmatriceRate struct {
	MemberID      uuid.UUID `json:"memberId"`
	Role          string    `json:"role"`
	FirstName     string    `json:"firstName"`
	LastName      string    `json:"lastName"`
	Email         string    `json:"email"`
	Total         int64     `json:"total"`
	Confirmed     int64     `json:"confirmed"`
	Delivered     int64     `json:"delivered"`
	Rate          *float64  `json:"rate"`
	DeliveredRate *float64  `json:"deliveredRate"`
}

// ConfirmationRates returns the "taux de confirmation" report: per shop member
// (all roles), how many orders this member acted upon (as the audit-log actor on
// any status-change), excluding still-pending "En attente". Confirms are read
// from audit_logs where metadata->>'to' = 'Confirmé'; delivered is the current
// orders.status = 'Livré' of confirmed orders (carrier syncs bypass LogAudit,
// so we read live status, not the audit trail — same pattern as deliveryRate
// in productsController.go).
func ConfirmationRates(shopID uuid.UUID, from, to *time.Time) ([]ConfirmatriceRate, error) {
	// Shop/date scope lives in the orders JOIN's ON clause (not WHERE) so a
	// member with zero touched orders still gets a row via the LEFT JOINs.
	ordersJoin := "LEFT JOIN orders o ON o.id = al.target_id AND o.shop_id = ? AND o.deleted_at IS NULL"
	ordersArgs := []any{shopID}
	if from != nil {
		ordersJoin += " AND o.created_at >= ?"
		ordersArgs = append(ordersArgs, *from)
	}
	if to != nil {
		ordersJoin += " AND o.created_at < ?"
		ordersArgs = append(ordersArgs, *to)
	}

	var rows []ConfirmatriceRate
	err := initializers.DB.
		Table("shop_members sm").
		Joins("JOIN users u ON u.id = sm.user_id").
		// actor_id is the User ID (see utils.LogAudit), not shop_member_id — join on user_id.
		Joins("LEFT JOIN audit_logs al ON al.actor_id = sm.user_id AND al.target_type = 'Order' AND al.action = 'order.status_changed'").
		Joins(ordersJoin, ordersArgs...).
		Where("sm.shop_id = ?", shopID).
		Select(`sm.id AS member_id, sm.role AS role, u.first_name, u.last_name, u.email,
			COUNT(DISTINCT o.id) FILTER (WHERE o.status <> 'En attente') AS total,
			COUNT(DISTINCT o.id) FILTER (WHERE o.status <> 'En attente' AND al.metadata::json->>'to' = 'Confirmé') AS confirmed,
			COUNT(DISTINCT o.id) FILTER (WHERE al.metadata::json->>'to' = 'Confirmé' AND o.status = 'Livré') AS delivered`).
		Group("sm.id, sm.role, u.first_name, u.last_name, u.email").
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
		if rows[i].Confirmed > 0 {
			// ponytail: same recency bias as dashboardController's deliveryRate
			// (a just-confirmed/shipped order can't be Livré yet), but here
			// Confirmed — not Shipped — is the denominator, and it's also the
			// displayed "confirmed" column and the Rate denominator. Gating it
			// on shipped_at would desync the on-screen counts from the rate.
			// Upgrade only if merchants report this number looking off.
			dr := float64(rows[i].Delivered) * 100 / float64(rows[i].Confirmed)
			rows[i].DeliveredRate = &dr
		}
	}
	return rows, nil
}
