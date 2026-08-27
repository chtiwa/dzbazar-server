package services

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/HugoSmits86/nativewebp"
)

var ErrNoImageGenerated = fmt.Errorf("AI provider returned no image")

type imageGenReq struct {
	Model    string          `json:"model"`
	Messages []aiChatMessage `json:"messages"`
}

type imageGenResponse struct {
	Choices []struct {
		Message struct {
			Images []struct {
				ImageURL struct {
					URL string `json:"url"`
				} `json:"image_url"`
			} `json:"images"`
		} `json:"message"`
	} `json:"choices"`
}

func callImageGenModel(content []aiContentPart) ([]byte, error) {
	baseURL := os.Getenv("AI_BASE_URL")
	apiKey := os.Getenv("AI_API_KEY")
	model := os.Getenv("AI_IMAGE_MODEL_NAME")
	if apiKey == "" || baseURL == "" || model == "" {
		return nil, ErrAIUnconfigured
	}

	var msgContent interface{} = content
	if len(content) == 1 && content[0].Type == "text" {
		msgContent = content[0].Text // plain string when there's no image input
	}

	reqBody, err := json.Marshal(imageGenReq{
		Model: model,
		Messages: []aiChatMessage{
			{Role: "user", Content: msgContent},
		},
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("AI provider returned %d: %s", resp.StatusCode, string(body))
	}

	var parsed imageGenResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	if len(parsed.Choices) == 0 || len(parsed.Choices[0].Message.Images) == 0 {
		return nil, ErrNoImageGenerated
	}

	dataURL := parsed.Choices[0].Message.Images[0].ImageURL.URL
	_, b64, found := strings.Cut(dataURL, ",")
	if !found {
		return nil, ErrNoImageGenerated
	}

	return base64.StdEncoding.DecodeString(b64)
}

// GenerateLandingPageImage turns a text prompt into a PNG image via the
// configured AI proxy's image-capable model (e.g. an OpenRouter
// "*-image" model). Returns raw decoded PNG bytes.
func GenerateLandingPageImage(prompt string) ([]byte, error) {
	return callImageGenModel([]aiContentPart{{Type: "text", Text: prompt}})
}

// ReferenceImage is one product photo fed to the model as visual reference.
type ReferenceImage struct {
	Bytes    []byte
	MimeType string // e.g. "image/png", "image/jpeg"
}

const landingPageSectionPromptTemplate = `You are a conversion-focused landing page visual designer.

Use the attached product photos as the ONLY reference for the product's appearance, packaging, logo, and colors. Keep the product 100%% consistent with the photos.

Product name: %s
Category: %s
Primary benefit: %s
Main pain point: %s

Generate ONE full-width landing page section image (16:9, 2K), photorealistic, high-end e-commerce style, optimized for Meta/TikTok traffic. Text must be legible, correctly spelled, professionally typeset. Do NOT alter the product's logo, shape, or packaging details.

This section is: %s`

var landingPageSections = []string{
	`HERO / SHOWCASE — large, clean hero featuring the product prominently. Strong headline "Transform Your [PRIMARY BENEFIT] Today" (or category-appropriate variant). Subheadline with a key benefit. Primary CTA button graphic: "Commander maintenant" / "Acheter". Premium, on-brand, minimal-distraction background.`,
	`PROBLEM AGITATION — visualize the pain point the customer faces WITHOUT this product, via relatable lifestyle imagery (frustration, inconvenience, wasted time/money). Overlay bold text "Tired of [PAIN POINT]?" or "Still struggling with [PAIN POINT]?". Keep the product visible but smaller, as the upcoming solution.`,
	`SOLUTION REVEAL — show the product in action, solving the problem clearly, before/after or cause/effect visual if possible. Headline "Introducing [PRODUCT NAME] – The Smart Way to [PRIMARY BENEFIT]". 2-3 short bullet icons or text callouts (e.g. "Fast", "Safe", "Proven").`,
	`TRUST & CONVICTION — social proof and trust elements: star ratings, short testimonial quotes ("Life-changing!", "Best purchase ever"), trust badges "Livraison gratuite", "Paiement à la livraison (COD)", "Garantie 30 jours". Clean, credible layout, product centered.`,
	`CONVERSION / CLOSE — strong final CTA section. Large headline "Commandez Aujourd'hui – Paiement à la Livraison (COD)". Icons for COD, 30-day guarantee, secure checkout, fast shipping. Urgency element ("Offre limitée", "Stock limité", or "Promotion se termine bientôt"). Big bold CTA button "Acheter Maintenant".`,
}

// GenerateLandingPageImageSet generates the 5 standard landing-page section
// images (hero, problem, solution, trust, close) in one buyer-journey flow,
// using the given product photos as the model's ONLY visual reference for
// appearance/packaging/logo/colors. Returns 5 PNG byte slices in order.
func GenerateLandingPageImageSet(refs []ReferenceImage, productName, category, benefit, painPoint string) ([][]byte, error) {
	imageParts := make([]aiContentPart, len(refs))
	for i, ref := range refs {
		imageParts[i] = aiContentPart{
			Type:     "image_url",
			ImageURL: &aiContentImage{URL: fmt.Sprintf("data:%s;base64,%s", ref.MimeType, base64.StdEncoding.EncodeToString(ref.Bytes))},
		}
	}

	results := make([][]byte, len(landingPageSections))
	for i, section := range landingPageSections {
		prompt := fmt.Sprintf(landingPageSectionPromptTemplate, productName, category, benefit, painPoint, section)
		content := append([]aiContentPart{{Type: "text", Text: prompt}}, imageParts...)

		img, err := callImageGenModel(content)
		if err != nil {
			return nil, fmt.Errorf("section %d: %w", i+1, err)
		}
		results[i] = img
	}
	return results, nil
}

// ToWebP re-encodes an image (PNG or JPEG bytes, as returned by the AI
// provider) as WebP using a pure-Go encoder — no system dependency, works
// identically on any machine Go builds for. It's lossless-only (no lossy
// encoder exists in the pure-Go ecosystem), so the size win over PNG is
// modest rather than the large cut a lossy cwebp encode would give.
func ToWebP(imageBytes []byte) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(imageBytes))
	if err != nil {
		return nil, fmt.Errorf("decode source image: %w", err)
	}

	var out bytes.Buffer
	if err := nativewebp.Encode(&out, img, nil); err != nil {
		return nil, fmt.Errorf("encode webp: %w", err)
	}
	return out.Bytes(), nil
}
