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
	IdentityTenantID string `json:"identityTenantId"`
	TenantID         string `json:"tenantId"`
	PartnerID        string `json:"partnerId"`
	ApplicationID    string `json:"applicationId"`
	BindingID        string `json:"bindingId"`
	TeamID           string `json:"teamId"`
	UserID           string `json:"userId"`
	// ArcubaseUserID identifies the human employee within TeamID. It is a numeric
	// string; store and compare it as a string. Use (TeamID, ArcubaseUserID) for
	// employee associations and (IdentityTenantID, UserID) for platform accounts.
	ArcubaseUserID    string `json:"arcubaseUserId"`
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
	if err != nil {
		return OpenSessionContext{}, err
	}
	if strings.TrimSpace(out.IdentityTenantID) == "" || strings.TrimSpace(out.UserID) == "" {
		return OpenSessionContext{}, errors.New("octopus app sdk: open session is missing identity context; reopen the application after updating the provider")
	}
	if strings.TrimSpace(out.ArcubaseUserID) == "" {
		return OpenSessionContext{}, errors.New("octopus app sdk: open session is missing employee identity (arcubaseUserId); reopen the application from the team app list")
	}
	return out, nil
}

const pointTransactionsBasePath = "/api/v1/partner-apps/point-transactions"

// PointTransaction is the platform projection of an application point transaction.
type PointTransaction struct {
	TransactionID                string `json:"transactionId"`
	TenantID                     string `json:"tenantId"`
	PartnerID                    string `json:"partnerId"`
	ApplicationID                string `json:"applicationId"`
	BindingID                    string `json:"bindingId"`
	TeamID                       string `json:"teamId"`
	AppTeamExternalID            string `json:"appTeamExternalId"`
	OperationRef                 string `json:"operationRef"`
	AccountTransactionID         string `json:"accountTransactionId"`
	OriginalAccountTransactionID string `json:"originalAccountTransactionId,omitempty"`
	Status                       string `json:"status"`
	ReservedPoints               int64  `json:"reservedPoints"`
	CapturedPoints               int64  `json:"capturedPoints"`
	RefundedPoints               int64  `json:"refundedPoints"`
	CreatedAt                    Time   `json:"createdAt,omitempty"`
	UpdatedAt                    Time   `json:"updatedAt,omitempty"`
}

// ReservePointsRequest starts a point reservation for an application operation.
type ReservePointsRequest struct {
	AppTeamExternalID string
	OperationRef      string
	ReservePoints     int64
	IdempotencyKey    string
	EffectiveAt       string
}

// CapturePointsRequest captures the actual consumed points from a reservation.
type CapturePointsRequest struct {
	CapturePoints  int64
	IdempotencyKey string
	EffectiveAt    string
}

// ReleasePointsRequest releases an unused reservation.
type ReleasePointsRequest struct {
	IdempotencyKey string
	EffectiveAt    string
}

// RefundPointsRequest refunds points from a captured transaction.
type RefundPointsRequest struct {
	RefundPoints   int64
	IdempotencyKey string
	EffectiveAt    string
}

type reservePointsRequestJSON struct {
	KeyID             string `json:"keyId"`
	Secret            string `json:"secret"`
	AppTeamExternalID string `json:"appTeamExternalId"`
	OperationRef      string `json:"operationRef"`
	ReservePoints     int64  `json:"reservePoints"`
	IdempotencyKey    string `json:"idempotencyKey"`
	EffectiveAt       string `json:"effectiveAt,omitempty"`
}

type pointTransactionCredentialRequest struct {
	KeyID  string `json:"keyId"`
	Secret string `json:"secret"`
}

type capturePointsRequestJSON struct {
	KeyID          string `json:"keyId"`
	Secret         string `json:"secret"`
	CapturePoints  int64  `json:"capturePoints"`
	IdempotencyKey string `json:"idempotencyKey"`
	EffectiveAt    string `json:"effectiveAt,omitempty"`
}

type releasePointsRequestJSON struct {
	KeyID          string `json:"keyId"`
	Secret         string `json:"secret"`
	IdempotencyKey string `json:"idempotencyKey"`
	EffectiveAt    string `json:"effectiveAt,omitempty"`
}

type refundPointsRequestJSON struct {
	KeyID          string `json:"keyId"`
	Secret         string `json:"secret"`
	RefundPoints   int64  `json:"refundPoints"`
	IdempotencyKey string `json:"idempotencyKey"`
	EffectiveAt    string `json:"effectiveAt,omitempty"`
}

// ReservePoints reserves digital employee points before the application performs work.
func (c *Client) ReservePoints(ctx context.Context, req ReservePointsRequest) (PointTransaction, error) {
	appTeamExternalID := strings.TrimSpace(req.AppTeamExternalID)
	operationRef := strings.TrimSpace(req.OperationRef)
	idempotencyKey := strings.TrimSpace(req.IdempotencyKey)
	if appTeamExternalID == "" {
		return PointTransaction{}, errors.New("octopus app sdk: app team external ID is required")
	}
	if operationRef == "" {
		return PointTransaction{}, errors.New("octopus app sdk: operation ref is required")
	}
	if req.ReservePoints <= 0 {
		return PointTransaction{}, errors.New("octopus app sdk: reserve points must be positive")
	}
	if idempotencyKey == "" {
		return PointTransaction{}, errors.New("octopus app sdk: idempotency key is required")
	}
	var out PointTransaction
	err := c.postJSON(ctx, pointTransactionsBasePath+"/reserve", reservePointsRequestJSON{KeyID: c.keyID, Secret: c.secret, AppTeamExternalID: appTeamExternalID, OperationRef: operationRef, ReservePoints: req.ReservePoints, IdempotencyKey: idempotencyKey, EffectiveAt: strings.TrimSpace(req.EffectiveAt)}, &out)
	return out, err
}

// GetPointTransaction queries a previous point transaction by Account transaction ID.
func (c *Client) GetPointTransaction(ctx context.Context, accountTransactionID string) (PointTransaction, error) {
	accountTransactionID = strings.TrimSpace(accountTransactionID)
	if accountTransactionID == "" {
		return PointTransaction{}, errors.New("octopus app sdk: account transaction ID is required")
	}
	var out PointTransaction
	err := c.postJSON(ctx, pointTransactionPath(accountTransactionID, "query"), pointTransactionCredentialRequest{KeyID: c.keyID, Secret: c.secret}, &out)
	return out, err
}

// CapturePoints captures actual consumed points from a reserved transaction.
func (c *Client) CapturePoints(ctx context.Context, accountTransactionID string, req CapturePointsRequest) (PointTransaction, error) {
	accountTransactionID = strings.TrimSpace(accountTransactionID)
	idempotencyKey := strings.TrimSpace(req.IdempotencyKey)
	if accountTransactionID == "" {
		return PointTransaction{}, errors.New("octopus app sdk: account transaction ID is required")
	}
	if req.CapturePoints <= 0 {
		return PointTransaction{}, errors.New("octopus app sdk: capture points must be positive")
	}
	if idempotencyKey == "" {
		return PointTransaction{}, errors.New("octopus app sdk: idempotency key is required")
	}
	var out PointTransaction
	err := c.postJSON(ctx, pointTransactionPath(accountTransactionID, "capture"), capturePointsRequestJSON{KeyID: c.keyID, Secret: c.secret, CapturePoints: req.CapturePoints, IdempotencyKey: idempotencyKey, EffectiveAt: strings.TrimSpace(req.EffectiveAt)}, &out)
	return out, err
}

// ReleasePoints releases an unused reserved transaction.
func (c *Client) ReleasePoints(ctx context.Context, accountTransactionID string, req ReleasePointsRequest) (PointTransaction, error) {
	accountTransactionID = strings.TrimSpace(accountTransactionID)
	idempotencyKey := strings.TrimSpace(req.IdempotencyKey)
	if accountTransactionID == "" {
		return PointTransaction{}, errors.New("octopus app sdk: account transaction ID is required")
	}
	if idempotencyKey == "" {
		return PointTransaction{}, errors.New("octopus app sdk: idempotency key is required")
	}
	var out PointTransaction
	err := c.postJSON(ctx, pointTransactionPath(accountTransactionID, "release"), releasePointsRequestJSON{KeyID: c.keyID, Secret: c.secret, IdempotencyKey: idempotencyKey, EffectiveAt: strings.TrimSpace(req.EffectiveAt)}, &out)
	return out, err
}

// RefundPoints refunds captured points from a transaction.
func (c *Client) RefundPoints(ctx context.Context, accountTransactionID string, req RefundPointsRequest) (PointTransaction, error) {
	accountTransactionID = strings.TrimSpace(accountTransactionID)
	idempotencyKey := strings.TrimSpace(req.IdempotencyKey)
	if accountTransactionID == "" {
		return PointTransaction{}, errors.New("octopus app sdk: account transaction ID is required")
	}
	if req.RefundPoints <= 0 {
		return PointTransaction{}, errors.New("octopus app sdk: refund points must be positive")
	}
	if idempotencyKey == "" {
		return PointTransaction{}, errors.New("octopus app sdk: idempotency key is required")
	}
	var out PointTransaction
	err := c.postJSON(ctx, pointTransactionPath(accountTransactionID, "refund"), refundPointsRequestJSON{KeyID: c.keyID, Secret: c.secret, RefundPoints: req.RefundPoints, IdempotencyKey: idempotencyKey, EffectiveAt: strings.TrimSpace(req.EffectiveAt)}, &out)
	return out, err
}

func pointTransactionPath(accountTransactionID string, action string) string {
	return pointTransactionsBasePath + "/" + accountTransactionID + "/" + action
}
