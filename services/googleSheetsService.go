package services

import (
	"context"
	"fmt"
	"strings"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

// SheetsHeaderRow is the fixed column schema for every shop's exported
// orders sheet. v1 does not support merchant-configurable columns — YAGNI,
// see docs/superpowers/specs/2026-09-07-google-sheets-export-design.md.
var SheetsHeaderRow = []interface{}{
	"Order ID", "Date", "Customer Name", "Phone", "Wilaya", "City",
	"Stopdesk Point", "Products", "Total", "Status",
}

// NewSheetsClient builds an authenticated Sheets API client from a pasted
// Google service-account JSON key. Service accounts self-authenticate via
// JWT — no user consent/OAuth redirect flow needed, unlike a normal Google
// login integration.
func NewSheetsClient(serviceAccountJSON string) (*sheets.Service, error) {
	ctx := context.Background()
	creds, err := google.CredentialsFromJSON(ctx, []byte(serviceAccountJSON), sheets.SpreadsheetsScope)
	if err != nil {
		return nil, fmt.Errorf("invalid service account credentials: %w", err)
	}
	svc, err := sheets.NewService(ctx, option.WithCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("failed to build sheets client: %w", err)
	}
	return svc, nil
}

// EnsureHeaderRow writes the fixed header row to row 1 of the sheet if it's
// currently empty. Called on connect (as a live credential/access check —
// it fails loudly if the key is invalid or the sheet isn't shared with the
// service account) and by the "test connection" action.
func EnsureHeaderRow(svc *sheets.Service, spreadsheetID, sheetName string) error {
	readRange := fmt.Sprintf("%s!A1:A1", sheetName)
	resp, err := svc.Spreadsheets.Values.Get(spreadsheetID, readRange).Do()
	if err != nil {
		if strings.Contains(err.Error(), "accessNotConfigured") {
			return fmt.Errorf("Google Sheets API is not enabled on this service account's Google Cloud project: %w", err)
		}
		if strings.Contains(err.Error(), "dial tcp") || strings.Contains(err.Error(), "cannot fetch token") {
			return fmt.Errorf("network error reaching Google (retry in a moment): %w", err)
		}
		if strings.Contains(err.Error(), "Unable to parse range") {
			return fmt.Errorf("tab %q not found in the sheet (check the tab name at the bottom of the spreadsheet, not the sheet's title): %w", sheetName, err)
		}
		return fmt.Errorf("failed to read sheet (check spreadsheet ID and that it's shared with the service account email): %w", err)
	}
	if len(resp.Values) > 0 {
		return nil
	}
	writeRange := fmt.Sprintf("%s!A1", sheetName)
	_, err = svc.Spreadsheets.Values.Update(spreadsheetID, writeRange, &sheets.ValueRange{
		Values: [][]interface{}{SheetsHeaderRow},
	}).ValueInputOption("USER_ENTERED").Do()
	if err != nil {
		return fmt.Errorf("failed to write header row: %w", err)
	}
	return nil
}

// AppendOrderRow appends one row to the end of the sheet.
func AppendOrderRow(svc *sheets.Service, spreadsheetID, sheetName string, row []interface{}) error {
	appendRange := fmt.Sprintf("%s!A:A", sheetName)
	_, err := svc.Spreadsheets.Values.Append(spreadsheetID, appendRange, &sheets.ValueRange{
		Values: [][]interface{}{row},
	}).ValueInputOption("USER_ENTERED").Do()
	if err != nil {
		return fmt.Errorf("failed to append row: %w", err)
	}
	return nil
}
