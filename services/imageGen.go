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

// callImageGen is a package var (not the func directly) so tests can stub it.
var callImageGen = callImageGenModel

func callImageGenModel(content []aiContentPart, model string) ([]byte, error) {
	baseURL := os.Getenv("AI_BASE_URL")
	apiKey := os.Getenv("AI_API_KEY")
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

// AIImageModel identifies which OpenRouter image model to call.
type AIImageModel string

const (
	AIImageModelPro   AIImageModel = "google/gemini-3-pro-image-preview" // Nano Banana Pro
	AIImageModelFlash AIImageModel = "google/gemini-2.5-flash-image"     // Nano Banana Flash
)

func (m AIImageModel) Valid() bool {
	return m == AIImageModelPro || m == AIImageModelFlash
}

// CreditCost returns the per-image credit charge for this model.
func (m AIImageModel) CreditCost() int {
	if m == AIImageModelFlash {
		return CreditCostImageFlash
	}
	return CreditCostImagePro
}

const landingPageImagePromptWrapper = `Use the attached product photo(s) as the ONLY reference for the product's appearance, packaging, logo, and colors. Keep the product 100%% consistent with the photos. Do NOT alter the product's logo, shape, or packaging details.

%s

Output: ONE full-width landing-page section image, 9:16 vertical, 2K, photorealistic, high-end e-commerce style. Any text rendered in the image MUST be in Arabic only — no French, no English, no other language anywhere in the image. Text must be legible, correctly spelled Arabic, right-to-left, professionally typeset.`

const (
	MaxLandingPageImageCount = 5
)

// GenerateLandingPageImage generates one landing-page image from a free-text
// prompt plus 1-5 reference photos, using the given model. The prompt is
// wrapped with fixed technical constraints (9:16, Arabic-only text,
// photorealistic) so output stays on-spec regardless of what the merchant
// writes as the creative brief.
func GenerateLandingPageImage(refs []ReferenceImage, prompt string, model AIImageModel) ([]byte, error) {
	imageParts := make([]aiContentPart, len(refs))
	for i, ref := range refs {
		imageParts[i] = aiContentPart{
			Type:     "image_url",
			ImageURL: &aiContentImage{URL: fmt.Sprintf("data:%s;base64,%s", ref.MimeType, base64.StdEncoding.EncodeToString(ref.Bytes))},
		}
	}
	fullPrompt := fmt.Sprintf(landingPageImagePromptWrapper, prompt)
	content := append([]aiContentPart{{Type: "text", Text: fullPrompt}}, imageParts...)
	return callImageGen(content, string(model))
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
