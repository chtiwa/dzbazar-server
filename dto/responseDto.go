package dto

import "time"

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
	ID       string `json:"id"`
	Value    string `json:"value"`
	Quantity int    `json:"quantity"`
	Price    int    `json:"price"`
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
