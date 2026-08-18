package superadmin

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// parsePageParams reads page/perPage query params with the same defaults and
// clamping used across the rest of the codebase (see GetProductsByShopAdmin).
func parsePageParams(c *gin.Context) (page int, perPage int) {
	page = 1
	perPage = 20

	if v := c.Query("page"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			page = parsed
		}
	}
	if v := c.Query("perPage"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 && parsed <= 100 {
			perPage = parsed
		}
	}

	return page, perPage
}

// resolveSort reads the sortBy/sortDir query params and validates sortBy
// against an allow-list of "public name" -> "GORM column" pairs, so the raw
// query param is never interpolated into SQL. Falls back to defaultOrder
// (a full "column DIR" clause) when sortBy is absent or unknown.
func resolveSort(c *gin.Context, allowed map[string]string, defaultOrder string) string {
	column, ok := allowed[strings.TrimSpace(c.Query("sortBy"))]
	if !ok {
		return defaultOrder
	}

	dir := "ASC"
	if strings.EqualFold(c.Query("sortDir"), "desc") {
		dir = "DESC"
	}
	return column + " " + dir
}

func paginationMeta(page, perPage int, totalRows int64) gin.H {
	totalPages := int((totalRows + int64(perPage) - 1) / int64(perPage))
	if totalPages < 1 {
		totalPages = 1
	}

	return gin.H{
		"page":       page,
		"perPage":    perPage,
		"totalRows":  totalRows,
		"totalPages": totalPages,
		"hasNext":    page < totalPages,
		"hasPrev":    page > 1,
	}
}
