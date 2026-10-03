package octopusapp

import (
	"context"
	"errors"
	"strings"
	"time"
)

// AccessContext identifies a currently authorized application execution. It is
// not a browser login or a human administrator. Enforce local business permissions
// using AppTeamExternalID, DigiEmployeeID and Method; never trust request team IDs.
type AccessContext struct {
	CooperationID     string `json:"cooperationId"`
	MarketID          string `json:"marketId"`
	BindingID         string `json:"bindingId"`
	TeamID            string `json:"teamId"`
	AppTeamExternalID string `json:"appTeamExternalId"`
	DigiEmployeeID    string `json:"digiEmployeeId,omitempty"`
	Method            string `json:"method"`
	Probe             bool   `json:"probe"`
	ExpiresAt         Time   `json:"expiresAt"`
}

// VerifyAccess validates a short-lived platform credential with this client's
// cooperation key. Call for every application request; do not cache across requests
// or try keys from other markets on failure.
func (c *Client) VerifyAccess(ctx context.Context, token string) (AccessContext, error) {
	var out AccessContext
	token = strings.TrimSpace(token)
	if token == "" {
		return out, errors.New("octopus app sdk: access token is required")
	}
	req := struct {
		KeyID       string `json:"keyId"`
		Secret      string `json:"secret"`
		AccessToken string `json:"accessToken"`
	}{c.keyID, c.secret, token}
	if err := c.postJSON(ctx, "/api/v1/partner-apps/access/verify", req, &out); err != nil {
		return AccessContext{}, err
	}
	if out.CooperationID == "" || out.MarketID == "" || out.BindingID == "" || out.TeamID == "" || out.AppTeamExternalID == "" || out.Method == "" || (!out.Probe && out.DigiEmployeeID == "") || !time.Now().Before(out.ExpiresAt) {
		return AccessContext{}, errors.New("octopus app sdk: invalid or expired application access context")
	}
	return out, nil
}

// AllowsMCPMethod permits transport initialization and only the authorized
// operation. Discovery permits metadata reads, never application business calls.
func (c AccessContext) AllowsMCPMethod(method string) bool {
	if !time.Now().Before(c.ExpiresAt) {
		return false
	}
	switch method {
	case "initialize", "notifications/initialized", "ping":
		return true
	case "tools/call", "resources/read":
		return !c.Probe && c.Method == method
	case "tools/list":
		return c.Method == method || c.Method == "server/discover" || c.Method == "tools/call"
	case "resources/list", "resources/templates/list":
		return c.Method == method || c.Method == "server/discover" || c.Method == "resources/read"
	default:
		return false
	}
}
