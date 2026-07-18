package controllers

import (
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// stockRow is one product-variant-combination (SKU) flattened with its parent product's
// title/image and its option values, for the cross-product stock triage page.
type stockRow struct {
	CombinationID uuid.UUID `json:"combinationId"`
	SKU           string    `json:"sku"`
	Quantity      int       `json:"quantity"`
	Price         float64   `json:"price"`
	ProductID     uuid.UUID `json:"productId"`
	ProductTitle  string    `json:"productTitle"`
	ProductImage  *string   `json:"productImage"`
	Option1       *string   `json:"option1"`
	Option2       *string   `json:"option2"`
	Option3       *string   `json:"option3"`
}

// GetShopStock returns every SKU (product variant combination) for the shop, flattened and
// paginated by SKU (not by product — a many-variant product would otherwise skew a per-product
// page size), sortable by quantity so merchants can triage low stock across their whole catalog.
func GetShopStock(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid shop ID",
		})
		return
	}

	search := strings.TrimSpace(c.Query("search"))
	page := 1
	perPage := 10

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

	base := initializers.DB.
		Table("product_variant_combinations AS c").
		Joins("JOIN products p ON p.id = c.product_id AND p.deleted_at IS NULL").
		Joins("LEFT JOIN variant_items o1 ON o1.id = c.option1_id").
		Joins("LEFT JOIN variant_items o2 ON o2.id = c.option2_id").
		Joins("LEFT JOIN variant_items o3 ON o3.id = c.option3_id").
		Where("p.shop_id = ? AND c.deleted_at IS NULL", shopID)

	if search != "" {
		like := "%" + search + "%"
		base = base.Where("(p.title ILIKE ? OR c.sku ILIKE ?)", like, like)
	}

	if v := c.Query("lowStock"); v != "" {
		if threshold, err := strconv.Atoi(v); err == nil && threshold >= 0 {
			base = base.Where("c.quantity <= ?", threshold)
		}
	}

	var totalRows int64
	if err := base.Count(&totalRows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "error while counting stock rows",
			"error":   err.Error(),
		})
		return
	}

	totalPages := int(math.Ceil(float64(totalRows) / float64(perPage)))
	if totalPages == 0 {
		totalPages = 1
	}
	if page > totalPages {
		page = totalPages
	}

	order := "c.quantity ASC, c.sku ASC"
	if c.Query("sort") == "quantity_desc" {
		order = "c.quantity DESC, c.sku ASC"
	}

	var rows []stockRow
	err = base.
		Select(`c.id AS combination_id, c.sku, c.quantity, c.price,
			p.id AS product_id, p.title AS product_title,
			(SELECT url FROM product_images pi WHERE pi.product_id = p.id AND pi.deleted_at IS NULL ORDER BY pi.order_index LIMIT 1) AS product_image,
			o1.value AS option1, o2.value AS option2, o3.value AS option3`).
		Order(order).
		Limit(perPage).
		Offset((page - 1) * perPage).
		Scan(&rows).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "error while retrieving stock",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    rows,
		"pagination": gin.H{
			"page":       page,
			"perPage":    perPage,
			"totalRows":  totalRows,
			"totalPages": totalPages,
			"hasNext":    page < totalPages,
			"hasPrev":    page > 1,
		},
	})
}

// UpdateStockQuantity sets the quantity for one SKU. Shop-scoped via a subquery so a
// combination belonging to another tenant can never be touched. Deliberately bypasses the
// full variant teardown/rebuild sequence used by UpdateProductByShop — that sequence exists to
// safely reconcile options/SKUs when variants change shape, which doesn't apply here: this
// only ever changes one integer on one existing row.
func UpdateStockQuantity(c *gin.Context) {
	shopID, err := uuid.Parse(c.Param("shopId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid shop ID",
		})
		return
	}

	combinationID, err := uuid.Parse(c.Param("combinationId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid combination ID",
		})
		return
	}

	var body struct {
		Quantity int `json:"quantity" binding:"gte=0"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request body",
			"error":   err.Error(),
		})
		return
	}

	var combination models.ProductVariantCombination
	if err := initializers.DB.
		Where("id = ? AND product_id IN (SELECT id FROM products WHERE shop_id = ?)", combinationID, shopID).
		First(&combination).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Stock item not found",
		})
		return
	}

	if err := initializers.DB.Model(&combination).Update("quantity", body.Quantity).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to update stock",
			"error":   err.Error(),
		})
		return
	}

	invalidateProductCaches(combination.ProductID, shopID)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Stock updated successfully",
		"data": gin.H{
			"combinationId": combination.ID,
			"quantity":      body.Quantity,
		},
	})
}
