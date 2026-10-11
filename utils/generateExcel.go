package utils

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/chtiwa/dzbazar-server/models"
	"github.com/xuri/excelize/v2"
)

func GenerateExcel(orders []models.Order) ([]byte, error) {
	f := excelize.NewFile()

	sheetName := "Orders"
	index, err := f.NewSheet(sheetName)
	if err != nil {
		return nil, fmt.Errorf("failed to create Excel file: %v", err)
	}

	// Remove default blank canvas sheet
	f.DeleteSheet("Sheet1")

	headers := []string{
		"Type De livraison",
		"Nom De Client Destinataire",
		"Telephone De Client Destinataire",
		"Wilaya De Client Destinataire",
		"De Client Destinataire Commune francais",
		"Adress De Client Destinataire",
		"Description de colis",
		"Reference",
		"Prix De Colis",
		"Fragile",
		"ouverable",
		"essayable",
		"poids",
		"Volume",
	}

	// Set header values on Row 1
	for i, header := range headers {
		cell, colErr := excelize.CoordinatesToCellName(i+1, 1)
		if colErr == nil {
			f.SetCellValue(sheetName, cell, header)
		}
	}

	// Populate data rows dynamically matching new relational constraints
	for i, order := range orders {
		row := i + 2

		// 1. Resolve Delivery Mode Layout Matrix
		deliveryType := "DOORSTEP"
		if strings.ToLower(order.ShippingMethod) == "stopdesk" {
			deliveryType = "STOPDESK"
		}

		// 2. Stitch line item array slices together into a clean description string
		var itemDescriptions []string
		for _, item := range order.Items {
			if item.ProductVariantCombination.CombinationString != "" {
				itemDescriptions = append(itemDescriptions, fmt.Sprintf("%s (%s) x%d", item.Product.Title, item.ProductVariantCombination.CombinationString, item.Quantity))
			} else {
				itemDescriptions = append(itemDescriptions, fmt.Sprintf("%s x%d", item.Product.Title, item.Quantity))
			}
		}
		descriptionText := strings.Join(itemDescriptions, ", ")

		// 3. Normalize logical boolean expressions to standard integer flags (1 or 0)
		fragileFlag := 0
		if order.Fragile {
			fragileFlag = 1
		}

		ouvrableFlag := 0
		if order.Ouvrable {
			ouvrableFlag = 1
		}

		essayableFlag := 0
		if order.Essayable {
			essayableFlag = 1
		}

		// Shipping defaults
		weightMetrics := 0.5
		volumeMetrics := 20

		// 4. Inject variables safely using the updated model relations (.Client & ID configurations)
		f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), deliveryType)
		f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), order.Client.FullName)
		f.SetCellValue(sheetName, fmt.Sprintf("C%d", row), order.Client.PhoneNumber)
		f.SetCellValue(sheetName, fmt.Sprintf("D%d", row), order.Client.StateCode) // e.g. "16" for Algiers
		f.SetCellValue(sheetName, fmt.Sprintf("E%d", row), order.Client.City)      // e.g. "Reghaia"
		f.SetCellValue(sheetName, fmt.Sprintf("F%d", row), order.Note)             // Fallback address notes or detailed location markers
		f.SetCellValue(sheetName, fmt.Sprintf("G%d", row), descriptionText)
		f.SetCellValue(sheetName, fmt.Sprintf("H%d", row), order.BaseModel.ID.String())
		f.SetCellValue(sheetName, fmt.Sprintf("I%d", row), int(order.TotalPrice))
		f.SetCellValue(sheetName, fmt.Sprintf("J%d", row), fragileFlag)
		f.SetCellValue(sheetName, fmt.Sprintf("K%d", row), ouvrableFlag)
		f.SetCellValue(sheetName, fmt.Sprintf("L%d", row), essayableFlag)
		f.SetCellValue(sheetName, fmt.Sprintf("M%d", row), weightMetrics)
		f.SetCellValue(sheetName, fmt.Sprintf("N%d", row), volumeMetrics)
	}

	StyleSheet(f, sheetName)
	f.SetActiveSheet(index)

	buffer, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("failed to write Excel dataset directly to stream: %v", err)
	}

	return buffer.Bytes(), nil
}

// StyleSheet makes an export readable: bold header, frozen header row, autofilter,
// column widths fitted to content (clamped), wrapped top-aligned cells.
func StyleSheet(f *excelize.File, sheet string) error {
	rows, err := f.GetRows(sheet)
	if err != nil || len(rows) == 0 {
		return err
	}
	ncols := len(rows[0])

	head, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"3B3B4F"}, Pattern: 1},
		Alignment: &excelize.Alignment{Vertical: "center", Horizontal: "center", WrapText: true},
	})
	body, _ := f.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{Vertical: "top", WrapText: true},
	})

	for c := 1; c <= ncols; c++ {
		width := 10
		for _, r := range rows {
			if c <= len(r) {
				if n := utf8.RuneCountInString(r[c-1]) + 2; n > width {
					width = n
				}
			}
		}
		if width > 50 {
			width = 50
		}
		col, _ := excelize.ColumnNumberToName(c)
		f.SetColWidth(sheet, col, col, float64(width))
	}

	last, _ := excelize.CoordinatesToCellName(ncols, len(rows))
	endHead, _ := excelize.CoordinatesToCellName(ncols, 1)
	f.SetCellStyle(sheet, "A1", endHead, head)
	if len(rows) > 1 {
		f.SetCellStyle(sheet, "A2", last, body)
	}
	f.SetRowHeight(sheet, 1, 30)
	f.AutoFilter(sheet, "A1:"+endHead, nil)
	return f.SetPanes(sheet, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})
}
