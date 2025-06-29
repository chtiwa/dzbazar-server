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
