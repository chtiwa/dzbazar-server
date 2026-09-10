package models

import "github.com/google/uuid"

// Bureau is one stopdesk desk name a shop owner typed in for one of their
// connected carriers in one wilaya. Only carriers without a live hub API get
// rows here -- Osen, Leopard, Anderson. ZR Express resolves hubs live through
// its own API and never reads this table.
//
// Uniqueness (shop + carrier + wilaya + name, partial on deleted_at IS NULL)
// lives in migration 00036 rather than a GORM uniqueIndex tag, because GORM
// can't express the partial predicate and AutoMigrate is not used here.
// Multiple distinct desk names per wilaya are expected and allowed.
type Bureau struct {
	BaseModel
	ShopID uuid.UUID `gorm:"type:uuid;not null;index:idx_bureaux_shop_wilaya" json:"shopId"`
	Shop   Shop      `gorm:"foreignKey:ShopID;references:ID" json:"-"`

	DeliveryCompanyID uuid.UUID       `gorm:"type:uuid;not null;index" json:"deliveryCompanyId"`
	DeliveryCompany   DeliveryCompany `gorm:"foreignKey:DeliveryCompanyID;references:ID" json:"deliveryCompany,omitempty"`

	WilayaID int    `gorm:"not null;index:idx_bureaux_shop_wilaya" json:"wilayaId"` // e.g. 16 for Alger
	Name     string `gorm:"not null" json:"name"`
}
