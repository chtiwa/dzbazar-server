package models

import (
	"time"

	"github.com/google/uuid"
)

// OfferConditions is the rule set stored as JSON text on the offer row.
// An empty Predicates slice means "always match".
type OfferConditions struct {
	Match      string           `json:"match"`      // "all"
	Predicates []OfferPredicate `json:"predicates"`
}

// OfferPredicate is one condition. Value holds a JSON-decoded value:
// string, float64, []interface{} — standard encoding/json unmarshaling types.
type OfferPredicate struct {
	Field string      `json:"field"` // current_product|current_landing_page|selected_variant|selected_quantity|customer_wilaya|utm_source
	Op    string      `json:"op"`    // eq|in|gte
	Value interface{} `json:"value"`
}

// Offer is a merchant-authored upsell or cross-sell scoped to a trigger product.
// LandingPageID = nil means it applies to the product page and every LP for that product.
type Offer struct {
	BaseModel
	ShopID       uuid.UUID `gorm:"not null;index"           json:"shopId"`
	InternalName string    `gorm:"not null"                 json:"internalName"`
	Status       string    `gorm:"not null;default:'draft'" json:"status"` // draft|published|archived
	Action       string    `gorm:"not null"                 json:"action"`  // replace|append|mutate_qty — also the offer's "type": replace=upsell, append=cross_sell/order_bump (see placement), mutate_qty=quantity_upsell

	TriggerProductID uuid.UUID `gorm:"not null;index"  json:"triggerProductId"`
	TriggerProduct   *Product  `gorm:"foreignKey:TriggerProductID;references:ID;constraint:OnDelete:CASCADE" json:"triggerProduct,omitempty"`

	// nil = base offer (all LPs). Non-nil = this offer is LP-scoped directly.
	LandingPageID *uuid.UUID   `gorm:"type:uuid;index" json:"landingPageId"`
	LandingPage   *LandingPage `gorm:"foreignKey:LandingPageID;references:ID;constraint:OnDelete:CASCADE" json:"landingPage,omitempty"`

	OfferProductID uuid.UUID `gorm:"not null"  json:"offerProductId"`
	OfferProduct   *Product  `gorm:"foreignKey:OfferProductID;references:ID;constraint:OnDelete:CASCADE" json:"offerProduct,omitempty"`

	// Combination IDs the merchant wants to offer. Stored as JSON array of UUID strings.
	OfferVariantIDs []uuid.UUID `gorm:"type:text;serializer:json" json:"offerVariantIds"`
	QuantityRule    int         `gorm:"not null;default:1"        json:"quantityRule"`

	DiscountType  string  `gorm:"not null;default:'percent'" json:"discountType"`  // percent|fixed|override_price
	DiscountValue float64 `gorm:"not null;default:0"         json:"discountValue"`

	Headline    string `gorm:"not null"                         json:"headline"`
	Subheadline string `json:"subheadline"`
	ButtonText  string `gorm:"not null;default:'Add to my order'" json:"buttonText"`
	MediaURL    string `json:"mediaUrl"`

	Placement string `gorm:"not null"         json:"placement"` // under_variant|above_submit|order_form
	Priority  int    `gorm:"not null;default:100" json:"priority"`

	Conditions        OfferConditions `gorm:"type:text;serializer:json"       json:"conditions"`
	InventoryBehavior string          `gorm:"not null;default:'skip_when_oos'" json:"inventoryBehavior"` // skip_when_oos|pause_when_oos
	AnalyticsTag      string          `json:"analyticsTag"`

	StartAt *time.Time `json:"startAt"`
	EndAt   *time.Time `json:"endAt"`
}

// OfferPageOverride lets a merchant tweak or disable a base offer for one specific LP.
// Nil override fields mean "use the base offer value".
type OfferPageOverride struct {
	BaseModel
	ShopID        uuid.UUID `gorm:"not null;index"                           json:"shopId"`
	OfferID       uuid.UUID `gorm:"not null;uniqueIndex:idx_override_offer_lp" json:"offerId"`
	Offer         Offer     `gorm:"foreignKey:OfferID;references:ID;constraint:OnDelete:CASCADE" json:"-"`
	LandingPageID uuid.UUID `gorm:"not null;uniqueIndex:idx_override_offer_lp" json:"landingPageId"`
	LandingPage   LandingPage `gorm:"foreignKey:LandingPageID;references:ID;constraint:OnDelete:CASCADE" json:"-"`

	Enabled        bool       `gorm:"not null;default:true" json:"enabled"`
	Headline       *string    `json:"headline"`
	Subheadline    *string    `json:"subheadline"`
	ButtonText     *string    `json:"buttonText"`
	OfferProductID *uuid.UUID `gorm:"type:uuid"            json:"offerProductId"`
	DiscountType   *string    `json:"discountType"`
	DiscountValue  *float64   `json:"discountValue"`
	Placement      *string    `json:"placement"`
}

// OfferEvent records one storefront interaction for analytics (impression/click/accept/revenue).
type OfferEvent struct {
	BaseModel
	ShopID    uuid.UUID  `gorm:"not null;index"  json:"shopId"`
	OfferID   uuid.UUID  `gorm:"not null;index"  json:"offerId"`
	Offer     Offer      `gorm:"foreignKey:OfferID;references:ID;constraint:OnDelete:CASCADE" json:"-"`
	OrderID   *uuid.UUID `gorm:"type:uuid;index" json:"orderId"`
	Event     string     `gorm:"not null"        json:"event"` // impression|click|accept|revenue
	VariantID *uuid.UUID `gorm:"type:uuid"       json:"variantId"`
	Wilaya    *int       `json:"wilaya"`
	Amount    float64    `gorm:"not null;default:0" json:"amount"`
}
