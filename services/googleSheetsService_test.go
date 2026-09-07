package services

import "testing"

func TestNewSheetsClient_InvalidJSON(t *testing.T) {
	_, err := NewSheetsClient("not valid json")
	if err == nil {
		t.Fatal("expected error for invalid service account JSON, got nil")
	}
}

func TestSheetsHeaderRow_Shape(t *testing.T) {
	want := []interface{}{"Order ID", "Date", "Customer Name", "Phone", "Wilaya", "City", "Stopdesk Point", "Products", "Total", "Status"}
	if len(SheetsHeaderRow) != len(want) {
		t.Fatalf("expected %d header columns, got %d", len(want), len(SheetsHeaderRow))
	}
	for i, col := range want {
		if SheetsHeaderRow[i] != col {
			t.Errorf("header[%d] = %v, want %v", i, SheetsHeaderRow[i], col)
		}
	}
}
