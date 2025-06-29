package dto

type ProductResponse struct {
	ID          string                 `json:"id"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Price       float64                `json:"price"`
	OldPrice    float64                `json:"oldPrice"`
	Images      []ProductImageResponse `json:"images"`
	Category    CategoryResponse       `json:"category"`
	Variants    []VariantResponse      `json:"variants"`
}

type CategoryResponse struct {
	ID    string `json:"id"`
	Title string `json:"title"`
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
