package services

import (
	"strings"
	"testing"
)

func TestGenerateLandingPageImagePropagatesError(t *testing.T) {
	orig := callImageGen
	defer func() { callImageGen = orig }()

	callImageGen = func(content []aiContentPart, model string) ([]byte, error) {
		return nil, ErrNoImageGenerated
	}

	_, err := GenerateLandingPageImage(nil, "a hero shot", AIImageModelPro)
	if err == nil {
		t.Fatal("want error, got nil")
	}
}

func TestGenerateLandingPageImageWrapsPromptAndUsesModel(t *testing.T) {
	orig := callImageGen
	defer func() { callImageGen = orig }()

	var gotModel string
	var gotContent []aiContentPart
	callImageGen = func(content []aiContentPart, model string) ([]byte, error) {
		gotModel = model
		gotContent = content
		return []byte("ok"), nil
	}

	_, err := GenerateLandingPageImage(nil, "a hero shot", AIImageModelFlash)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotModel != string(AIImageModelFlash) {
		t.Errorf("want model %q, got %q", AIImageModelFlash, gotModel)
	}
	if len(gotContent) != 1 || gotContent[0].Type != "text" {
		t.Fatalf("want one text content part, got %+v", gotContent)
	}
	if !strings.Contains(gotContent[0].Text, "a hero shot") {
		t.Errorf("wrapped prompt does not contain the original prompt: %s", gotContent[0].Text)
	}
}
