package utils

import (
	"fmt"

	"github.com/chtiwa/herbs-store-client/models"
	"github.com/xuri/excelize/v2"
)

func GenerateExcel(orders []models.Order) ([]byte, error) {
	// Create a new Excel file
	f := excelize.NewFile()

	// Create a new sheet and set it as active
	index, err := f.NewSheet("Orders")
	if err != nil {
		return nil, fmt.Errorf("failed to create Excel file: %v", err)
	}

	// Add headers to the sheet
	f.SetCellValue("Orders", "A1", "Nom Complet")
	f.SetCellValue("Orders", "B1", "Téléphone 1")
	f.SetCellValue("Orders", "C1", "Téléphone 2")
	f.SetCellValue("Orders", "D1", "Produit")
	f.SetCellValue("Orders", "E1", "Qauntité")
	f.SetCellValue("Orders", "F1", "Adresse")
	f.SetCellValue("Orders", "G1", "Wilaya")
	f.SetCellValue("Orders", "H1", "Commune")
	f.SetCellValue("Orders", "I1", "Total à ramasser")
	f.SetCellValue("Orders", "J1", "Note")
	f.SetCellValue("Orders", "K1", "ID")
	f.SetCellValue("Orders", "L1", "Echange")
	f.SetCellValue("Orders", "M1", "Stopdesk")

	// Loop through the Orders and add data to the sheet
	for i, order := range orders {
		// Convert to row index 2 and onward
		row := i + 2

		f.SetCellValue("Orders", fmt.Sprintf("A%d", row), order.FullName)
		f.SetCellValue("Orders", fmt.Sprintf("B%d", row), order.PhoneNumber)
		f.SetCellValue("Orders", fmt.Sprintf("C%d", row), "")
		f.SetCellValue("Orders", fmt.Sprintf("D%d", row), order.ProductName)
		f.SetCellValue("Orders", fmt.Sprintf("E%d", row), order.Quantity)
		f.SetCellValue("Orders", fmt.Sprintf("F%d", row), "")
		f.SetCellValue("Orders", fmt.Sprintf("G%d", row), order.StateNumber)
		f.SetCellValue("Orders", fmt.Sprintf("H%d", row), order.City)
		f.SetCellValue("Orders", fmt.Sprintf("I%d", row), int(order.TotalPrice))
		f.SetCellValue("Orders", fmt.Sprintf("J%d", row), "Autorisation d'ouvrir")
		f.SetCellValue("Orders", fmt.Sprintf("K%d", row), "")
		f.SetCellValue("Orders", fmt.Sprintf("L%d", row), "")
		shippingMethod := ""
		if order.ShippingMethod == "Stopdesk" {
			shippingMethod = "OUI"
		}
		f.SetCellValue("Orders", fmt.Sprintf("M%d", row), shippingMethod)
	}

	// Set the active sheet
	f.SetActiveSheet(index)

	// Write the file to a buffer
	buffer, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("failed to create Excel file: %v", err)
	}

	// Return the Excel file content as a byte slice
	return buffer.Bytes(), nil
}
