package dto

import (
	"github.com/chtiwa/herbs-store-client/models"
)

type ProductResponse struct {
	ID          string                `json:"id"`
	Title       string                `json:"title"`
	Description string                `json:"description"`
	Price       float64               `json:"price"`
	OldPrice    float64               `json:"oldPrice"`
	Images      []models.ProductImage `json:"images"`
	Category    CategoryResponse      `json:"category"`
	Variants    []VariantResponse     `json:"variants"`
}

type CategoryResponse struct {
	ID    string `json:"id"`
	Title string `json:"title"`
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
}
