package octopusapp

import (
	"errors"
	"net/http"
	"strings"
)

// ClientConfig configures an Octopus partner application client.
type ClientConfig struct {
	BaseURL    string
	KeyID      string
	Secret     string
	HTTPClient *http.Client
}

// Client calls the Octopus partner application server-to-server API.
type Client struct {
	baseURL    string
	keyID      string
	secret     string
	httpClient *http.Client
}

// NewClient validates config and returns an API client.
func NewClient(cfg ClientConfig) (*Client, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	keyID := strings.TrimSpace(cfg.KeyID)
	secret := strings.TrimSpace(cfg.Secret)
	if baseURL == "" {
		return nil, errors.New("octopus app sdk: base URL is required")
	}
	if keyID == "" {
		return nil, errors.New("octopus app sdk: key ID is required")
	}
	if secret == "" {
		return nil, errors.New("octopus app sdk: secret is required")
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{baseURL: baseURL, keyID: keyID, secret: secret, httpClient: httpClient}, nil
}
