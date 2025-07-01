package utils

type PaginationData struct {
	CurrentPage int    `json:"currentPage"`
	TotalPages  int    `json:"totalPages"`
	BaseURL     string `json:"baseUrl"`
}

func GetPaginationData(page int, totalPages int, baseURL string) PaginationData {
	return PaginationData{
		CurrentPage: page,
		TotalPages:  totalPages,
		BaseURL:     baseURL,
	}
}
