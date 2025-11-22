package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

type TikTokEvent struct {
	Event      string                 `json:"event"`
	EventID    string                 `json:"event_id"`
	Timestamp  int64                  `json:"timestamp"`
	Context    TikTokContext          `json:"context"`
	Properties map[string]interface{} `json:"properties"`
}

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
	ExternalID   string `json:"external_id,omitempty"`
	PhoneNumber  string `json:"phone_number,omitempty"`
	Email        string `json:"email,omitempty"`
	IP           string `json:"ip,omitempty"`
	UserAgent    string `json:"user_agent,omitempty"`
	ClientUserID string `json:"client_user_id,omitempty"`
}

type TikTokPayload struct {
	EventSource   string        `json:"event_source"`
	EventSourceID string        `json:"event_source_id"`
	PartnerName   string        `json:"partner_name"`
	Data          []TikTokEvent `json:"data"`
}

func SendTikTokPurchase(orderID, fullName, phone, ttclid string, value float64, currency string, createdAt time.Time) error {
	pixelID := os.Getenv("TIKTOK_PIXEL_ID")
	accessToken := os.Getenv("TIKTOK_ACCESS_TOKEN")

	if pixelID == "" || accessToken == "" {
		return fmt.Errorf("missing TIKTOK_PIXEL_ID or TIKTOK_ACCESS_TOKEN")
	}

	url := "https://business-api.tiktok.com/open_api/v1.3/event/track/"

	hashedPhone := hashData(phone)
	hashedName := hashData(fullName)

	event := TikTokEvent{
		Event:     "CompletePayment", // TikTok's name for Purchase
		EventID:   orderID,
		Timestamp: createdAt.Unix(),
		Context: TikTokContext{
			Ad: TikTokAdContext{
				Callback: ttclid, // Optional but recommended
			},
			Page: TikTokPageContext{
				URL: "https://lkparfumo.com",
			},
			User: TikTokUserContext{
				PhoneNumber: hashedPhone,
				ExternalID:  hashedName,
			},
		},
		Properties: map[string]interface{}{
			"currency": currency,
			"value":    value,
			"contents": []map[string]interface{}{
				{"content_id": orderID, "content_type": "product"},
			},
		},
	}

	payload := TikTokPayload{
		EventSource:   "web",
		EventSourceID: pixelID,
		PartnerName:   "Flodybox",
		Data:          []TikTokEvent{event},
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
