# Octopus App SDK for Go

Go SDK for applications integrating with Octopus digital employee application access and point transactions.

This package is intentionally independent from BotWorks source code. It uses the public partner application API and standard Go HTTP primitives.

## Install

```bash
go get github.com/thecybersailor/octopus-app-sdk-go
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

// Use ctx.UserID, ctx.TeamID, ctx.TeamRole and ctx.AppTeamExternalID
// to find or create your local user/member records, then issue your own app session.
```

The SDK returns trusted platform context. Your application still owns local user records, team/member records, business permissions and session issuance.

## Status

Open session exchange is available. Point transaction methods are added in a later P07 task.

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
