package services

import "gorm.io/gorm"

// PlatformTrafficWindowDays is the lookback window for the super-admin
// dashboard's traffic KPIs — matches the merchant-side GetVisits default.
const PlatformTrafficWindowDays = 30

// PlatformTrafficKPIs returns platform-wide (cross-tenant) traffic aggregates
// for the super-admin dashboard: unique visitors and page views across every
// shop over the last PlatformTrafficWindowDays days. shop_visits/page_visits
// are already deduped one-row-per-unique-visitor-per-day(-per-page) by their
// storefront beacons (see models/ShopVisit.go), so a plain COUNT here is
// already a unique count — COUNT(DISTINCT ...) is only needed for visitors
// since the same visitor can show up on multiple days within the window.
func PlatformTrafficKPIs(db *gorm.DB) (uniqueVisitors int64, pageViews int64, err error) {
	if err = db.Table("shop_visits").
		Where("day > CURRENT_DATE - ?::int", PlatformTrafficWindowDays).
		Select("COUNT(DISTINCT (shop_id, visitor_id))").
		Row().Scan(&uniqueVisitors); err != nil {
		return 0, 0, err
	}

	if err = db.Table("page_visits").
		Where("day > CURRENT_DATE - ?::int", PlatformTrafficWindowDays).
		Count(&pageViews).Error; err != nil {
		return 0, 0, err
	}

	return uniqueVisitors, pageViews, nil
}
