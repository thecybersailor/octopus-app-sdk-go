package octopusapp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestVerifyAccessUsesOnlyConfiguredCooperation(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if r.URL.Path != "/api/v1/partner-apps/access/verify" || body["keyId"] != "key_123" || body["secret"] != "secret_123" || body["accessToken"] != "pat_test" {
			t.Errorf("unexpected request path or credentials")
		}
		json.NewEncoder(w).Encode(map[string]any{"data": AccessContext{CooperationID: "coop", MarketID: "market", BindingID: "binding", TeamID: "team", AppTeamExternalID: "app_team", DigiEmployeeID: "employee", Method: "tools/call", ExpiresAt: time.Now().Add(time.Minute)}})
	}))
	defer server.Close()
	c := testClient(t, server.URL)
	if _, err := c.VerifyAccess(t.Context(), ""); err == nil {
		t.Fatal("empty token must fail")
	}
	out, err := c.VerifyAccess(t.Context(), "pat_test")
	if err != nil {
		t.Fatal(err)
	}
	if out.AppTeamExternalID != "app_team" || calls != 1 {
		t.Fatal("unexpected access context or request count")
	}
	if !out.AllowsMCPMethod("tools/call") || !out.AllowsMCPMethod("tools/list") || !out.AllowsMCPMethod("initialize") || out.AllowsMCPMethod("resources/read") {
		t.Fatal("method scope not enforced")
	}
	out.Probe = true
	if out.AllowsMCPMethod("tools/call") {
		t.Fatal("probe allowed business call")
	}
	out.ExpiresAt = time.Now().Add(-time.Minute)
	if out.AllowsMCPMethod("initialize") {
		t.Fatal("expired credential allowed")
	}
}

func TestVerifyAccessRejectsIncompleteContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"data":{"cooperationId":"a"}}`)) }))
	defer server.Close()
	if _, err := testClient(t, server.URL).VerifyAccess(t.Context(), "pat_test"); err == nil {
		t.Fatal("incomplete identity accepted")
	}
}
