package services

import (
	"testing"

	"github.com/chtiwa/dzbazar-server/models"
)

func TestSheetsClientFor_InvalidJSON(t *testing.T) {
	_, err := SheetsClientFor(&models.GoogleSheetsIntegration{ServiceAccountJSON: "not valid json"})
	if err == nil {
		t.Fatal("expected error for invalid service account JSON, got nil")
	}
}

func TestParseSpreadsheetID(t *testing.T) {
	for in, want := range map[string]string{
		"https://docs.google.com/spreadsheets/d/1aB_c-D/edit#gid=0": "1aB_c-D",
		"  1aB_c-D ": "1aB_c-D",
	} {
		if got := ParseSpreadsheetID(in); got != want {
			t.Errorf("ParseSpreadsheetID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOrderRowFollowsColumns(t *testing.T) {
	o := &models.Order{Status: "Confirmé", Client: models.Client{PhoneNumber: "0555123456"}}
	row := OrderRow([]models.SheetColumn{{Key: "status"}, {Key: "phone"}, {Key: "nope"}}, o)
	if row[0] != "Confirmé" || row[1] != "0555123456" || row[2] != "" {
		t.Fatalf("unexpected row %#v", row)
	}
}

func TestEveryCatalogKeyHasValue(t *testing.T) {
	for _, c := range OrderSheetColumns {
		if c.Val == nil {
			t.Errorf("order column %s has no value fn", c.Key)
		}
	}
	for _, c := range AbandonedSheetColumns {
		if c.Val == nil {
			t.Errorf("abandoned column %s has no value fn", c.Key)
		}
	}
	if len(defaultSelection(SheetKindOrders)) != 10 {
		t.Error("default orders selection should have 10 columns")
	}
}
