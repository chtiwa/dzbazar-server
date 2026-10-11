package services

import (
	"encoding/json"
	"testing"
)

// A malformed Params string would make the provider reject every Pro shop's
// chat request, so catch it at build time.
func TestAgentToolsWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, tool := range agentTools {
		if seen[tool.Name] {
			t.Errorf("duplicate tool %s", tool.Name)
		}
		seen[tool.Name] = true
		var schema map[string]any
		if err := json.Unmarshal([]byte(tool.Params), &schema); err != nil {
			t.Errorf("%s: bad Params JSON: %v", tool.Name, err)
		}
		if tool.Action == "" || tool.Description == "" {
			t.Errorf("%s: missing action or description", tool.Name)
		}
	}
}
