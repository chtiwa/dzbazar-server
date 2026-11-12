package utils

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode"
)

type FacebookEvent struct {
	EventName      string                 `json:"event_name"`
	EventTime      int64                  `json:"event_time"`
	EventId        string                 `json:"event_id"`
	EventSourceURL string                 `json:"event_source_url,omitempty"`
	ActionSource   string                 `json:"action_source"`
	UserData       map[string]interface{} `json:"user_data"`
	CustomData     map[string]interface{} `json:"custom_data"`
}

type FacebookPayload struct {
	Data          []FacebookEvent `json:"data"`
	TestEventCode string          `json:"test_event_code,omitempty"`
}

// SendFacebookPurchase fires a confirmed purchase event via Conversion API,
// using the shop's own pixel ID and access token (each shop configures its own).
func SendFacebookPurchase(pixelID, accessToken, orderID, fullName, phone string, value float64, currency, fbc, fbp string, createdAt time.Time, clientUserAgent, clientIP, testCode, eventSourceURL, city, state, externalID string) error {
	if pixelID == "" || accessToken == "" {
		return fmt.Errorf("missing pixel ID or access token")
	}
	// Meta rejects Purchase without event_source_url (old/dashboard orders have none).
	// ponytail: not per-shop, add if shops get custom domains.
	if eventSourceURL == "" {
		eventSourceURL = os.Getenv("CLIENT_URL")
	}

	url := fmt.Sprintf("https://graph.facebook.com/v24.0/%s/events?access_token=%s", pixelID, accessToken)

	// Hash phone (SHA256) — Meta requires digits-only, country code, no
	// leading 0 (see "conditions de formatage" in Events Manager); local
	// numbers are stored as 05xxxxxxxx, so a raw hash never matches.
	hashedPhone := hashData(normalizePhoneForHashing(phone))

	parts := strings.Split(fullName, " ")
	first := parts[0]
	last := ""
	if len(parts) > 1 {
		last = parts[len(parts)-1]
	}

	// normalize first and last name
	first = strings.ToLower(strings.TrimSpace(first))
	last = strings.ToLower(strings.TrimSpace(last))

	// Include fbp and fbc in user_data. Meta flags empty-string hashed
	// values as malformed — omit a field entirely rather than send "".
	userData := map[string]interface{}{
		"client_user_agent": clientUserAgent,
		"client_ip_address": clientIP,
	}
	if fbp != "" {
		userData["fbp"] = fbp
	}
	if fbc != "" {
		userData["fbc"] = fbc
	}
	if hashedFirstName := hashData(first); hashedFirstName != "" {
		userData["fn"] = hashedFirstName
	}
	if hashedLastName := hashData(last); hashedLastName != "" {
		userData["ln"] = hashedLastName
	}
	if hashedPhone != "" {
		userData["ph"] = hashedPhone
	}
	userData["country"] = hashData("dz")
	if h := hashData(alnumOnly(city)); h != "" {
		userData["ct"] = h
	}
	if h := hashData(alnumOnly(state)); h != "" {
		userData["st"] = h
	}
	if h := hashData(externalID); h != "" {
		userData["external_id"] = h
	}

	customData := map[string]interface{}{
		"currency": currency,
		"value":    value,
		"order_id": orderID,
	}

	event := FacebookEvent{
		EventName:      "Purchase",
		EventTime:      createdAt.UTC().Unix(),
		EventId:        orderID,
		ActionSource:   "website",
		EventSourceURL: eventSourceURL,
		UserData:       userData,
		CustomData:     customData,
	}

	payload := FacebookPayload{
		Data:          []FacebookEvent{event},
		TestEventCode: testCode,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %v", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// Read response body for debugging
	var respBody bytes.Buffer
	respBody.ReadFrom(resp.Body)

	if resp.StatusCode >= 300 {
		return fmt.Errorf("facebook API error: status %d, body: %s", resp.StatusCode, respBody.String())
	}

	fmt.Printf("Facebook API response: status %d, body: %s\n", resp.StatusCode, respBody.String())
	return nil
}

// normalizePhoneForHashing converts a local Algerian number (e.g. "05 54 12 34 56")
// into Meta's required ph format: digits only, country code, no leading 0.
func normalizePhoneForHashing(phone string) string {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, phone)
	digits = strings.TrimPrefix(digits, "0")
	if digits == "" {
		return ""
	}
	if !strings.HasPrefix(digits, "213") {
		digits = "213" + digits
	}
	return digits
}

// alnumOnly strips spaces/punctuation (Meta ct/st format), keeps any-script letters.
func alnumOnly(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, s)
}

// hashData computes SHA256 hash of a string and returns lowercase hex
func hashData(data string) string {
	if data == "" {
		return ""
	}
	hash := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(data))))
	return hex.EncodeToString(hash[:])
}
