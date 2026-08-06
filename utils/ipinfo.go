package utils

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

// IPPrivacy mirrors ipinfo.io's /privacy response fields.
type IPPrivacy struct {
	VPN     bool `json:"vpn"`
	Proxy   bool `json:"proxy"`
	Tor     bool `json:"tor"`
	Relay   bool `json:"relay"`
	Hosting bool `json:"hosting"`
}

// LookupIPPrivacy calls ipinfo.io's privacy endpoint for a single IP. Returns
// an error on missing token, network failure, or a non-200 response — callers
// must treat any error as "no signal" (fail open) rather than a ban.
func LookupIPPrivacy(ip string) (IPPrivacy, error) {
	token := os.Getenv("IPINFO_TOKEN")
	if token == "" {
		return IPPrivacy{}, fmt.Errorf("IPINFO_TOKEN not set")
	}

	url := fmt.Sprintf("https://ipinfo.io/%s/privacy?token=%s", ip, token)
	client := &http.Client{Timeout: 2 * time.Second}

	resp, err := client.Get(url)
	if err != nil {
		return IPPrivacy{}, fmt.Errorf("ipinfo request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return IPPrivacy{}, fmt.Errorf("ipinfo API error: status %d", resp.StatusCode)
	}

	var priv IPPrivacy
	if err := json.NewDecoder(resp.Body).Decode(&priv); err != nil {
		return IPPrivacy{}, fmt.Errorf("failed to decode ipinfo response: %v", err)
	}

	return priv, nil
}
