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
