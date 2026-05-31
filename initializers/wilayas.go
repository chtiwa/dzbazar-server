package initializers

import (
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"sync"
)

//go:embed data/static_wilayas.json

var staticFiles embed.FS

type WilayaSeed struct {
	ID           int     `json:"id"`
	Name         string  `json:"name"`
	IsActive     bool    `json:"isActive"`
	HasStopdesk  bool    `json:"hasStopdesk"`
	StopdeskRate float64 `json:"stopdeskRate"`
	HasDoorstep  bool    `json:"hasDoorstep"`
	DoorstepRate float64 `json:"doorstepRate"`
}

var loadWilayasOnce = sync.OnceValues(func() ([]WilayaSeed, error) {
	data, err := staticFiles.ReadFile("data/static_wilayas.json")
	if err != nil {
		return nil, fmt.Errorf("failed to read static wilayas json: %w", err)
	}

	var wilayas []WilayaSeed
	if err := json.Unmarshal(data, &wilayas); err != nil {
		return nil, fmt.Errorf("failed to unmarshal static wilayas json: %w", err)
	}

	return wilayas, nil
})

func GetStaticWilayas() ([]WilayaSeed, error) {
	return loadWilayasOnce()
}

func InitStaticData() {
	if _, err := GetStaticWilayas(); err != nil {
		log.Fatalf("failed to initialize static wilayas: %v", err)
	}
}
