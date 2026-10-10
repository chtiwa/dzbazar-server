package services

import (
	"fmt"

	"github.com/chtiwa/dzbazar-server/models"
	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

// ponytail: unbounded in-memory export; stream rows (excelize StreamWriter) if a shop exceeds ~50k leads
func ExportAbandonedLeadsExcel(db *gorm.DB, shopID uuid.UUID) ([]byte, error) {
	var leads []models.AbandonedLead
	if err := db.Where("shop_id = ?", shopID).Order("created_at DESC").Find(&leads).Error; err != nil {
		return nil, err
	}

	f := excelize.NewFile()
	sheetName := "Abandonnées"
	index, err := f.NewSheet(sheetName)
	if err != nil {
		return nil, fmt.Errorf("failed to create Excel file: %v", err)
	}
	f.DeleteSheet("Sheet1")

	for i, c := range AbandonedSheetColumns {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheetName, cell, c.Header)
	}

	for i := range leads {
		for j, c := range AbandonedSheetColumns {
			cell, _ := excelize.CoordinatesToCellName(j+1, i+2)
			f.SetCellValue(sheetName, cell, c.Val(&leads[i]))
		}
	}

	f.SetActiveSheet(index)
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("failed to write Excel file: %v", err)
	}
	return buf.Bytes(), nil
}
