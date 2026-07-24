package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mdemg/internal/sanitize"
	"net/http"
	"time"

	"github.com/spf13/cobra"
)

func newDemoCmd() *cobra.Command {
	var endpoint string

	cmd := &cobra.Command{
		Use:   "demo",
		Short: "Seed sample data and demonstrate MDEMG features",
		Long: `Run an interactive demo that seeds sample observations into a demo space
and demonstrates recall, consolidation, and knowledge graph features.

Requires a running MDEMG server (start with: mdemg start --auto-migrate).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDemo(endpoint)
		},
	}

	cmd.Flags().StringVar(&endpoint, "endpoint", "http://localhost:9999", "MDEMG server endpoint")

	return cmd
}

type demoObservation struct {
	Content string `json:"content"`
	ObsType string `json:"obs_type"`
}

func runDemo(endpoint string) error {
	spaceID := "mdemg-demo"

	fmt.Println("MDEMG Demo")
	fmt.Println("==========")
	fmt.Println()

	// Check server health
	fmt.Print("Checking server... ")
	if err := demoHealthCheck(endpoint); err != nil {
		fmt.Println("FAILED")
		fmt.Println()
		fmt.Println("Server not reachable. Start it with:")
		fmt.Println("  mdemg start --auto-migrate")
		return err
	}
	fmt.Println("ok")
	fmt.Println()

	// Seed sample observations
	observations := []demoObservation{
		{Content: "The authentication service uses JWT tokens with RS256 signing. Tokens expire after 1 hour and refresh tokens after 7 days.", ObsType: "learning"},
		{Content: "Database connection pooling is configured with max 20 connections. Under load testing, we saw timeouts above 15 concurrent users.", ObsType: "learning"},
		{Content: "User requested that all API responses include request-id headers for tracing. This is now a hard requirement.", ObsType: "preference"},
		{Content: "The caching layer uses Redis with a 5-minute TTL for user profiles and 1-hour TTL for configuration data.", ObsType: "learning"},
		{Content: "Fixed bug where batch imports over 1000 records caused OOM. Solution: stream processing with 100-record chunks.", ObsType: "decision"},
		{Content: "The deployment pipeline runs: lint → unit tests → integration tests → canary deploy → full rollout. Never skip the canary step.", ObsType: "decision"},
		{Content: "GraphQL subscriptions use WebSocket transport. Initial implementation used polling but was changed to push-based for lower latency.", ObsType: "learning"},
		{Content: "Rate limiting is set to 100 requests per minute per API key. Enterprise tier gets 1000 req/min.", ObsType: "learning"},
	}

	fmt.Printf("Seeding %d observations into space '%s'...\n", len(observations), spaceID)
	fmt.Println()

	for i, obs := range observations {
		fmt.Printf("  [%d/%d] %s: %.60s", i+1, len(observations), obs.ObsType, obs.Content)
		if len(obs.Content) > 60 {
			fmt.Print("...")
		}

		body, _ := json.Marshal(map[string]string{
			"space_id":   spaceID,
			"session_id": "demo",
			"content":    obs.Content,
			"obs_type":   obs.ObsType,
		})

		resp, err := http.Post(endpoint+"/v1/conversation/observe", "application/json", bytes.NewReader(body))
		if err != nil {
			fmt.Println(" FAILED")
			return fmt.Errorf("observe: %w", err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			fmt.Printf(" FAILED (HTTP %d)\n", resp.StatusCode)
			return fmt.Errorf("observe returned %d", resp.StatusCode)
		}
		fmt.Println(" ✓")
	}

	fmt.Println()

	// Demo recall
	queries := []string{
		"How does authentication work?",
		"What are the rate limits?",
		"How do we handle large data imports?",
	}

	fmt.Println("Demonstrating semantic recall...")
	fmt.Println()

	for _, query := range queries {
		fmt.Printf("  Query: %q\n", query)

		body, _ := json.Marshal(map[string]any{
			"space_id":   spaceID,
			"query_text": query,
			"top_k":      3,
		})

		resp, err := http.Post(endpoint+"/v1/memory/retrieve", "application/json", bytes.NewReader(body))
		if err != nil {
			fmt.Printf("    Error: %v\n\n", err)
			continue
		}

		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			fmt.Printf("    Error: HTTP %d\n\n", resp.StatusCode)
			continue
		}

		var result struct {
			Nodes []struct {
				Content string  `json:"content"`
				Score   float64 `json:"score"`
			} `json:"nodes"`
		}
		if err := json.Unmarshal(respBody, &result); err != nil {
			fmt.Printf("    Error parsing response: %v\n\n", err)
			continue
		}

		if len(result.Nodes) == 0 {
			fmt.Println("    No results (embedding provider may not be configured)")
		}
		for j, node := range result.Nodes {
			content := sanitize.CutRuneSafeSuffix(node.Content, 80, "...")
			fmt.Printf("    %d. [%.3f] %s\n", j+1, node.Score, content)
		}
		fmt.Println()
	}

	fmt.Println("Demo complete!")
	fmt.Println()
	fmt.Printf("The demo space '%s' contains sample data you can explore:\n", spaceID)
	fmt.Println("  mdemg space list                    — list all spaces")
	fmt.Printf("  curl localhost:9999/v1/memory/retrieve — query the '%s' space\n", spaceID)
	fmt.Println()
	fmt.Println("To clean up:")
	fmt.Printf("  curl -X DELETE localhost:9999/v1/spaces/%s\n", spaceID)

	return nil
}

func demoHealthCheck(endpoint string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(endpoint + "/healthz")
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned %d", resp.StatusCode)
	}
	return nil
}
