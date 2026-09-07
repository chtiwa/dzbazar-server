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

// ReferenceImage is one photo fed to the model as visual reference.
type ReferenceImage struct {
	Bytes    []byte
	MimeType string // e.g. "image/png", "image/jpeg"
}

// AIImageModel identifies which OpenRouter image model to call.
type AIImageModel string

const (
	AIImageModelPro   AIImageModel = "google/gemini-3-pro-image-preview" // Nano Banana Pro
	AIImageModelFlash AIImageModel = "google/gemini-2.5-flash-image"     // Nano Banana Flash
	AIImageModelGPT   AIImageModel = "openai/gpt-image-1"                // GPT Image 1
	AIImageModelFlux  AIImageModel = "black-forest-labs/flux.2-flex"     // FLUX.2 Flex
)

// AIImageModelCredits is the per-generation credit cost for each selectable
// model, ranked by the provider's own per-image price (priciest first).
// Merchants pick a model via the shopId/generate form field; the controller
// validates it against this map and bills the matching cost.
var AIImageModelCredits = map[AIImageModel]int{
	AIImageModelGPT:   25,
	AIImageModelPro:   20,
	AIImageModelFlash: 10,
	AIImageModelFlux:  6,
}

func (m AIImageModel) Valid() bool {
	_, ok := AIImageModelCredits[m]
	return ok
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
