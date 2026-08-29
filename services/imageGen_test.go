package services

import (
	"fmt"
	"math/rand"
	"testing"
	"time"
)

// TestGenerateLandingPageImageSetPreservesOrder stubs callImageGen with a
// randomized delay so goroutines finish out of order, then asserts each
// result still lands at the section's original index — the one property
// the parallel fan-out must not break.
func TestGenerateLandingPageImageSetPreservesOrder(t *testing.T) {
	orig := callImageGen
	defer func() { callImageGen = orig }()

	callImageGen = func(content []aiContentPart) ([]byte, error) {
		time.Sleep(time.Duration(rand.Intn(5)) * time.Millisecond)
		return []byte(content[0].Text), nil
	}

	in := LandingPageImageSetInput{ProductName: "P", Category: "C", Benefit: "B", PainPoint: "PP", Audience: "A", OfferDetails: "O", Count: MaxLandingPageImageCount}
	results, err := GenerateLandingPageImageSet(nil, in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sections := pickSections(in.Count)
	if len(results) != len(sections) {
		t.Fatalf("want %d results, got %d", len(sections), len(results))
	}
	for i, section := range sections {
		wantPrompt := fmt.Sprintf(landingPageSectionPromptTemplate, in.ProductName, in.Category, in.Benefit, in.PainPoint, in.Audience, in.OfferDetails, personaBlock(in), landingPageSections[section])
		if string(results[i]) != wantPrompt {
			t.Errorf("index %d: result does not match section %d's prompt", i, section)
		}
	}
}

func TestGenerateLandingPageImageSetPropagatesError(t *testing.T) {
	orig := callImageGen
	defer func() { callImageGen = orig }()

	callImageGen = func(content []aiContentPart) ([]byte, error) {
		return nil, ErrNoImageGenerated
	}

	_, err := GenerateLandingPageImageSet(nil, LandingPageImageSetInput{Count: 2})
	if err == nil {
		t.Fatal("want error, got nil")
	}
}
