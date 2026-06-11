package initializers

import (
	"encoding/json"
	"fmt"
	"sync"
)

type OsenMunicipalitySeed struct {
	ID        int    `json:"id"`
	NameLatin string `json:"name_latin"`
}

type OsenProvinceSeed struct {
	ID             int                    `json:"id"`
	NameLatin      string                 `json:"name_latin"`
	Municipalities []OsenMunicipalitySeed `json:"municipalities"`
}

var loadOsenMunicipalitiesOnce = sync.OnceValues(func() ([]OsenProvinceSeed, error) {
	data, err := staticFiles.ReadFile("data/osen_municipalities.json")
	if err != nil {
		return nil, fmt.Errorf("failed to read static osen municipalities json: %w", err)
	}

	var provinces []OsenProvinceSeed
	if err := json.Unmarshal(data, &provinces); err != nil {
		return nil, fmt.Errorf("failed to unmarshal static osen municipalities json: %w", err)
	}

	return provinces, nil
})

func GetOsenMunicipalities() ([]OsenProvinceSeed, error) {
	return loadOsenMunicipalitiesOnce()
}
