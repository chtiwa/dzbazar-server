package services

import (
	"testing"

	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/utils"
)

func TestFraudHiddenReason(t *testing.T) {
	allEnabled := models.Shop{BanIncognitoEnabled: true, BanVpnEnabled: true, BanDatacenterEnabled: true}
	allDisabled := models.Shop{}

	t.Run("all toggles off never flags, even with every signal present", func(t *testing.T) {
		priv := utils.IPPrivacy{VPN: true, Proxy: true, Tor: true, Hosting: true}
		if reason := FraudHiddenReason(allDisabled, true, priv); reason != "" {
			t.Errorf("got %q, want \"\" — disabled toggles must never flag", reason)
		}
	})

	t.Run("fail-open signal (empty IPPrivacy) never flags even with toggles on", func(t *testing.T) {
		if reason := FraudHiddenReason(allEnabled, false, utils.IPPrivacy{}); reason != "" {
			t.Errorf("got %q, want \"\" — a failed/empty lookup must never ban", reason)
		}
	})

	t.Run("incognito flags only when its own toggle is on", func(t *testing.T) {
		shop := models.Shop{BanIncognitoEnabled: true}
		if reason := FraudHiddenReason(shop, true, utils.IPPrivacy{}); reason != HiddenReasonIncognito {
			t.Errorf("got %q, want %q", reason, HiddenReasonIncognito)
		}
		if reason := FraudHiddenReason(allDisabled, true, utils.IPPrivacy{}); reason != "" {
			t.Errorf("got %q, want \"\" — incognito toggle off", reason)
		}
	})

	t.Run("vpn/proxy/tor all map to the vpn reason when toggle is on", func(t *testing.T) {
		shop := models.Shop{BanVpnEnabled: true}
		for name, priv := range map[string]utils.IPPrivacy{
			"vpn":   {VPN: true},
			"proxy": {Proxy: true},
			"tor":   {Tor: true},
		} {
			if reason := FraudHiddenReason(shop, false, priv); reason != HiddenReasonVpn {
				t.Errorf("%s: got %q, want %q", name, reason, HiddenReasonVpn)
			}
		}
	})

	t.Run("relay (iCloud Private Relay) never triggers the vpn reason", func(t *testing.T) {
		shop := models.Shop{BanVpnEnabled: true}
		if reason := FraudHiddenReason(shop, false, utils.IPPrivacy{Relay: true}); reason != "" {
			t.Errorf("got %q, want \"\" — relay must be excluded, legitimate iPhone customers use it", reason)
		}
	})

	t.Run("hosting flags datacenter reason only when its toggle is on", func(t *testing.T) {
		shop := models.Shop{BanDatacenterEnabled: true}
		if reason := FraudHiddenReason(shop, false, utils.IPPrivacy{Hosting: true}); reason != HiddenReasonDatacenter {
			t.Errorf("got %q, want %q", reason, HiddenReasonDatacenter)
		}
		if reason := FraudHiddenReason(allDisabled, false, utils.IPPrivacy{Hosting: true}); reason != "" {
			t.Errorf("got %q, want \"\" — datacenter toggle off", reason)
		}
	})

	t.Run("datacenter wins over vpn and incognito when all toggles on and all signals present", func(t *testing.T) {
		priv := utils.IPPrivacy{VPN: true, Hosting: true}
		if reason := FraudHiddenReason(allEnabled, true, priv); reason != HiddenReasonDatacenter {
			t.Errorf("got %q, want %q — datacenter is the highest-priority reason", reason, HiddenReasonDatacenter)
		}
	})

	t.Run("vpn wins over incognito when both toggles on and both signals present", func(t *testing.T) {
		if reason := FraudHiddenReason(allEnabled, true, utils.IPPrivacy{VPN: true}); reason != HiddenReasonVpn {
			t.Errorf("got %q, want %q", reason, HiddenReasonVpn)
		}
	})
}
