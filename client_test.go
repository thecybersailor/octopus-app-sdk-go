package octopusapp

import "testing"

func TestNewClientNormalizesBaseURLAndRequiresCredentials(t *testing.T) {
	client, err := NewClient(ClientConfig{
		BaseURL: "https://octopus.example.com/",
		KeyID:   "key_123",
		Secret:  "secret_123",
	})
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}
	if client.baseURL != "https://octopus.example.com" {
		t.Fatalf("baseURL should trim trailing slash, got %q", client.baseURL)
	}

	cases := []ClientConfig{
		{KeyID: "key_123", Secret: "secret_123"},
		{BaseURL: "https://octopus.example.com", Secret: "secret_123"},
		{BaseURL: "https://octopus.example.com", KeyID: "key_123"},
	}
	for _, cfg := range cases {
		if _, err := NewClient(cfg); err == nil {
			t.Fatalf("NewClient(%+v) expected validation error", cfg)
		}
	}
}
