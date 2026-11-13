# Octopus App SDK for Go

Go SDK for applications integrating with Octopus digital employee application access and point transactions.

This package is intentionally independent from BotWorks source code. It uses the public partner application API and standard Go HTTP primitives.

## Install

```bash
go get github.com/thecybersailor/octopus-app-sdk-go@v0.4.0
```

## Create a client

```go
client, err := octopusapp.NewClient(octopusapp.ClientConfig{
    BaseURL: "https://your-octopus-host.example.com",
    KeyID:   "your-key-id",
    Secret:  "your-secret",
})
```

## Exchange an application open token

When a user opens your app from the digital employee application page, your backend receives an `openToken`. Exchange it on the server:

```go
ctx, err := client.ExchangeOpenSession(request.Context(), openToken)
if err != nil {
    // Return a business-facing retry or expired-link message.
}

// Resolve an independent local identity using the fixed provider,
// ctx.IdentityTenantID and ctx.UserID (never native phone/email matching).
// Then ensure membership using ctx.TeamID, ctx.TeamRole and ctx.AppTeamExternalID.
// Issue a session for the local identity ID, not the platform UserID.
```

`ctx.ArcubaseUserID` (available from v0.4.0) identifies the human employee who
opened the application within `ctx.TeamID`. It is a numeric string: store and
compare it as a string, never as a number. Use `(TeamID, ArcubaseUserID)` for
employee associations and `(IdentityTenantID, UserID)` for platform account
mapping. The launch URL hint `octopus_arcubase_user_id` can be edited by the
browser; trust only the exchanged value.

The exchange fails when the employee identity is missing. Platforms reject the
open request with `ARCUBASE_USER_BINDING_REQUIRED` when the member has no ready
employee binding, and reject old tokens without an employee snapshot with
`INVALID_REQUEST`. In both cases, ask the user to reopen the application from
the team app list after the team administrator enables employee access.

The SDK returns trusted platform context. Your application still owns local user records, team/member records, business permissions and session issuance.

## Status

v0.4.0 includes open-session exchange with the human employee identity (`arcubaseUserId`), point transaction lifecycle methods, and per-cooperation runtime API/MCP access verification. Runtime access requires a platform deployment implementing `/api/v1/partner-apps/access/verify`, and the employee identity requires a platform deployment that returns `arcubaseUserId` from open-session exchange; publishing the SDK does not deploy either.

## Point transaction lifecycle

Applications send digital employee point amounts to Octopus. If your product has its own app points, apply your own conversion ratio before calling the SDK. Developer revenue share is an offline agreement and is not configured, calculated or displayed by this SDK or the platform API.

```go
reservation, err := client.ReservePoints(ctx, octopusapp.ReservePointsRequest{
    AppTeamExternalID: "your-local-team-id",
    OperationRef:      "job-123",
    ReservePoints:     100,
    IdempotencyKey:    "job-123:reserve",
})
if err != nil {
    // Do not continue the billable operation until the reservation succeeds.
}

// If the result of a later call is unknown, query the original transaction
// before retrying or releasing. Reuse the same operation and idempotency keys.
current, err := client.GetPointTransaction(ctx, reservation.AccountTransactionID)
_ = current

captured, err := client.CapturePoints(ctx, reservation.AccountTransactionID, octopusapp.CapturePointsRequest{
    CapturePoints:  80,
    IdempotencyKey: "job-123:capture",
})
_ = captured

_, _ = client.ReleasePoints(ctx, reservation.AccountTransactionID, octopusapp.ReleasePointsRequest{
    IdempotencyKey: "job-123:release-unused",
})

_, _ = client.RefundPoints(ctx, reservation.AccountTransactionID, octopusapp.RefundPointsRequest{
    RefundPoints:   30,
    IdempotencyKey: "job-123:refund-1",
})
```

Reserve before work, capture the actual consumed amount after work succeeds, release unused reservations, and refund only after a captured transaction needs reversal. A timeout is not proof of failure; query the transaction before retrying with the same idempotency key.

## Example

Run the basic example without sending a request:

```bash
OCTOPUS_BASE_URL=https://octopus.example.com OCTOPUS_APP_KEY_ID=key_xxx OCTOPUS_APP_SECRET=secret_xxx go run ./examples/basic
```

Add `OCTOPUS_OPEN_TOKEN` only when handling a real application-open callback on your backend. The example never prints the app secret.

## Local consumer verification

Before publishing this module, local verification can use a temporary consumer with a `replace` directive pointing at this checkout. Published consumers should import `github.com/thecybersailor/octopus-app-sdk-go` without `replace`.

## Identity isolation and first access

First creation of a digital employee identity must start from the Octopus application entry and a server-validated one-time open token. Website QR login may only resolve an already provisioned identity. Keep native phone accounts separate even when phone numbers match; never use phone/email to reuse a native user, or pass the platform user ID as a local account ID.

Use the authenticated identity tenant, distinct from the application ownership tenant, together with provider, subject and local instance as the stable identity key. Reuse that independent identity across apps; ensure membership separately for each valid binding. Existing native team hosting requires verified ownership and explicit admin selection, and never merges account identities or rewrites historical authors. Reject missing identity context and never fall back to native login, default teams or billing.

## Market cooperation and automatic API/MCP access

An application has no application-version hierarchy. Each application × market
cooperation owns its own code, connection settings, platform key and consumption
bill. Create one `Client` per cooperation. Select it by an explicit configured
cooperation ID; never try other keys if verification fails.

`VerifyAccess` (available from v0.3.0) verifies a short-lived platform
execution token. Unlike `ExchangeOpenSession`, it does not create a browser login.

```go
access, err := client.VerifyAccess(request.Context(), bearerToken)
if err != nil {
    // Deny the request. Do not fall back to native or another market's identity.
    return
}
if access.CooperationID != configuredCooperationID ||
    access.MarketID != configuredMarketID || !access.AllowsMCPMethod(method) {
    return
}
// Resolve your immutable local binding using access.BindingID, access.TeamID,
// and access.AppTeamExternalID. Scope every query to that team.
// access.DigiEmployeeID is an execution principal, not a human administrator.
```

Verify each incoming request. The platform checks current cooperation, binding,
registration, employee and credential state. Discovery/probe credentials cannot
execute business operations. `AllowsMCPMethod` permits initialization and the
metadata lookup required by the authorized operation; it does not allow a token
for `tools/call` to perform `resources/read`.

## Idempotent team provisioning

Register an HTTPS provisioning URL in the cooperation's connection settings. The
platform posts this body using the application-provided service Bearer secret:

```json
{
  "cooperationId": "papp_example",
  "marketId": "market_example",
  "bindingId": "papp_binding_example",
  "teamId": "team_example",
  "teamName": "Example team"
}
```

Authenticate the service secret and compare cooperation/market with the selected
configuration before changing data. Lock a persistent operation unique on
`(cooperationId, teamId)`, retain `bindingId`, create the local team and immutable
binding in one transaction, and return the same result for retries:

```json
{"data":{"bindingId":"papp_binding_example","appTeamExternalId":"local_team","appTeamName":"Example team"}}
```

No human account is created here. Ensure users on the existing verified open-session
path. A different binding ID for the same cooperation/team must fail. If the
platform sends `existingAppTeamExternalId`, verify an explicitly mapped local
binding; never use that field to claim an unbound native team or match by phone.

Runtime API/MCP tokens and one-time login tokens have no user-managed settings.
Application-to-digital-employee point conversion remains an application configuration;
developer revenue sharing stays outside the platform consumption bill.
