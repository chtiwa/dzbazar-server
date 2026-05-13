package utils

import (
	"fmt"

	"github.com/chtiwa/lk-parfumo-server/models"
	"github.com/xuri/excelize/v2"
)

func GenerateExcel(orders []models.Order) ([]byte, error) {
	f := excelize.NewFile()

	sheetName := "Orders"
	index, err := f.NewSheet(sheetName)
	if err != nil {
		return nil, fmt.Errorf("failed to create Excel file: %v", err)
	}

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

	columns := []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K", "L", "M", "N"}

	for i, header := range headers {
		cell := fmt.Sprintf("%s1", columns[i])
		f.SetCellValue(sheetName, cell, header)
	}

	for i, order := range orders {
		row := i + 2

		deliveryType := "DOORSTEP"
		if order.ShippingMethod == "Stopdesk" {
			deliveryType = "STOPDESK"
		}

		description := order.ProductName
		if order.Quantity > 1 {
			description = fmt.Sprintf("%s x%d", order.ProductName, order.Quantity)
		}

		reference := order.ID
		address := ""
		fragile := 1
		ouvrable := 1
		essayable := 1
		poids := 0.5
		volume := 1

		f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), deliveryType)
		f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), order.FullName)
		f.SetCellValue(sheetName, fmt.Sprintf("C%d", row), order.PhoneNumber)
		f.SetCellValue(sheetName, fmt.Sprintf("D%d", row), order.StateNumber)
		f.SetCellValue(sheetName, fmt.Sprintf("E%d", row), order.City)
		f.SetCellValue(sheetName, fmt.Sprintf("F%d", row), address)
		f.SetCellValue(sheetName, fmt.Sprintf("G%d", row), description)
		f.SetCellValue(sheetName, fmt.Sprintf("H%d", row), reference)
		f.SetCellValue(sheetName, fmt.Sprintf("I%d", row), int(order.TotalPrice))
		f.SetCellValue(sheetName, fmt.Sprintf("J%d", row), fragile)
		f.SetCellValue(sheetName, fmt.Sprintf("K%d", row), ouvrable)
		f.SetCellValue(sheetName, fmt.Sprintf("L%d", row), essayable)
		f.SetCellValue(sheetName, fmt.Sprintf("M%d", row), poids)
		f.SetCellValue(sheetName, fmt.Sprintf("N%d", row), volume)
	}

	f.SetActiveSheet(index)

	buffer, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("failed to create Excel file: %v", err)
	}

	return buffer.Bytes(), nil
}
