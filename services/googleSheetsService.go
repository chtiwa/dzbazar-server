package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
	"gorm.io/gorm"
)

const (
	SheetKindOrders    = "orders"
	SheetKindAbandoned = "abandoned"
)

var (
	ErrSheetsNotConfigured  = errors.New("google sheets service account not configured")
	ErrSheetsBadSpreadsheet = errors.New("missing spreadsheet")
	ErrSheetsBadColumns     = errors.New("invalid columns")
	ErrSheetsBadCredentials = errors.New("invalid service account credentials")
	ErrSheetsSameTab        = errors.New("same spreadsheet tab already used by the other sheet")
	errSheetsBadKind        = errors.New("invalid sheet kind")
)

type orderCol struct {
	Key, Header string
	Val         func(*models.Order) any
}

type leadCol struct {
	Key, Header string
	Val         func(*models.AbandonedLead) any
}

func orderProducts(o *models.Order) string {
	parts := make([]string, 0, len(o.Items))
	for _, item := range o.Items {
		name := item.Product.Title
		if name == "" {
			name = "Produit"
		}
		if combo := item.ProductVariantCombination.CombinationString; combo != "" {
			parts = append(parts, fmt.Sprintf("%s (%s) x%d", name, combo, item.Quantity))
		} else {
			parts = append(parts, fmt.Sprintf("%s x%d", name, item.Quantity))
		}
	}
	return strings.Join(parts, "; ")
}

// OrderSheetColumns: slice order = default order.
var OrderSheetColumns = []orderCol{
	{"id", "ID commande", func(o *models.Order) any { return o.ID.String() }},
	{"date", "Date", func(o *models.Order) any { return o.CreatedAt.Format("2006-01-02 15:04:05") }},
	{"name", "Nom", func(o *models.Order) any { return o.Client.FullName }},
	{"phone", "Téléphone", func(o *models.Order) any { return o.Client.PhoneNumber }},
	{"phone2", "Téléphone 2", func(o *models.Order) any { return o.Client.PhoneNumber2 }},
	{"wilaya", "Wilaya", func(o *models.Order) any { return o.Client.State }},
	{"commune", "Commune", func(o *models.Order) any { return o.Client.City }},
	{"stopdesk", "Point stop desk", func(o *models.Order) any { return o.Client.StopdeskPoint }},
	{"shipping_method", "Livraison", func(o *models.Order) any { return o.ShippingMethod }},
	{"products", "Produits", func(o *models.Order) any { return orderProducts(o) }},
	{"quantity", "Quantité", func(o *models.Order) any {
		n := 0
		for _, it := range o.Items {
			n += int(it.Quantity)
		}
		return n
	}},
	{"shipping_price", "Prix livraison", func(o *models.Order) any { return o.ShippingPrice }},
	{"discount", "Remise", func(o *models.Order) any { return o.DiscountAmount }},
	{"total", "Total", func(o *models.Order) any { return o.TotalPrice }},
	{"status", "Statut", func(o *models.Order) any { return o.Status }},
	{"note", "Note", func(o *models.Order) any { return o.Note }},
	{"source", "Source", func(o *models.Order) any { return o.ConversionSource }},
	{"page_url", "Page", func(o *models.Order) any { return o.PageURL }},
}

// AbandonedSheetColumns match the abandoned-leads Excel export exactly.
var AbandonedSheetColumns = []leadCol{
	{"date", "Date", func(l *models.AbandonedLead) any { return l.CreatedAt.Format("2006-01-02 15:04") }},
	{"source", "Source", func(l *models.AbandonedLead) any { return l.ConversionSource }},
	{"name", "Nom", func(l *models.AbandonedLead) any { return l.FullName }},
	{"phone", "Téléphone", func(l *models.AbandonedLead) any { return l.PhoneNumber }},
	{"wilaya", "Wilaya", func(l *models.AbandonedLead) any { return l.State }},
	{"commune", "Commune", func(l *models.AbandonedLead) any { return l.City }},
	{"product", "Produit", func(l *models.AbandonedLead) any { return l.ProductTitle }},
	{"variant", "Variante", func(l *models.AbandonedLead) any { return l.CombinationStr }},
	{"quantity", "Quantité", func(l *models.AbandonedLead) any { return l.Quantity }},
	{"price", "Prix", func(l *models.AbandonedLead) any { return l.Price }},
	{"shipping_method", "Méthode", func(l *models.AbandonedLead) any { return l.ShippingMethod }},
}

var defaultOrderKeys = []string{"id", "date", "name", "phone", "wilaya", "commune", "stopdesk", "products", "total", "status"}

// SheetCatalog lists every available column (default headers) for a kind;
// nil for an unknown kind.
func SheetCatalog(kind string) []models.SheetColumn {
	var out []models.SheetColumn
	switch kind {
	case SheetKindOrders:
		for _, c := range OrderSheetColumns {
			out = append(out, models.SheetColumn{Key: c.Key, Header: c.Header})
		}
	case SheetKindAbandoned:
		for _, c := range AbandonedSheetColumns {
			out = append(out, models.SheetColumn{Key: c.Key, Header: c.Header})
		}
	}
	return out
}

func defaultSelection(kind string) []models.SheetColumn {
	cat := SheetCatalog(kind)
	if kind != SheetKindOrders {
		return cat
	}
	var out []models.SheetColumn
	for _, k := range defaultOrderKeys {
		for _, c := range cat {
			if c.Key == k {
				out = append(out, c)
			}
		}
	}
	return out
}

func SheetHeaders(cols []models.SheetColumn) []any {
	out := make([]any, len(cols))
	for i, c := range cols {
		out[i] = c.Header
	}
	return out
}

func OrderRow(cols []models.SheetColumn, o *models.Order) []any {
	row := make([]any, len(cols))
	for i, c := range cols {
		row[i] = ""
		for _, oc := range OrderSheetColumns {
			if oc.Key == c.Key {
				row[i] = oc.Val(o)
			}
		}
	}
	return row
}

func LeadRow(cols []models.SheetColumn, l *models.AbandonedLead) []any {
	row := make([]any, len(cols))
	for i, c := range cols {
		row[i] = ""
		for _, lc := range AbandonedSheetColumns {
			if lc.Key == c.Key {
				row[i] = lc.Val(l)
			}
		}
	}
	return row
}

// SheetsEmailFor is the client_email of the key this integration uses (its own
// per-shop key, else the platform one), "" if neither is set.
func SheetsEmailFor(integ *models.GoogleSheetsIntegration) string {
	raw := os.Getenv("GOOGLE_SHEETS_SERVICE_ACCOUNT_JSON")
	if integ != nil && integ.ServiceAccountJSON != "" {
		raw = integ.ServiceAccountJSON
	}
	var k struct {
		ClientEmail string `json:"client_email"`
	}
	if json.Unmarshal([]byte(raw), &k) != nil {
		return ""
	}
	return k.ClientEmail
}

func newSheetsClient(serviceAccountJSON string) (*sheets.Service, error) {
	ctx := context.Background()
	creds, err := google.CredentialsFromJSON(ctx, []byte(serviceAccountJSON), sheets.SpreadsheetsScope)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSheetsBadCredentials, err)
	}
	return sheets.NewService(ctx, option.WithCredentials(creds))
}

var platformClient = sync.OnceValues(func() (*sheets.Service, error) {
	raw := os.Getenv("GOOGLE_SHEETS_SERVICE_ACCOUNT_JSON")
	if raw == "" {
		return nil, ErrSheetsNotConfigured
	}
	return newSheetsClient(raw)
})

// SheetsClientFor uses the integration's legacy per-shop key if stored,
// else the cached platform client.
func SheetsClientFor(integ *models.GoogleSheetsIntegration) (*sheets.Service, error) {
	if integ.ServiceAccountJSON != "" {
		return newSheetsClient(integ.ServiceAccountJSON)
	}
	return platformClient()
}

var spreadsheetIDRe = regexp.MustCompile(`/d/([A-Za-z0-9_-]+)`)

func ParseSpreadsheetID(s string) string {
	s = strings.TrimSpace(s)
	if m := spreadsheetIDRe.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return s
}

func tabRange(tab, cells string) string {
	return "'" + strings.ReplaceAll(tab, "'", "''") + "'!" + cells
}

// FirstTabTitle doubles as the access check.
func FirstTabTitle(svc *sheets.Service, id string) (string, error) {
	resp, err := svc.Spreadsheets.Get(id).Fields("sheets.properties.title").Do()
	if err != nil {
		return "", err
	}
	if len(resp.Sheets) == 0 {
		return "", errors.New("spreadsheet has no tabs")
	}
	return resp.Sheets[0].Properties.Title, nil
}

// WriteHeaderRow writes row 1 if A1 is empty or overwrite is set.
func WriteHeaderRow(svc *sheets.Service, id, tab string, headers []any, overwrite bool) error {
	resp, err := svc.Spreadsheets.Values.Get(id, tabRange(tab, "A1:A1")).Do()
	if err != nil {
		return err
	}
	if len(resp.Values) > 0 && !overwrite {
		return nil
	}
	_, err = svc.Spreadsheets.Values.Update(id, tabRange(tab, "A1"), &sheets.ValueRange{
		Values: [][]any{headers},
	}).ValueInputOption("RAW").Do()
	return err
}

func AppendSheetRow(svc *sheets.Service, id, tab string, row []any) error {
	_, err := svc.Spreadsheets.Values.Append(id, tabRange(tab, "A:A"), &sheets.ValueRange{
		Values: [][]any{row},
	}).ValueInputOption("RAW").Do()
	return err
}

// UpdateSheetCellByKey writes value into valCol of the row whose keyCol equals key (0-based cols).
// ponytail: reads whole column per status change; batch per shop if 429s appear
func UpdateSheetCellByKey(svc *sheets.Service, id, tab string, keyCol, valCol int, key string, value any) (bool, error) {
	kc, err := excelize.ColumnNumberToName(keyCol + 1)
	if err != nil {
		return false, err
	}
	vc, err := excelize.ColumnNumberToName(valCol + 1)
	if err != nil {
		return false, err
	}
	resp, err := svc.Spreadsheets.Values.Get(id, tabRange(tab, kc+":"+kc)).Do()
	if err != nil {
		return false, err
	}
	for i, r := range resp.Values {
		if len(r) > 0 && fmt.Sprint(r[0]) == key {
			_, err = svc.Spreadsheets.Values.Update(id, tabRange(tab, fmt.Sprintf("%s%d", vc, i+1)), &sheets.ValueRange{
				Values: [][]any{{value}},
			}).ValueInputOption("RAW").Do()
			return true, err
		}
	}
	return false, nil
}

// SheetsErrorCode maps any error to a short code the admin translates.
func SheetsErrorCode(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrSheetsNotConfigured):
		return "not_configured"
	case errors.Is(err, ErrSheetsBadSpreadsheet):
		return "bad_spreadsheet"
	case errors.Is(err, ErrSheetsBadColumns):
		return "bad_columns"
	case errors.Is(err, ErrSheetsBadCredentials):
		return "bad_credentials"
	case errors.Is(err, ErrSheetsSameTab):
		return "same_tab"
	}
	var gerr *googleapi.Error
	if errors.As(err, &gerr) {
		switch {
		case gerr.Code == 403 && strings.Contains(gerr.Error(), "accessNotConfigured"):
			return "api_disabled"
		case gerr.Code == 403 || gerr.Code == 404:
			return "no_access"
		case gerr.Code == 400 && strings.Contains(gerr.Error(), "Unable to parse range"):
			return "tab_not_found"
		case gerr.Code == 429:
			return "quota"
		}
	}
	var nerr net.Error
	msg := err.Error()
	if errors.As(err, &nerr) || strings.Contains(msg, "dial tcp") || strings.Contains(msg, "cannot fetch token") {
		return "network"
	}
	return "unknown"
}

type SheetIntegrationInput struct {
	Spreadsheet string               `json:"spreadsheet"`
	SheetName   string               `json:"sheetName"`
	Columns     []models.SheetColumn `json:"columns"`
	IsActive    *bool                `json:"isActive"`
	// Optional per-shop service-account key; blank keeps the stored/platform one.
	ServiceAccountJSON string `json:"serviceAccountJson"`
}

func normalizeColumns(kind string, in []models.SheetColumn) ([]models.SheetColumn, error) {
	if len(in) == 0 {
		return defaultSelection(kind), nil
	}
	cat := SheetCatalog(kind)
	seen := map[string]bool{}
	out := make([]models.SheetColumn, 0, len(in))
	for _, c := range in {
		i := slices.IndexFunc(cat, func(x models.SheetColumn) bool { return x.Key == c.Key })
		if i < 0 || seen[c.Key] {
			return nil, ErrSheetsBadColumns
		}
		seen[c.Key] = true
		h := []rune(strings.TrimSpace(c.Header))
		if len(h) == 0 {
			h = []rune(cat[i].Header)
		}
		out = append(out, models.SheetColumn{Key: c.Key, Header: string(h[:min(len(h), 100)])})
	}
	return out, nil
}

func SaveSheetIntegration(shopID uuid.UUID, kind string, in SheetIntegrationInput) (models.GoogleSheetsIntegration, error) {
	var integ models.GoogleSheetsIntegration
	if SheetCatalog(kind) == nil {
		return integ, errSheetsBadKind
	}
	err := initializers.DB.Where("shop_id = ? AND kind = ?", shopID, kind).First(&integ).Error
	existing := err == nil
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return integ, err
	}
	if !existing {
		integ = models.GoogleSheetsIntegration{ShopID: shopID, Kind: kind, IsActive: true}
	}

	id := ParseSpreadsheetID(in.Spreadsheet)
	if id == "" {
		return integ, ErrSheetsBadSpreadsheet
	}
	cols, err := normalizeColumns(kind, in.Columns)
	if err != nil {
		return integ, err
	}
	tab := strings.TrimSpace(in.SheetName)

	if key := strings.TrimSpace(in.ServiceAccountJSON); key != "" {
		integ.ServiceAccountJSON = key
	}
	svc, err := SheetsClientFor(&integ)
	if err != nil {
		return integ, err
	}
	if tab == "" {
		if tab, err = FirstTabTitle(svc, id); err != nil {
			return integ, err
		}
	}

	otherKind := SheetKindOrders
	if kind == SheetKindOrders {
		otherKind = SheetKindAbandoned
	}
	var other models.GoogleSheetsIntegration
	if initializers.DB.Where("shop_id = ? AND kind = ? AND spreadsheet_id = ? AND sheet_name = ?", shopID, otherKind, id, tab).
		First(&other).Error == nil {
		return integ, ErrSheetsSameTab
	}

	// Never overwrite row 1 of a freshly connected sheet; only rewrite when
	// this same sheet's columns/headers changed.
	overwrite := existing && integ.SpreadsheetID == id && integ.SheetName == tab && !slices.Equal(integ.Columns, cols)
	if err := WriteHeaderRow(svc, id, tab, SheetHeaders(cols), overwrite); err != nil {
		return integ, err
	}

	integ.SpreadsheetID, integ.SheetName, integ.Columns = id, tab, cols
	if in.IsActive != nil {
		integ.IsActive = *in.IsActive
	}
	integ.LastError = ""
	return integ, initializers.DB.Save(&integ).Error
}
