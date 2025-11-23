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
)

type FacebookEvent struct {
	EventName      string                 `json:"event_name"`
	EventTime      int64                  `json:"event_time"`
	EventSourceURL string                 `json:"event_source_url,omitempty"`
	ActionSource   string                 `json:"action_source"`
	UserData       map[string]interface{} `json:"user_data"`
	CustomData     map[string]interface{} `json:"custom_data"`
}

type FacebookPayload struct {
	Data          []FacebookEvent `json:"data"`
	TestEventCode string          `json:"test_event_code,omitempty"`
}

// SendFacebookPurchase fires a confirmed purchase event via Conversion API
func SendFacebookPurchase(orderID, fullName, phone string, value float64, currency, fbc, fbp string, createdAt time.Time, testCode string) error {
	pixelID := os.Getenv("FACEBOOK_PIXEL_ID")
	accessToken := os.Getenv("FACEBOOK_ACCESS_TOKEN")

	if pixelID == "" || accessToken == "" {
		return fmt.Errorf("missing FACEBOOK_PIXEL_ID or FACEBOOK_ACCESS_TOKEN")
	}

	url := fmt.Sprintf("https://graph.facebook.com/v23.0/%s/events?access_token=%s", pixelID, accessToken)

	// normalize phone
	phone = strings.TrimSpace(phone)
	// Hash phone (SHA256)
	hashedPhone := hashData(phone)

	parts := strings.Split(fullName, " ")
	first := parts[0]
	last := ""
	if len(parts) > 1 {
		last = parts[len(parts)-1]
	}

	// normalize first and last name
	first = strings.ToLower(strings.TrimSpace(first))
	last = strings.ToLower(strings.TrimSpace(last))
	var hashedFirstName, hashedLastName string

	// Hash fn and ln (SHA256)
	hashedFirstName = hashData(first)
	if last != "" {
		hashedLastName = hashData((last))
	}

	// Include fbp and fbc in user_data
	userData := map[string]interface{}{
		"fn":  hashedFirstName,
		"ln":  hashedLastName,
		"ph":  hashedPhone,
		"fbp": fbp,
		"fbc": fbc,
	}

	customData := map[string]interface{}{
		"currency": currency,
		"value":    value,
		"order_id": orderID,
	}

	event := FacebookEvent{
		EventName:      "Purchase",
		EventTime:      createdAt.Unix(),
		ActionSource:   "website",
		EventSourceURL: "https://lkparfumo.com",
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

	client := &http.Client{}
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

// hashData computes SHA256 hash of a string and returns lowercase hex
func hashData(data string) string {
	if data == "" {
		return ""
	}
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}
