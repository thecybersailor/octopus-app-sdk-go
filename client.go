package octopusapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Time aliases time.Time for SDK response fields.
type Time = time.Time

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

// APIError is returned when the platform responds with a pin error envelope.
type APIError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *APIError) Error() string {
	if e == nil {
		return "octopus app sdk: api error"
	}
	if e.Code != "" && e.Message != "" {
		return fmt.Sprintf("octopus app sdk: api error %s: %s", e.Code, e.Message)
	}
	if e.Message != "" {
		return fmt.Sprintf("octopus app sdk: api error: %s", e.Message)
	}
	if e.Code != "" {
		return fmt.Sprintf("octopus app sdk: api error %s", e.Code)
	}
	return fmt.Sprintf("octopus app sdk: api error status %d", e.StatusCode)
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

type pinEnvelope[T any] struct {
	Data  *T        `json:"data"`
	Error *pinError `json:"error"`
}

type pinError struct {
	Code    string `json:"code"`
	Key     string `json:"key"`
	Message string `json:"message"`
}

func (c *Client) postJSON(ctx context.Context, path string, request any, response any) error {
	body, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("octopus app sdk: encode %s request: %w", path, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("octopus app sdk: create %s request: %w", path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	res, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("octopus app sdk: post %s: %w", path, err)
	}
	defer res.Body.Close()

	var envelope pinEnvelope[json.RawMessage]
	if err := json.NewDecoder(res.Body).Decode(&envelope); err != nil {
		return fmt.Errorf("octopus app sdk: decode %s response: %w", path, err)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 || envelope.Error != nil {
		apiErr := &APIError{StatusCode: res.StatusCode}
		if envelope.Error != nil {
			apiErr.Code = strings.TrimSpace(envelope.Error.Code)
			if apiErr.Code == "" {
				apiErr.Code = strings.TrimSpace(envelope.Error.Key)
			}
			apiErr.Message = strings.TrimSpace(envelope.Error.Message)
		}
		return apiErr
	}
	if envelope.Data == nil {
		return fmt.Errorf("octopus app sdk: decode %s response: missing data", path)
	}
	if err := json.Unmarshal(*envelope.Data, response); err != nil {
		return fmt.Errorf("octopus app sdk: decode %s data: %w", path, err)
	}
	return nil
}

const openSessionExchangePath = "/api/v1/partner-apps/open-sessions/exchange"

type exchangeOpenSessionRequest struct {
	KeyID     string `json:"keyId"`
	Secret    string `json:"secret"`
	OpenToken string `json:"openToken"`
}

// OpenSessionContext is the trusted context returned after exchanging an open token.
type OpenSessionContext struct {
	TenantID          string `json:"tenantId"`
	PartnerID         string `json:"partnerId"`
	ApplicationID     string `json:"applicationId"`
	BindingID         string `json:"bindingId"`
	TeamID            string `json:"teamId"`
	UserID            string `json:"userId"`
	TeamRole          string `json:"teamRole"`
	AppTeamExternalID string `json:"appTeamExternalId"`
	ExpiresAt         Time   `json:"expiresAt"`
}

// ExchangeOpenSession exchanges a one-time open token for trusted application context.
func (c *Client) ExchangeOpenSession(ctx context.Context, openToken string) (OpenSessionContext, error) {
	openToken = strings.TrimSpace(openToken)
	if openToken == "" {
		return OpenSessionContext{}, errors.New("octopus app sdk: open token is required")
	}
	var out OpenSessionContext
	err := c.postJSON(ctx, openSessionExchangePath, exchangeOpenSessionRequest{KeyID: c.keyID, Secret: c.secret, OpenToken: openToken}, &out)
	return out, err
}
