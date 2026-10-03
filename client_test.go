package octopusapp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

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

func TestPostJSONDecodesDataAndAPIError(t *testing.T) {
	var receivedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		if r.URL.Path == "/ok" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"value":"done"}}`))
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":"INVALID_REQUEST","message":"bad request"}}`))
	}))
	defer server.Close()

	client := testClient(t, server.URL)
	var out struct {
		Value string `json:"value"`
	}
	if err := client.postJSON(t.Context(), "/ok", map[string]string{"hello": "world"}, &out); err != nil {
		t.Fatalf("postJSON returned error: %v", err)
	}
	if receivedPath != "/ok" || out.Value != "done" {
		t.Fatalf("unexpected success decode path=%q out=%+v", receivedPath, out)
	}

	err := client.postJSON(t.Context(), "/bad", map[string]string{}, &out)
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T %v", err, err)
	}
	if apiErr.StatusCode != http.StatusBadRequest || apiErr.Code != "INVALID_REQUEST" || apiErr.Message != "bad request" {
		t.Fatalf("unexpected APIError: %+v", apiErr)
	}
}

func TestPostJSONMalformedResponseMentionsPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer server.Close()

	client := testClient(t, server.URL)
	var out struct{ Value string }
	err := client.postJSON(t.Context(), "/broken", map[string]string{}, &out)
	if err == nil || !strings.Contains(err.Error(), "/broken") {
		t.Fatalf("malformed response error should mention endpoint path, got %v", err)
	}
}

func TestPostJSONErrorDoesNotExposeSecret(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":"BAD_SECRET","message":"bad secret"}}`))
	}))
	defer server.Close()

	client := testClient(t, server.URL)
	var out struct{ Value string }
	err := client.postJSON(t.Context(), "/secret", map[string]string{"secret": client.secret}, &out)
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), client.secret) {
		t.Fatalf("error must not expose secret: %v", err)
	}
}

func testClient(t *testing.T, baseURL string) *Client {
	t.Helper()
	client, err := NewClient(ClientConfig{BaseURL: baseURL, KeyID: "key_123", Secret: "secret_123"})
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}
	return client
}

func TestExchangeOpenSessionPostsCredentialsAndDecodesContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/partner-apps/open-sessions/exchange" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body["keyId"] != "key_123" || body["secret"] != "secret_123" || body["openToken"] != "open_token_1" {
			t.Fatalf("unexpected request body: %+v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"identityTenantId":"identity_main","tenantId":"tenant_1","partnerId":"partner_1","applicationId":"app_1","bindingId":"binding_1","teamId":"team_1","userId":"user_1","teamRole":"admin","appTeamExternalId":"laoma_team_1","expiresAt":"2026-10-03T00:00:00Z"}}`))
	}))
	defer server.Close()

	client := testClient(t, server.URL)
	ctx, err := client.ExchangeOpenSession(t.Context(), " open_token_1 ")
	if ctx.IdentityTenantID != "identity_main" {
		t.Fatalf("identity tenant must not use application tenant: %+v", ctx)
	}
	if err != nil {
		t.Fatalf("ExchangeOpenSession returned error: %v", err)
	}
	if ctx.TenantID != "tenant_1" || ctx.TeamRole != "admin" || ctx.AppTeamExternalID != "laoma_team_1" {
		t.Fatalf("unexpected context: %+v", ctx)
	}
	if ctx.ExpiresAt.IsZero() {
		t.Fatalf("expiresAt should decode")
	}
}

func TestExchangeOpenSessionRequiresOpenToken(t *testing.T) {
	client := testClient(t, "https://octopus.example.com")
	if _, err := client.ExchangeOpenSession(t.Context(), " "); err == nil {
		t.Fatal("expected open token validation error")
	}
}

func TestPointTransactionLifecyclePostsExpectedRequests(t *testing.T) {
	requests := make([]string, 0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		requests = append(requests, r.URL.Path)
		if body["keyId"] != "key_123" || body["secret"] != "secret_123" {
			t.Fatalf("credentials missing from %s request: %+v", r.URL.Path, body)
		}
		switch r.URL.Path {
		case "/api/v1/partner-apps/point-transactions/reserve":
			if body["appTeamExternalId"] != "team_ext_1" || body["operationRef"] != "op_1" || body["idempotencyKey"] != "idem_reserve" || body["effectiveAt"] != "2026-10-03T00:00:00Z" || body["reservePoints"] != float64(100) {
				t.Fatalf("unexpected reserve body: %+v", body)
			}
		case "/api/v1/partner-apps/point-transactions/apt_1/query":
			// credentials only.
		case "/api/v1/partner-apps/point-transactions/apt_1/capture":
			if body["capturePoints"] != float64(80) || body["idempotencyKey"] != "idem_capture" {
				t.Fatalf("unexpected capture body: %+v", body)
			}
		case "/api/v1/partner-apps/point-transactions/apt_1/release":
			if body["idempotencyKey"] != "idem_release" {
				t.Fatalf("unexpected release body: %+v", body)
			}
		case "/api/v1/partner-apps/point-transactions/apt_1/refund":
			if body["refundPoints"] != float64(30) || body["idempotencyKey"] != "idem_refund" {
				t.Fatalf("unexpected refund body: %+v", body)
			}
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"transactionId":"tx_projection_1","tenantId":"tenant_1","partnerId":"partner_1","applicationId":"app_1","bindingId":"binding_1","teamId":"team_1","appTeamExternalId":"team_ext_1","operationRef":"op_1","accountTransactionId":"apt_1","status":"CAPTURED","reservedPoints":100,"capturedPoints":80,"refundedPoints":0,"createdAt":"2026-10-03T00:00:00Z","updatedAt":"2026-10-03T00:00:01Z"}}`))
	}))
	defer server.Close()

	client := testClient(t, server.URL)
	reserve, err := client.ReservePoints(t.Context(), ReservePointsRequest{AppTeamExternalID: "team_ext_1", OperationRef: "op_1", ReservePoints: 100, IdempotencyKey: "idem_reserve", EffectiveAt: "2026-10-03T00:00:00Z"})
	if err != nil {
		t.Fatalf("ReservePoints returned error: %v", err)
	}
	if reserve.AccountTransactionID != "apt_1" || reserve.CapturedPoints != 80 {
		t.Fatalf("unexpected reserve transaction: %+v", reserve)
	}
	if _, err := client.GetPointTransaction(t.Context(), "apt_1"); err != nil {
		t.Fatalf("GetPointTransaction returned error: %v", err)
	}
	if _, err := client.CapturePoints(t.Context(), "apt_1", CapturePointsRequest{CapturePoints: 80, IdempotencyKey: "idem_capture"}); err != nil {
		t.Fatalf("CapturePoints returned error: %v", err)
	}
	if _, err := client.ReleasePoints(t.Context(), "apt_1", ReleasePointsRequest{IdempotencyKey: "idem_release"}); err != nil {
		t.Fatalf("ReleasePoints returned error: %v", err)
	}
	if _, err := client.RefundPoints(t.Context(), "apt_1", RefundPointsRequest{RefundPoints: 30, IdempotencyKey: "idem_refund"}); err != nil {
		t.Fatalf("RefundPoints returned error: %v", err)
	}

	expected := []string{
		"/api/v1/partner-apps/point-transactions/reserve",
		"/api/v1/partner-apps/point-transactions/apt_1/query",
		"/api/v1/partner-apps/point-transactions/apt_1/capture",
		"/api/v1/partner-apps/point-transactions/apt_1/release",
		"/api/v1/partner-apps/point-transactions/apt_1/refund",
	}
	if !reflect.DeepEqual(requests, expected) {
		t.Fatalf("unexpected request paths:\nwant %#v\n got %#v", expected, requests)
	}
}

func TestPointTransactionValidation(t *testing.T) {
	client := testClient(t, "https://octopus.example.com")
	if _, err := client.ReservePoints(t.Context(), ReservePointsRequest{OperationRef: "op", ReservePoints: 1, IdempotencyKey: "idem"}); err == nil {
		t.Fatal("ReservePoints should require app team external ID")
	}
	if _, err := client.ReservePoints(t.Context(), ReservePointsRequest{AppTeamExternalID: "team", ReservePoints: 1, IdempotencyKey: "idem"}); err == nil {
		t.Fatal("ReservePoints should require operation ref")
	}
	if _, err := client.ReservePoints(t.Context(), ReservePointsRequest{AppTeamExternalID: "team", OperationRef: "op", ReservePoints: 0, IdempotencyKey: "idem"}); err == nil {
		t.Fatal("ReservePoints should require positive points")
	}
	if _, err := client.ReservePoints(t.Context(), ReservePointsRequest{AppTeamExternalID: "team", OperationRef: "op", ReservePoints: 1}); err == nil {
		t.Fatal("ReservePoints should require idempotency key")
	}
	if _, err := client.GetPointTransaction(t.Context(), " "); err == nil {
		t.Fatal("GetPointTransaction should require account transaction ID")
	}
	if _, err := client.CapturePoints(t.Context(), "apt", CapturePointsRequest{CapturePoints: 0, IdempotencyKey: "idem"}); err == nil {
		t.Fatal("CapturePoints should require positive points")
	}
	if _, err := client.CapturePoints(t.Context(), "apt", CapturePointsRequest{CapturePoints: 1}); err == nil {
		t.Fatal("CapturePoints should require idempotency key")
	}
	if _, err := client.ReleasePoints(t.Context(), "apt", ReleasePointsRequest{}); err == nil {
		t.Fatal("ReleasePoints should require idempotency key")
	}
	if _, err := client.RefundPoints(t.Context(), "apt", RefundPointsRequest{RefundPoints: 0, IdempotencyKey: "idem"}); err == nil {
		t.Fatal("RefundPoints should require positive points")
	}
}

func TestExchangeOpenSessionRejectsMissingIdentityTenant(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"tenantId":"application-tenant","userId":"user"}}`))
	}))
	defer server.Close()
	_, err := testClient(t, server.URL).ExchangeOpenSession(t.Context(), "token")
	if err == nil {
		t.Fatal("application tenant must never substitute identity tenant")
	}
}
