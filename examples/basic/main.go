package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	octopusapp "github.com/thecybersailor/octopus-app-sdk-go"
)

func main() {
	baseURL := strings.TrimSpace(os.Getenv("OCTOPUS_BASE_URL"))
	keyID := strings.TrimSpace(os.Getenv("OCTOPUS_APP_KEY_ID"))
	secret := strings.TrimSpace(os.Getenv("OCTOPUS_APP_SECRET"))
	if baseURL == "" || keyID == "" || secret == "" {
		fmt.Println("Set OCTOPUS_BASE_URL, OCTOPUS_APP_KEY_ID and OCTOPUS_APP_SECRET to call Octopus.")
		fmt.Println("Optional: set OCTOPUS_OPEN_TOKEN to exchange an application open token.")
		return
	}
	client, err := octopusapp.NewClient(octopusapp.ClientConfig{BaseURL: baseURL, KeyID: keyID, Secret: secret})
	if err != nil {
		fmt.Fprintf(os.Stderr, "create client: %v\n", err)
		os.Exit(1)
	}
	openToken := strings.TrimSpace(os.Getenv("OCTOPUS_OPEN_TOKEN"))
	if openToken == "" {
		fmt.Println("Client configured. No OCTOPUS_OPEN_TOKEN provided, so no request was sent.")
		return
	}
	ctx, err := client.ExchangeOpenSession(context.Background(), openToken)
	if err != nil {
		fmt.Fprintf(os.Stderr, "exchange open session: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Open session exchanged for team %s, app team %s, user %s, role %s\n", ctx.TeamID, ctx.AppTeamExternalID, ctx.UserID, ctx.TeamRole)
}
