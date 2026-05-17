package dto

type UpdateVariantDTO struct {
	Title        string `json:"title"`
	VariantItems []struct {
		ID       string `json:"id"`
		Price    int    `json:"price"`
		Value    string `json:"value"`
		Quantity int    `json:"quantity"`
	} `json:"variantItems"`
}

type UpdateProductsImageInput struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

type UpdateLandingPageImageInput struct {
	ID         string `json:"id"`
	URL        string `json:"url"`
	OrderIndex int    `json:"orderIndex"`
}

type CombinationInput struct {
	SKU      string  `json:"sku"`
	Price    float64 `json:"price"`
	Quantity int     `json:"quantity"`
	Option1  *string `json:"option1"` // e.g., "Red"
	Option2  *string `json:"option2"` // e.g., "41"
	Option3  *string `json:"option3"`
}

type VariantInput struct {
	Title        string `json:"title"`
	VariantItems []struct {
		Value    string  `json:"value"`
		Price    float64 `json:"price"`
		Quantity int     `json:"quantity"`
	} `json:"variantItems"`
}

type VariantItemInput struct {
	Value    string `json:"value"`
	Price    int    `json:"price"`
	Quantity int    `json:"quantity"`
}

type TagInput struct {
	Name string `json:"name"`
}

type CreateProductInput struct {
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Price       float64        `json:"price"`
	CategoryID  string         `json:"categoryId"`
	Variants    []VariantInput `json:"variants"`
	Tags        []string       `json:"tags"`
}
