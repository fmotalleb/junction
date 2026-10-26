package cmd

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRedactConfigMasksNestedSecretsAndURLs(t *testing.T) {
	input := map[string]any{
		"password": "field-password-sentinel",
		"proxy":    "ssh://user:url-password-sentinel@proxy.example:22?token=query-token-sentinel",
		"extra": map[string]any{
			"api_key": "api-key-sentinel",
			"region":  "test-region",
		},
	}
	redacted, err := redactConfig(input)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(redacted)
	if err != nil {
		t.Fatal(err)
	}
	output := string(data)
	for _, sentinel := range []string{
		"field-password-sentinel",
		"url-password-sentinel",
		"query-token-sentinel",
		"api-key-sentinel",
	} {
		if strings.Contains(output, sentinel) {
			t.Errorf("redacted config leaked %q: %s", sentinel, output)
		}
	}
	if !strings.Contains(output, "test-region") || !strings.Contains(output, "REDACTED") {
		t.Fatalf("redacted config lost safe values or markers: %s", output)
	}
}
