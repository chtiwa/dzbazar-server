package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// needs stape integration

// TikTokEvent represents a single event
type TikTokEvent struct {
	Event      string                 `json:"event"`
	EventID    string                 `json:"event_id"`
	EventTime  int64                  `json:"event_time"`
	Context    TikTokContext          `json:"context"`
	Properties map[string]interface{} `json:"properties"`
}

// TikTokContext holds user, page, and ad info
type TikTokContext struct {
	Ad   TikTokAdContext   `json:"ad,omitempty"`
	Page TikTokPageContext `json:"page"`
	User TikTokUserContext `json:"user"`
}

type TikTokAdContext struct {
	Callback string `json:"callback,omitempty"` // ttclid
}

type TikTokPageContext struct {
	URL string `json:"url"`
}

type TikTokUserContext struct {
	PhoneNumber  string `json:"phone_number,omitempty"` // SHA256 hashed
	Email        string `json:"email,omitempty"`
	FirstName    string `json:"first_name,omitempty"`
	LastName     string `json:"last_name,omitempty"`
	ClientUserID string `json:"client_user_id,omitempty"`
	ExternalID   string `json:"external_id,omitempty"`
}

type TikTokPayload struct {
	EventSource   string        `json:"event_source"`    // "server" for server-side events
	EventSourceID string        `json:"event_source_id"` // your pixel ID
	PartnerName   string        `json:"partner_name"`
	Data          []TikTokEvent `json:"data"`
	TestEventCode string        `json:"test_event_code,omitempty"`
}

// SendTikTokPurchase sends a purchase event via TikTok CAPI
func SendTikTokPurchase(orderID, productName, fullName, phone, ttclid string, value float64, currency string, createdAt time.Time, testCode, clientUserAgent, clientIP string) error {
	pixelID := os.Getenv("TIKTOK_PIXEL_ID")
	accessToken := os.Getenv("TIKTOK_ACCESS_TOKEN")

	if pixelID == "" || accessToken == "" {
		return fmt.Errorf("missing TIKTOK_PIXEL_ID or TIKTOK_ACCESS_TOKEN")
	}

	url := "https://business-api.tiktok.com/open_api/v1.3/event/track/"

	hashedPhone := hashData(phone)
	parts := strings.Split(fullName, " ")
	first := parts[0]
	last := ""
	if len(parts) > 1 {
		last = parts[len(parts)-1]
	}

	var hashedFirstName, hashedLastName string
	hashedFirstName = hashData(first)
	if last != "" {
		hashedLastName = hashData((last))
	}

	data := []map[string]interface{}{
		{
			"event":      "Purchase",
			"event_id":   orderID,
			"event_time": createdAt.Unix(),
			"user": map[string]string{
				"phone_number_sha256": hashedPhone,
				"first_name_sha256":   hashedFirstName,
				"last_name_sha256":    hashedLastName,
			},
			"properties": map[string]interface{}{
				"currency": currency,
				"value":    value,
				"order_id": orderID,
				"contents": []map[string]interface{}{{
					"content_id":   orderID,
					"content_type": "product", // or "product_group"
				}},
			},
			"context": map[string]interface{}{
				"ip":         clientIP, // add params
				"user_agent": clientUserAgent,
				"ad":         map[string]string{"callback": ttclid},
				"page":       map[string]string{"url": "https://lkparfumo.com"},
			},
		},
	}

	// payload := map[string]interface{}{
	// 	"data":            data,
	// 	"pixel_code":      pixelID,
	// 	"partner_name":    "default",
	// 	"test_event_code": testCode,
	// }

	payload := map[string]interface{}{
		"event_source":    "web", // or "server" depending on your setup
		"event_source_id": pixelID,
		"partner_name":    "default",
		"data":            data, // your existing event array
	}
	if testCode != "" {
		payload["test_event_code"] = testCode
	}

	jsonData, err := json.Marshal(payload)
	fmt.Printf("SENDING PAYLOAD: %s\n", string(jsonData))
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %v", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Access-Token", accessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	var respBody bytes.Buffer
	respBody.ReadFrom(resp.Body)

	if resp.StatusCode >= 300 {
		return fmt.Errorf("TikTok API error: %d, body: %s", resp.StatusCode, respBody.String())
	}

	fmt.Printf("TikTok API response: %d, body: %s\n", resp.StatusCode, respBody.String())
	return nil
}
