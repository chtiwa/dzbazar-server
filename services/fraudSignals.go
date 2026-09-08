package services

import (
	"encoding/json"
	"net"
	"time"

	"github.com/chtiwa/dzbazar-server/initializers"
	"github.com/chtiwa/dzbazar-server/models"
	"github.com/chtiwa/dzbazar-server/utils"
)

const (
	HiddenReasonCussword     = "cussword"
	HiddenReasonBannedClient = "banned_client"
	HiddenReasonIncognito    = "incognito"
	HiddenReasonVpn          = "vpn"
	HiddenReasonDatacenter   = "datacenter"
	HiddenReasonRateLimited  = "rate_limited"
)

// FraudHiddenReason decides whether an order should be shadow-hidden based on
// this shop's enabled toggles and the detected signals. Priority: datacenter
// > vpn/proxy/tor > incognito — a hosting IP is the strongest signal, so it
// wins when multiple are present. iCloud Private Relay (priv.Relay) is
// deliberately excluded from the vpn bucket: it's used by legitimate paying
// iPhone customers, not just fraud.
func FraudHiddenReason(shop models.Shop, incognito bool, priv utils.IPPrivacy) string {
	if shop.BanDatacenterEnabled && priv.Hosting {
		return HiddenReasonDatacenter
	}
	if shop.BanVpnEnabled && (priv.VPN || priv.Proxy || priv.Tor) {
		return HiddenReasonVpn
	}
	if shop.BanIncognitoEnabled && incognito {
		return HiddenReasonIncognito
	}
	return ""
}

const ipIntelCacheTTL = 7 * 24 * time.Hour

func ipIntelCacheKey(ip string) string {
	return "ipintel:" + ip
}

// ClassifyIP resolves an IP's vpn/proxy/hosting signal via ipinfo.io, cached
// in Redis per IP so a repeat-order storm from one IP burns one quota unit.
// Private/loopback IPs (dev, staff on office LAN) are never sent externally.
// Any failure — cache, network, or missing token — returns a zero-value
// IPPrivacy and an error; callers must fail open on error, never ban.
func ClassifyIP(ip string) (utils.IPPrivacy, error) {
	parsed := net.ParseIP(ip)
	if parsed == nil || parsed.IsPrivate() || parsed.IsLoopback() {
		return utils.IPPrivacy{}, nil
	}

	key := ipIntelCacheKey(ip)
	if cached, err := initializers.RClient.Get(initializers.Ctx, key).Bytes(); err == nil {
		var priv utils.IPPrivacy
		if json.Unmarshal(cached, &priv) == nil {
			return priv, nil
		}
	}

	priv, err := utils.LookupIPPrivacy(ip)
	if err != nil {
		return utils.IPPrivacy{}, err
	}

	if encoded, err := json.Marshal(priv); err == nil {
		initializers.RClient.Set(initializers.Ctx, key, encoded, ipIntelCacheTTL)
	}

	return priv, nil
}
