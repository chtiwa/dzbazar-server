package controllers

import (
	"strings"
)

// isBureauEligibleCarrier reports whether a carrier, identified by its
// AvailableDeliveryCompany.Name, is allowed to own bureau rows.
//
// ZR Express is the one exclusion: it resolves stopdesk hubs live via its own
// API (resolveZrHubID in zrGeoController.go), so a typed bureau list for ZR
// would be dead data that silently diverges from the real hub network. The
// case-insensitive substring match mirrors the carrier filter already used in
// admin/src/pages/orders/BatchShipModal.tsx, since the catalog stores display
// names ("ZR Express") rather than stable slugs.
func isBureauEligibleCarrier(availableCompanyName string) bool {
	name := strings.ToLower(strings.TrimSpace(availableCompanyName))
	if name == "" {
		return false
	}
	return !strings.Contains(name, "zr")
}

// normalizeBureauName trims a typed desk name and collapses internal
// whitespace runs, so "OUM EL   BOUGHI" and "OUM EL BOUGHI" collide on the
// unique index instead of both landing in the dropdown.
func normalizeBureauName(raw string) string {
	return strings.Join(strings.Fields(raw), " ")
}
