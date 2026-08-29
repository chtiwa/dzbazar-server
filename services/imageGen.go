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
Target audience: %s
Offer / guarantee details: %s

Generate ONE full-width landing page section image (9:16 vertical, 2K), photorealistic, high-end e-commerce style, optimized for Meta/TikTok traffic. ALL text rendered in the image MUST be in Arabic only — no French, no English, no other language anywhere in the image. Text must be legible, correctly spelled Arabic, right-to-left, professionally typeset. Imagery, tone, and casting should speak directly to the target audience above. Where the section calls for trust badges, guarantees, or a CTA, use the exact offer/guarantee details given above instead of generic placeholder claims. Do NOT alter the product's logo, shape, or packaging details.

This section is: %s`

// hero and close always appear; the rest fill in by priority as the image
// count grows, so a 1-image request still opens strong and an 8-image
// request adds depth (benefits, comparison, FAQ) without reordering the arc.
const (
	sectionHero = iota
	sectionClose
	sectionSolution
	sectionTrust
	sectionProblem
	sectionBenefits
	sectionComparison
	sectionFAQ
)

var landingPageSections = map[int]string{
	sectionHero: `HERO / SHOWCASE — large, clean hero featuring the product prominently. Strong Arabic headline meaning "Transform Your [PRIMARY BENEFIT] Today" (or category-appropriate variant), e.g. "غيّر [الفائدة الرئيسية] اليوم". Subheadline with a key benefit. Primary CTA button graphic reading "اطلب الآن" or "اشترِ الآن". Premium, on-brand, minimal-distraction background.`,
	sectionProblem: `PROBLEM AGITATION — visualize the pain point the customer faces WITHOUT this product, via relatable lifestyle imagery (frustration, inconvenience, wasted time/money). Overlay bold Arabic text meaning "Tired of [PAIN POINT]?", e.g. "هل سئمت من [المشكلة]؟". Keep the product visible but smaller, as the upcoming solution.`,
	sectionSolution: `SOLUTION REVEAL — show the product in action, solving the problem clearly, before/after or cause/effect visual if possible. Arabic headline meaning "Introducing [PRODUCT NAME] – The Smart Way to [PRIMARY BENEFIT]". 2-3 short Arabic bullet icons or text callouts (e.g. "سريع", "آمن", "مضمون").`,
	sectionBenefits: `BENEFITS BREAKDOWN — a features/benefits grid, 3-4 icons each paired with a short Arabic label (e.g. specific product benefits relevant to the category). Product shown alongside or centered. Clean grid layout, scannable at a glance.`,
	sectionComparison: `COMPARISON — a simple "Before [PRODUCT NAME] vs After" or "Us vs Them" two-column comparison visual, product's advantages on one side (checkmarks), the old/painful way on the other (X marks). Arabic headline meaning "Why Choose Us", e.g. "لماذا تختارنا".`,
	sectionTrust: `TRUST & CONVICTION — social proof and trust elements: star ratings, short Arabic testimonial quotes (e.g. "غيّر حياتي!", "أفضل عملية شراء قمت بها"), plus trust badges in Arabic built from the offer/guarantee details given above (e.g. delivery promise, payment method, guarantee length). Clean, credible layout, product centered.`,
	sectionFAQ: `FAQ & GUARANTEE — a short Arabic FAQ block (2-3 common objections answered, e.g. delivery time, return policy, product fit, grounded in the offer/guarantee details given above) paired with a prominent guarantee badge in Arabic reflecting those same details. Reassuring, clean, text-legible layout.`,
	sectionClose: `CONVERSION / CLOSE — strong final CTA section. Large Arabic headline driving urgency to order now, built around the offer/guarantee details given above (price, discount, delivery, payment method), e.g. "اطلب الآن – الدفع عند الاستلام". Icons for the relevant guarantees. Arabic urgency element (e.g. "عرض محدود", "الكمية محدودة", or "العرض ينتهي قريبًا"). Big bold CTA button reading "اشترِ الآن".`,
}

// sectionPriority fills in beyond hero+close as the requested count grows.
var sectionPriority = []int{sectionHero, sectionClose, sectionSolution, sectionTrust, sectionProblem, sectionBenefits, sectionComparison, sectionFAQ}

const (
	MinLandingPageImageCount = 1
	MaxLandingPageImageCount = 8
)

// pickSections returns `count` section keys (1-8) in buyer-journey order:
// hero first, close last, everything else by sectionPriority in between.
func pickSections(count int) []int {
	if count > len(sectionPriority) {
		count = len(sectionPriority)
	}
	chosen := map[int]bool{}
	for _, s := range sectionPriority[:count] {
		chosen[s] = true
	}

	ordered := []int{sectionHero, sectionProblem, sectionSolution, sectionBenefits, sectionComparison, sectionTrust, sectionFAQ, sectionClose}
	result := make([]int, 0, count)
	for _, s := range ordered {
		if chosen[s] {
			result = append(result, s)
		}
	}
	return result
}

// GenerateLandingPageImageSet generates `count` (1-8) landing-page section
// images in one buyer-journey flow, using the given product photos as the
// model's ONLY visual reference for appearance/packaging/logo/colors.
func GenerateLandingPageImageSet(refs []ReferenceImage, productName, category, benefit, painPoint, audience, offerDetails string, count int) ([][]byte, error) {
	imageParts := make([]aiContentPart, len(refs))
	for i, ref := range refs {
		imageParts[i] = aiContentPart{
			Type:     "image_url",
			ImageURL: &aiContentImage{URL: fmt.Sprintf("data:%s;base64,%s", ref.MimeType, base64.StdEncoding.EncodeToString(ref.Bytes))},
		}
	}

	sections := pickSections(count)
	results := make([][]byte, len(sections))
	for i, section := range sections {
		prompt := fmt.Sprintf(landingPageSectionPromptTemplate, productName, category, benefit, painPoint, audience, offerDetails, landingPageSections[section])
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
