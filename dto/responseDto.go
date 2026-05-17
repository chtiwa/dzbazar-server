package dto

type ProductResponse struct {
	ID           string                 `json:"id"`
	Title        string                 `json:"title"`
	Description  string                 `json:"description"`
	Brand        string                 `json:"brand"`
	Price        float64                `json:"price"`
	OldPrice     float64                `json:"oldPrice"`
	Images       []ProductImageResponse `json:"images"`
	Variants     []VariantResponse      `json:"variants"`
	Tags         []string               `json:"tags"`
	Combinations []CombinationResponse
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
	Option1ID         *string `json:"option1Id"`
	Option2ID         *string `json:"option2Id"`
	Option3ID         *string `json:"option3Id"`
	CombinationString string  `json:"combinationString"`
}
