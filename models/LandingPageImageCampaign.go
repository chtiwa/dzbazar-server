package models

import "github.com/google/uuid"

// LandingPageImageCampaign persists the inputs behind one generate-image-set
// call (product context, buyer persona, brand/style prefs) so the merchant
// can fetch the set as a unit and regenerate it later with the same inputs.
type LandingPageImageCampaign struct {
	BaseModel
	ShopID uuid.UUID  `gorm:"type:uuid;not null;index" json:"shopId"`
	UserID *uuid.UUID `gorm:"type:uuid" json:"userId"`

	ProductName    string `gorm:"not null" json:"productName"`
	Category       string `gorm:"not null" json:"category"`
	PrimaryBenefit string `gorm:"not null" json:"primaryBenefit"`
	PainPoint      string `gorm:"not null" json:"painPoint"`
	Audience       string `gorm:"not null" json:"audience"`
	OfferDetails   string `gorm:"not null" json:"offerDetails"`

	AgeRange       string `json:"ageRange"`
	Gender         string `json:"gender"`
	Desires        string `json:"desires"`
	Objections     string `json:"objections"`
	BrandColors    string `json:"brandColors"`
	Mood           string `json:"mood"`
	ReferenceStyle string `json:"referenceStyle"`
	CustomNotes    string `json:"customNotes"`

	ImageCount int `gorm:"not null" json:"imageCount"`

	Images []LandingPageImageGenUsage `gorm:"foreignKey:CampaignID" json:"images,omitempty"`
}
