package models

import "github.com/google/uuid"

// ConfirmatriceProduct scopes a confirmation-role ShopMember to a product
// they're allowed to see/handle orders for. No rows for a member means the
// member sees nothing until scoped (an allow-list, not a wildcard).
type ConfirmatriceProduct struct {
	ShopMemberID uuid.UUID `gorm:"type:uuid;primaryKey" json:"shopMemberId"`
	ProductID    uuid.UUID `gorm:"type:uuid;primaryKey" json:"productId"`
}
