package octopusapp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
		_, _ = w.Write([]byte(`{"data":{"tenantId":"tenant_1","partnerId":"partner_1","applicationId":"app_1","bindingId":"binding_1","teamId":"team_1","userId":"user_1","teamRole":"admin","appTeamExternalId":"laoma_team_1","expiresAt":"2026-10-03T00:00:00Z"}}`))
	}))
	defer server.Close()

	client := testClient(t, server.URL)
	ctx, err := client.ExchangeOpenSession(t.Context(), " open_token_1 ")
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
