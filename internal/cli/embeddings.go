package cli

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/spf13/cobra"
	"mdemg/internal/embeddings"
)

func newEmbeddingsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "embeddings",
		Short: "Embedding provider management",
		Long:  "Commands for managing and testing embedding providers.",
	}

	cmd.AddCommand(newEmbeddingsCheckCmd())

	return cmd
}

func newEmbeddingsCheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Verify the configured embedding provider",
		Long: `Check that the configured embedding provider is working correctly.

Unlike 'config validate' which only probes connectivity, this command
performs an actual test embedding to verify the full pipeline works.

Reports provider type, model, endpoint reachability, and embedding dimensions.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return fmt.Errorf("config error: %w", err)
			}

			embCfg := embeddings.Config{
				Provider:       cfg.EmbeddingProvider,
				OpenAIAPIKey:   cfg.OpenAIAPIKey,
				OpenAIModel:    cfg.OpenAIModel,
				OpenAIEndpoint: cfg.OpenAIEndpoint,
				OllamaEndpoint: cfg.OllamaEndpoint,
				OllamaModel:    cfg.OllamaModel,
			}

			fmt.Println("Embedding Provider Check")
			fmt.Println("========================")

			if embCfg.Provider == "" || embCfg.Provider == "none" {
				fmt.Println()
				fmt.Println("Status: disabled (no provider configured)")
				fmt.Println()
				fmt.Println("To enable embeddings, set one of:")
				fmt.Println("  Ollama:  EMBEDDING_PROVIDER=ollama")
				fmt.Println("  OpenAI:  EMBEDDING_PROVIDER=openai + OPENAI_API_KEY=...")
				fmt.Println()
				fmt.Println("Or in .mdemg/config.yaml:")
				fmt.Println("  embedding:")
				fmt.Println("    provider: ollama")
				fmt.Println("    model: nomic-embed-text")
				return nil
			}

			fmt.Printf("Provider: %s\n", embCfg.Provider)

			switch embCfg.Provider {
			case "ollama":
				return checkOllama(embCfg)
			case "openai":
				return checkOpenAI(embCfg)
			default:
				return fmt.Errorf("unknown provider: %s", embCfg.Provider)
			}
		},
	}
}

func checkOllama(cfg embeddings.Config) error {
	endpoint := cfg.OllamaEndpoint
	if endpoint == "" {
		endpoint = "http://localhost:11434"
	}
	model := cfg.OllamaModel
	if model == "" {
		model = "nomic-embed-text"
	}

	fmt.Printf("Endpoint: %s\n", endpoint)
	fmt.Printf("Model:    %s\n", model)
	fmt.Println()

	// TCP connectivity check
	fmt.Print("Connectivity... ")
	host, port, _ := net.SplitHostPort(endpoint[len("http://"):])
	if port == "" {
		port = "11434"
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 5*time.Second)
	if err != nil {
		fmt.Println("FAILED")
		fmt.Printf("\nCannot reach Ollama at %s\n", endpoint)
		fmt.Println("Ensure Ollama is running: ollama serve")
		fmt.Println("Install: https://ollama.com")
		return fmt.Errorf("ollama unreachable: %w", err)
	}
	_ = conn.Close()
	fmt.Println("ok")

	// Model availability check
	fmt.Print("Model check... ")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(endpoint + "/api/tags")
	if err != nil {
		fmt.Println("FAILED")
		return fmt.Errorf("list models: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		fmt.Println("ok")
	} else {
		fmt.Printf("warning (status %d)\n", resp.StatusCode)
	}

	// Actual test embedding
	fmt.Print("Test embedding... ")
	embedder, err := embeddings.NewOllama(cfg)
	if err != nil {
		fmt.Println("FAILED")
		return fmt.Errorf("create embedder: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	vec, err := embedder.Embed(ctx, "test embedding for verification")
	if err != nil {
		fmt.Println("FAILED")
		fmt.Printf("\nEmbedding failed: %v\n", err)
		fmt.Printf("Ensure model is pulled: ollama pull %s\n", model)
		return fmt.Errorf("test embed failed: %w", err)
	}
	fmt.Println("ok")

	fmt.Println()
	fmt.Printf("Dimensions: %d\n", len(vec))
	fmt.Println("Status:     working")
	return nil
}

func checkOpenAI(cfg embeddings.Config) error {
	endpoint := cfg.OpenAIEndpoint
	if endpoint == "" {
		endpoint = "https://api.openai.com/v1"
	}
	model := cfg.OpenAIModel
	if model == "" {
		model = "text-embedding-ada-002"
	}

	fmt.Printf("Endpoint: %s\n", endpoint)
	fmt.Printf("Model:    %s\n", model)

	if cfg.OpenAIAPIKey == "" {
		fmt.Println()
		fmt.Println("Status: FAILED")
		fmt.Println("OPENAI_API_KEY is not set")
		return fmt.Errorf("OPENAI_API_KEY required for openai provider")
	}

	maskedKey := cfg.OpenAIAPIKey[:4] + "****" + cfg.OpenAIAPIKey[len(cfg.OpenAIAPIKey)-4:]
	fmt.Printf("API Key:  %s\n", maskedKey)
	fmt.Println()

	// Actual test embedding
	fmt.Print("Test embedding... ")
	embedder, err := embeddings.NewOpenAI(cfg)
	if err != nil {
		fmt.Println("FAILED")
		return fmt.Errorf("create embedder: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	vec, err := embedder.Embed(ctx, "test embedding for verification")
	if err != nil {
		fmt.Println("FAILED")
		fmt.Printf("\nEmbedding failed: %v\n", err)
		return fmt.Errorf("test embed failed: %w", err)
	}
	fmt.Println("ok")

	fmt.Println()
	fmt.Printf("Dimensions: %d\n", len(vec))
	fmt.Println("Status:     working")
	return nil
}
