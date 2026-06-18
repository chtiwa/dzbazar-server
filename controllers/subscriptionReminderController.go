package controllers

import (
	"log"
	"time"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/utils"
)

// ── Subscription expiry reminders ───────────────────────────────────────────
// There is no payment gateway wired up yet, so plan expiry is enforced purely
// by the expires_at date a super admin sets manually. To warn a shop owner
// before their access lapses, this ticker periodically scans for
// subscriptions expiring within reminderWindowDays and emails the owner once.
//   - The reminder is sent at most once per expiry: ExpiryReminderSentAt is
//     stamped after a successful send, and the query excludes already-sent
//     rows. Any change to ExpiresAt (owner re-subscribes, admin edits it)
//     resets ExpiryReminderSentAt to nil so the warning re-arms.
//   - If the email send fails, the row is left unstamped so the next tick
//     retries it.
//   - Runs once immediately on boot (so a restart doesn't wait a full
//     interval before the first scan), then on a slow ticker thereafter.

const (
	subscriptionReminderInterval = 12 * time.Hour
	reminderWindowDays           = 3
)

// StartSubscriptionExpiryReminders runs forever, periodically emailing shop
// owners whose subscription is about to expire. Intended to be run in its
// own goroutine.
func StartSubscriptionExpiryReminders() {
	sendExpiryReminders() // run once immediately on boot, don't wait 12h

	ticker := time.NewTicker(subscriptionReminderInterval)
	defer ticker.Stop()

	for range ticker.C {
		sendExpiryReminders()
	}
}

// sendExpiryReminders finds subscriptions expiring within the reminder
// window that haven't been warned about yet, and emails each shop owner.
func sendExpiryReminders() {
	now := time.Now()
	windowEnd := now.AddDate(0, 0, reminderWindowDays)

	var subs []models.ShopSubscription
	if err := initializers.DB.
		Preload("Plan").
		Where("expires_at IS NOT NULL").
		Where("expires_at > ?", now).
		Where("expires_at <= ?", windowEnd).
		Where("expiry_reminder_sent_at IS NULL").
		Find(&subs).Error; err != nil {
		log.Printf("subscription reminder: failed to list expiring subscriptions: %v", err)
		return
	}

	for _, sub := range subs {
		var shop models.Shop
		if err := initializers.DB.Preload("Owner").First(&shop, "id = ?", sub.ShopID).Error; err != nil {
			log.Printf("subscription reminder: failed to load shop %s: %v", sub.ShopID, err)
			continue
		}

		if shop.Owner.Email == "" {
			continue
		}

		if err := utils.SendPlanExpiryEmail(shop.Owner.Email, shop.Name, sub.Plan.Name, *sub.ExpiresAt); err != nil {
			log.Printf("subscription reminder: failed to email shop %s owner: %v", sub.ShopID, err)
			continue // don't stamp — retry next tick
		}

		initializers.DB.Model(&models.ShopSubscription{}).Where("id = ?", sub.ID).Update("expiry_reminder_sent_at", time.Now())
	}
}
