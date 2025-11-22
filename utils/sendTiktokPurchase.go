package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

// TikTokEvent represents a single event
type TikTokEvent struct {
	Event      string                 `json:"event"`
	EventID    string                 `json:"event_id"`
	Timestamp  int64                  `json:"timestamp"`
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
	ClientUserID string `json:"client_user_id,omitempty"`
}

type TikTokPayload struct {
	EventSource   string        `json:"event_source"`    // "server" for server-side events
	EventSourceID string        `json:"event_source_id"` // your pixel ID
	PartnerName   string        `json:"partner_name"`
	Data          []TikTokEvent `json:"data"`
}

// SendTikTokPurchase sends a purchase event via TikTok CAPI
func SendTikTokPurchase(orderID, phone, ttclid string, value float64, currency string, createdAt time.Time) error {
	pixelID := os.Getenv("TIKTOK_PIXEL_ID")
	accessToken := os.Getenv("TIKTOK_ACCESS_TOKEN")

	if pixelID == "" || accessToken == "" {
		return fmt.Errorf("missing TIKTOK_PIXEL_ID or TIKTOK_ACCESS_TOKEN")
	}

	url := "https://business-api.tiktok.com/open_api/v1.3/event/track/"

	hashedPhone := hashData(phone)

	event := TikTokEvent{
		Event:     "Purchase", // TikTok's Purchase event
		EventID:   orderID,
		Timestamp: createdAt.Unix(),
		Context: TikTokContext{
			Ad: TikTokAdContext{
				Callback: ttclid, // optional, only if available
			},
			Page: TikTokPageContext{
				URL: "https://lkparfumo.com",
			},
			User: TikTokUserContext{
				PhoneNumber: hashedPhone, // required if no email
			},
		},
		Properties: map[string]interface{}{
			"currency": currency,
			"value":    value,
			"contents": []map[string]interface{}{
				{"content_id": orderID, "quantity": 1, "content_type": "product"},
			},
		},
	}

	payload := TikTokPayload{
		EventSource:   "server", // must be "server" for server-side events
		EventSourceID: pixelID,
		PartnerName:   "Lkparfumo",
		Data:          []TikTokEvent{event},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %v", err)
	}

	// Debug log
	fmt.Printf("Sending TikTok payload: %s\n", string(jsonData))

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Access-Token", accessToken)

	client := &http.Client{Timeout: 10 * time.Second}
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
