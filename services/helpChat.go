package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// helpChatSystemPrompt is the entire knowledge base for the in-app help
// chatbot: a flat list of admin-panel features and their click-paths. Small
// enough to send as a system message on every call instead of building a
// retrieval pipeline — revisit only if this grows past a few thousand words
// and answers start degrading.
const helpChatSystemPrompt = `You are the in-app help assistant for the dz-bazare merchant admin dashboard.
Answer only questions about how to use this admin dashboard. Give short, numbered,
step-by-step instructions using the exact sidebar/menu labels below. Answer in the
same language the merchant writes in (French, Arabic, or English). If asked about
anything unrelated to using this dashboard, say you can only help with using the
platform.

Sidebar sections and what they do:

- Dashboard: sales KPIs, delivery metrics, conversion rate overview.
- Orders: view/filter orders, change order status, edit an order, see confirmation
  actor history, abandoned-cart leads tab.
- Clients: list of storefront customers (by phone), their order history.
- Products > Products: list/search products. "New Product" button creates one:
  set title, base description (or use the "Generate with AI" button to turn a short
  draft into formatted HTML), images, variants (e.g. size/color) and their SKU
  combinations with price/quantity per combination.
- Products > Stock: adjust quantity per product/SKU combination, low-stock filter.
- Landing Pages: create a custom landing page for a product ("New Landing Page"),
  pick a template, attach it to a product, get a shareable preview link.
- Experiments: A/B tests on landing pages/offers — create a variant, split traffic,
  view conversion-rate leaderboard per variant.
- Offers (this is where upsell / order-bump offers are managed): go to "Offers" in
  the sidebar, click "New Offer". Choose the offer type (e.g. upsell shown after
  add-to-cart or at checkout), pick the trigger product and the offered product,
  set the discounted price, save. Edit or deactivate an existing offer from the
  Offers list by clicking it.
- Coupons: create discount codes ("New Coupon"), set percentage/fixed amount,
  usage limits, expiry date, activate/deactivate with the toggle in the list.
- Pixels: connect Facebook or TikTok conversion tracking pixels for a shop,
  add/edit/delete pixel IDs.
- Delivery Rates: set shipping price per wilaya (doorstep/stopdesk), per shop.
- Confirmatrices: manage confirmation-staff accounts and see their delivery-rate
  leaderboard.
- Users: manage staff accounts and roles (owner, moderator, confirmation) for
  this shop.
- Plans: see the shop's current plan/limits, upgrade.
- API: connect delivery-company integrations (Osen Express, Leopard Express,
  ZR Express) with carrier credentials.
- Settings: shop profile, password, general account settings.

General navigation: the sidebar on the left lists every section above; click a
section to open its page. Most list pages have a "New <thing>" button top-right
to create, and a row action menu (usually three dots or edit icon) to edit/delete.`

type helpChatReq struct {
	Model    string          `json:"model"`
	Messages []aiChatMessage `json:"messages"`
}

// AskHelpChat answers one merchant question about using the admin dashboard.
// history is prior turns (role "user"/"assistant") excluding the new question.
func AskHelpChat(question string, history []aiChatMessage) (string, AIUsage, error) {
	baseURL := os.Getenv("AI_BASE_URL")
	apiKey := os.Getenv("AI_API_KEY")
	model := os.Getenv("AI_MODEL_NAME")
	if apiKey == "" || baseURL == "" || model == "" {
		return "", AIUsage{}, ErrAIUnconfigured
	}

	messages := make([]aiChatMessage, 0, len(history)+2)
	messages = append(messages, aiChatMessage{Role: "system", Content: helpChatSystemPrompt})
	messages = append(messages, history...)
	messages = append(messages, aiChatMessage{Role: "user", Content: question})

	reqBody, err := json.Marshal(helpChatReq{Model: model, Messages: messages})
	if err != nil {
		return "", AIUsage{}, err
	}

	req, err := http.NewRequest(http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return "", AIUsage{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", AIUsage{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", AIUsage{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return "", AIUsage{}, fmt.Errorf("AI provider returned %d: %s", resp.StatusCode, string(body))
	}

	var parsed aiChatResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", AIUsage{}, err
	}
	if len(parsed.Choices) == 0 {
		return "", AIUsage{}, fmt.Errorf("AI provider returned no choices")
	}

	usage := AIUsage{
		PromptTokens:     parsed.Usage.PromptTokens,
		CompletionTokens: parsed.Usage.CompletionTokens,
		TotalTokens:      parsed.Usage.TotalTokens,
	}
	text, _ := parsed.Choices[0].Message.Content.(string)
	return text, usage, nil
}
