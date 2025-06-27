package dto

var ProductInput struct {
	Title       string        `json:"title"`
	Description string        `json:"description"`
	Price       float64       `json:"price"`
	OldPrice    float64       `json:"oldPrice"`
	CategoryID  string        `json:"categoryId"` // at first it would be a string
	Images      []interface{} `json:"images"`
}
