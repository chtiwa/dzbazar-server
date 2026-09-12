package dto

import (
	"time"

	"github.com/chtiwa/dzbazar-server/models"
	"github.com/google/uuid"
)

// ExperimentSetStandingResponse is one set's live standing inside an A/B test —
// views/conversions/rate come straight from the same queries the standalone
// landing-page PagePerf panel already uses, just scoped to this set's ID.
type ExperimentSetStandingResponse struct {
	LandingPageID  string   `json:"landingPageId"`
	Position       int      `json:"position"`
	Title          string   `json:"title"`
	Views          int64    `json:"views"`
	Conversions    int64    `json:"conversions"`
	ConversionRate *float64 `json:"conversionRate"`
	Active         bool     `json:"active"`
	IsWinner       bool     `json:"isWinner"`
}

// ExperimentResponse is an experiment plus its live per-set standings.
// LeadingLandingPageID/PValue/IsSignificant describe the current rate-leader
// vs the rest, pre-decision — nil/false until enough data exists to compute.
type ExperimentResponse struct {
	ID                   string                          `json:"id"`
	Name                 string                          `json:"name"`
	ProductID            string                          `json:"productId"`
	TargetConversions    int                             `json:"targetConversions"`
	Status               string                          `json:"status"`
	WinnerLandingPageID  *string                         `json:"winnerLandingPageId,omitempty"`
	LeadingLandingPageID *string                         `json:"leadingLandingPageId,omitempty"`
	PValue               *float64                        `json:"pValue,omitempty"`
	IsSignificant        bool                            `json:"isSignificant"`
	Standings            []ExperimentSetStandingResponse `json:"standings"`
	CreatedAt            time.Time                       `json:"createdAt"`
}

type ProductResponse struct {
	ID           string                 `json:"id"`
	Title        string                 `json:"title"`
	Description  string                 `json:"description"`
	Price        float64                `json:"price"`
	OldPrice     float64                `json:"oldPrice"`
	Images       []ProductImageResponse `json:"images"`
	Variants     []VariantResponse      `json:"variants"`
	Tags         []string               `json:"tags"`
	Combinations []CombinationResponse  `json:"combinations"`
}

type TagResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type ProductImageResponse struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

type VariantResponse struct {
	ID           string              `json:"id"`
	Title        string              `json:"title"`
	VariantItems []VariantItemSimple `json:"variantItems"`
}

type VariantItemSimple struct {
	ID       string  `json:"id"`
	Value    string  `json:"value"`
	Quantity int     `json:"quantity"`
	Price    int     `json:"price"`
	ImageURL *string `json:"imageUrl,omitempty"`
}
type CombinationResponse struct {
	ID                string  `json:"id"`
	SKU               string  `json:"sku"`
	Price             float64 `json:"price"`
	Quantity          int     `json:"quantity"`
	Option1ID         *string `json:"option1Id,omitempty"`
	Option2ID         *string `json:"option2Id,omitempty"`
	Option3ID         *string `json:"option3Id,omitempty"`
	Option1Value      *string `json:"option1Value,omitempty"`
	Option2Value      *string `json:"option2Value,omitempty"`
	Option3Value      *string `json:"option3Value,omitempty"`
	CombinationString string  `json:"combinationString"`
}

// PublicLandingPageShop is the nested shop projection for a public landing
// page — deliberately drops Owner, fraud toggles (Ban*Enabled), SuspendReason,
// OwnerID, Phone, Email, Address: none of that belongs on an unauthenticated
// public page.
type PublicLandingPageShop struct {
	ID        string                `json:"id"`
	Slug      string                `json:"slug"`
	Name      string                `json:"name"`
	LogoImage *ProductImageResponse `json:"logoImage,omitempty"`
}

// PublicLandingPageVariantItem/Variant mirror models.VariantItem/Variant
// trimmed to what a public landing page renders.
type PublicLandingPageVariantItem struct {
	ID       string  `json:"id"`
	Value    string  `json:"value"`
	ImageURL *string `json:"imageUrl,omitempty"`
}

type PublicLandingPageVariant struct {
	ID           string                         `json:"id"`
	Title        string                         `json:"title"`
	VariantItems []PublicLandingPageVariantItem `json:"variantItems"`
}

type PublicLandingPageCombination struct {
	ID                string                        `json:"id"`
	ProductID         string                        `json:"productId"`
	Price             float64                       `json:"price"`
	Quantity          int                           `json:"quantity"`
	CombinationString string                        `json:"combinationString"`
	Option1           *PublicLandingPageVariantItem `json:"option1,omitempty"`
	Option2           *PublicLandingPageVariantItem `json:"option2,omitempty"`
	Option3           *PublicLandingPageVariantItem `json:"option3,omitempty"`
}

// PublicLandingPageProduct is models.Product trimmed for the public landing
// page path: drops HiddenByPlatformAt (moderation state — the handler
// already 404s on it, it shouldn't also leak in the body) and the
// Orders/DeliveryRate/Views/ConversionRate/ConfirmationRate stats fields,
// which are always zero/null on this path and unused by the client.
type PublicLandingPageProduct struct {
	ID           string                         `json:"id"`
	Title        string                         `json:"title"`
	Description  string                         `json:"description"`
	Price        float64                        `json:"price"`
	OldPrice     *float64                       `json:"oldPrice"`
	Images       []ProductImageResponse         `json:"images"`
	Variants     []PublicLandingPageVariant     `json:"variants"`
	Combinations []PublicLandingPageCombination `json:"combinations"`
}

// PublicShopResponse is the DTO for the public, unauthenticated
// shop-by-slug endpoint (IndexShopBySlug). Deliberately drops Owner (with its
// nested Password/EmailOTP), OwnerID, SuspendReason, Ban*Enabled fraud
// toggles, Phone, Email, Address — none of that belongs on an unauthenticated
// public page. FacebookURL/InstagramURL/TiktokURL are kept: they're public
// social links actively rendered by client/src/components/Footer.tsx off
// this same response.
type PublicShopResponse struct {
	ID           string                `json:"id"`
	Slug         string                `json:"slug"`
	Name         string                `json:"name"`
	Description  string                `json:"description"`
	LogoImage    *ProductImageResponse `json:"logoImage,omitempty"`
	FacebookURL  string                `json:"facebookUrl,omitempty"`
	InstagramURL string                `json:"instagramUrl,omitempty"`
	TiktokURL    string                `json:"tiktokUrl,omitempty"`
}

// PublicLandingPageResponse is the DTO for the public, unauthenticated
// landing-page-by-id endpoint (IndexLandingPage). It projects the landing
// page's own public fields plus a trimmed Shop and Product — see
// PublicLandingPageShop/PublicLandingPageProduct for what's dropped.
type PublicLandingPageResponse struct {
	ID           string                   `json:"id"`
	ShopID       string                   `json:"shopId"`
	Shop         PublicLandingPageShop    `json:"shop"`
	ProductID    string                   `json:"productId"`
	Product      PublicLandingPageProduct `json:"product"`
	Title        string                   `json:"title"`
	Images       []ProductImageResponse   `json:"images"`
	Active       bool                     `json:"active"`
	ExperimentID *string                  `json:"experimentId,omitempty"`
}

// OrderResponse is models.Order minus ClientIP/ClientUserAgent — those are
// server-internal (only ever read from the DB directly by the Meta CAPI
// replay code, never from this HTTP response) and never belong in the
// admin-facing order response. Every other field the admin frontend consumes
// (see admin/src/store/types/Order.ts) is kept as-is, including nested
// Client/Items/ShippedVia/AssignedMember — those aren't the leak.
type OrderResponse struct {
	ID     uuid.UUID `json:"id"`
	ShopID uuid.UUID `json:"shopId"`

	ClientID uuid.UUID     `json:"clientId"`
	Client   models.Client `json:"client"`

	ShippingMethod string  `json:"shippingMethod"`
	ShippingPrice  float64 `json:"shippingPrice"`
	TotalPrice     float64 `json:"totalPrice"`
	Status         string  `json:"status"`

	CouponID       *uuid.UUID `json:"couponId"`
	DiscountAmount float64    `json:"discountAmount"`

	ReportedDate *time.Time `json:"reportedDate"`

	Note string `json:"note"`

	TrackingNumber   string `json:"trackingNumber"`
	Ouvrable         bool   `json:"ouvrable"`
	Fragile          bool   `json:"fragile"`
	Essayable        bool   `json:"essayable"`
	FBclid           string `json:"fbclid"`
	FBc              string `json:"fbc"`
	FBp              string `json:"fbp"`
	TTclid           string `json:"ttclid"`
	TTp              string `json:"ttp"`
	ConversionSource string `json:"conversionSource"`
	IsShipped        bool   `json:"isShipped"`

	LandingPageID *uuid.UUID `json:"landingPageId"`

	IsHidden     bool   `json:"isHidden"`
	HiddenReason string `json:"hiddenReason"`

	MetaPurchaseSentAt   *time.Time `json:"metaPurchaseSentAt"`
	MetaPurchaseAttempts int        `json:"metaPurchaseAttempts"`

	SheetsExportSentAt   *time.Time `json:"sheetsExportSentAt"`
	SheetsExportAttempts int        `json:"sheetsExportAttempts"`

	PageURL string `json:"pageUrl"`

	ShippedAt    *time.Time                       `json:"shippedAt"`
	ShippedViaID *uuid.UUID                       `json:"shippedViaId"`
	ShippedVia   *models.AvailableDeliveryCompany `json:"shippedVia"`

	AssignedMemberID *uuid.UUID         `json:"assignedMemberId"`
	AssignedAt       *time.Time         `json:"assignedAt"`
	AssignedMember   *models.ShopMember `json:"assignedMember,omitempty"`

	Items []models.OrderItem `json:"items"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ToOrderResponse projects a models.Order down to OrderResponse, dropping
// ClientIP/ClientUserAgent. Used by GetOrdersByShopID/IndexOrderByShopID —
// the two admin order-detail/list handlers that serialize a full order.
func ToOrderResponse(o models.Order) OrderResponse {
	return OrderResponse{
		ID:                   o.ID,
		ShopID:               o.ShopID,
		ClientID:             o.ClientID,
		Client:               o.Client,
		ShippingMethod:       o.ShippingMethod,
		ShippingPrice:        o.ShippingPrice,
		TotalPrice:           o.TotalPrice,
		Status:               o.Status,
		CouponID:             o.CouponID,
		DiscountAmount:       o.DiscountAmount,
		ReportedDate:         o.ReportedDate,
		Note:                 o.Note,
		TrackingNumber:       o.TrackingNumber,
		Ouvrable:             o.Ouvrable,
		Fragile:              o.Fragile,
		Essayable:            o.Essayable,
		FBclid:               o.FBclid,
		FBc:                  o.FBc,
		FBp:                  o.FBp,
		TTclid:               o.TTclid,
		TTp:                  o.TTp,
		ConversionSource:     o.ConversionSource,
		IsShipped:            o.IsShipped,
		LandingPageID:        o.LandingPageID,
		IsHidden:             o.IsHidden,
		HiddenReason:         o.HiddenReason,
		MetaPurchaseSentAt:   o.MetaPurchaseSentAt,
		MetaPurchaseAttempts: o.MetaPurchaseAttempts,
		SheetsExportSentAt:   o.SheetsExportSentAt,
		SheetsExportAttempts: o.SheetsExportAttempts,
		PageURL:              o.PageURL,
		ShippedAt:            o.ShippedAt,
		ShippedViaID:         o.ShippedViaID,
		ShippedVia:           o.ShippedVia,
		AssignedMemberID:     o.AssignedMemberID,
		AssignedAt:           o.AssignedAt,
		AssignedMember:       o.AssignedMember,
		Items:                o.Items,
		CreatedAt:            o.CreatedAt,
		UpdatedAt:            o.UpdatedAt,
	}
}

// ToOrderResponses maps ToOrderResponse over a slice, for list endpoints.
func ToOrderResponses(orders []models.Order) []OrderResponse {
	out := make([]OrderResponse, 0, len(orders))
	for _, o := range orders {
		out = append(out, ToOrderResponse(o))
	}
	return out
}

// ClientResponse is models.Client minus Banned — the admin (merchant)
// clients endpoints in controllers/clientsController.go never need it
// (admin/src/services/clientsService.ts's Client type doesn't declare it and
// no admin UI reads it). Distinct from the super-admin clients endpoints
// (controllers/superadmin/clientsController.go), which genuinely need Banned
// for the ban/unban feature and keep serializing models.Client directly.
type ClientResponse struct {
	ID     uuid.UUID `json:"id"`
	ShopID uuid.UUID `json:"shopId"`

	FullName      string `json:"fullName"`
	PhoneNumber   string `json:"phoneNumber"`
	PhoneNumber2  string `json:"phoneNumber2"`
	State         string `json:"state"`
	StateCode     string `json:"stateCode"`
	City          string `json:"city"`
	StopdeskPoint string `json:"stopdeskPoint"`

	Orders []models.Order `json:"orders,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func ToClientResponse(cl models.Client) ClientResponse {
	return ClientResponse{
		ID:            cl.ID,
		ShopID:        cl.ShopID,
		FullName:      cl.FullName,
		PhoneNumber:   cl.PhoneNumber,
		PhoneNumber2:  cl.PhoneNumber2,
		State:         cl.State,
		StateCode:     cl.StateCode,
		City:          cl.City,
		StopdeskPoint: cl.StopdeskPoint,
		Orders:        cl.Orders,
		CreatedAt:     cl.CreatedAt,
		UpdatedAt:     cl.UpdatedAt,
	}
}

func ToClientResponses(clients []models.Client) []ClientResponse {
	out := make([]ClientResponse, 0, len(clients))
	for _, cl := range clients {
		out = append(out, ToClientResponse(cl))
	}
	return out
}
