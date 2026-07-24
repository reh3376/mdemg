package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mdemg/internal/sanitize"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"mdemg/internal/config"
)

// Sprint MODEL-DIST-001 follow-up #1 — `mdemg model run`.
//
// Talks to the existing LLM_ENDPOINT (default: llama-server at port 8102
// per Phase 13.5) via the OpenAI-compatible `/chat/completions` route.
// Two modes:
//
//   - One-shot: `mdemg model run -p "hello"` — sends one prompt, prints
//     the assistant response, exits.
//   - Interactive REPL: `mdemg model run` with no prompt — reads stdin
//     line-by-line, accumulates conversation history, prints responses.
//     Ctrl-D / Ctrl-C exits.
//
// Pure stdlib HTTP (no llmclient retries/breakers/recording) — this is an
// ad-hoc exploration tool, not a production code path. CLI invocations are
// intentionally NOT recorded to llm_interactions to keep training-data
// corpus clean.
//
// Per the no-hardcoding rule (memory: feedback_no_hardcoded_values.md),
// every operator-visible value flows through cfg + flags with sensible
// defaults. See newModelRunCmd flags below for the full surface.

// runOverrides captures the run-specific flag values. Resolved against
// cfg via resolveRunConfig().
type runOverrides struct {
	endpoint    string
	model       string
	prompt      string
	system      string
	temperature float64
	maxTokens   int
	timeout     time.Duration
}

func newModelRunCmd() *cobra.Command {
	var o runOverrides
	cmd := &cobra.Command{
		Use:   "run [-- <prompt>]",
		Short: "Chat with the running LLM endpoint (one-shot or interactive REPL)",
		Long: `Send a chat completion request to the configured LLM endpoint
(default: llama-server at $LLM_ENDPOINT, port 8102 in Phase 13.5
production).

Two modes:
  - One-shot: pass --prompt/-p (or a positional argument after --) to
    send a single message and print the response.
  - Interactive REPL: no prompt provided; reads stdin line-by-line and
    accumulates conversation history.

This is an ad-hoc exploration tool. CLI invocations are not recorded to
llm_interactions; use the mdemg server's HTTP API for tracked calls.

Examples:
  mdemg model run -p "Explain the no-hardcoding rule in 2 sentences"
  mdemg model run --system "You are a Go expert" -p "What is errgroup?"
  echo "summarize Phase 13.5" | mdemg model run -- "$(</dev/stdin)"
  mdemg model run                 # interactive REPL`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			// Positional argument is treated as the prompt if --prompt
			// wasn't explicitly set.
			if o.prompt == "" && len(args) > 0 {
				o.prompt = strings.Join(args, " ")
			}
			return runModelRun(cmd.Context(), cfg, o)
		},
	}
	cmd.Flags().StringVarP(&o.prompt, "prompt", "p", "", "one-shot prompt (omit to enter interactive REPL)")
	cmd.Flags().StringVarP(&o.system, "system", "s", "", "system message prepended to the conversation")
	cmd.Flags().StringVar(&o.endpoint, "endpoint", "", "LLM endpoint override (default from cfg.EffectiveLLMEndpoint)")
	cmd.Flags().StringVar(&o.model, "model", "", "model name to send in the request (default from cfg.LLMModel)")
	cmd.Flags().Float64Var(&o.temperature, "temperature", 0.7, "sampling temperature")
	cmd.Flags().IntVar(&o.maxTokens, "max-tokens", 1024, "max tokens to generate")
	cmd.Flags().DurationVar(&o.timeout, "timeout", 60*time.Second, "per-request timeout")
	return cmd
}

// chatMessage mirrors the OpenAI-compatible chat completion message shape.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatRequest is the request body sent to <endpoint>/chat/completions.
type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

// chatResponse is the relevant subset of the OpenAI chat response.
type chatResponse struct {
	Choices []struct {
		Message      chatMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

// resolveRunConfig applies flag → env → default precedence.
func resolveRunConfig(cfg config.Config, o runOverrides) runOverrides {
	out := o
	if out.endpoint == "" {
		out.endpoint = cfg.EffectiveLLMEndpoint()
	}
	if out.model == "" {
		out.model = cfg.LLMModel
	}
	if out.model == "" {
		// Final fallback: the production model name. Operators with a
		// non-default model must set --model or LLM_MODEL in env.
		out.model = "mdemg-llm-v1"
	}
	return out
}

// callChat sends one POST to <endpoint>/chat/completions and returns the
// assistant message content (choices[0].message.content). Returns the
// parsed response so callers can also access usage stats.
func callChat(ctx context.Context, endpoint string, body chatRequest, timeout time.Duration) (*chatResponse, error) {
	url := strings.TrimRight(endpoint, "/") + "/chat/completions"
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	rctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(rctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("POST %s: %w", url, err)
	}
	defer resp.Body.Close()
	rawBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("POST %s: status %d: %s", url, resp.StatusCode, truncateRunBody(string(rawBody), 500))
	}
	var parsed chatResponse
	if err := json.Unmarshal(rawBody, &parsed); err != nil {
		return nil, fmt.Errorf("decode response: %w (body: %s)", err, truncateRunBody(string(rawBody), 500))
	}
	if parsed.Error != nil {
		return nil, fmt.Errorf("LLM endpoint error: %s (%s)", parsed.Error.Message, parsed.Error.Type)
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("LLM endpoint returned no choices (body: %s)", truncateRunBody(string(rawBody), 500))
	}
	return &parsed, nil
}

// truncateRunBody bounds error/log body strings. Named to avoid colliding
// with a same-named helper in data.go.
func truncateRunBody(s string, max int) string {
	return sanitize.CutRuneSafeSuffix(s, max, "...(truncated)")
}

// runModelRun is the command entry point.
func runModelRun(ctx context.Context, cfg config.Config, o runOverrides) error {
	r := resolveRunConfig(cfg, o)
	fmt.Fprintf(os.Stderr, "endpoint=%s model=%s timeout=%s\n", r.endpoint, r.model, r.timeout)

	if r.prompt != "" {
		// One-shot mode.
		msgs := buildMessages(r.system, nil, r.prompt)
		resp, err := callChat(ctx, r.endpoint, chatRequest{
			Model:       r.model,
			Messages:    msgs,
			Temperature: r.temperature,
			MaxTokens:   r.maxTokens,
		}, r.timeout)
		if err != nil {
			return err
		}
		fmt.Println(resp.Choices[0].Message.Content)
		return nil
	}

	// Interactive REPL.
	return runREPL(ctx, r)
}

// buildMessages composes the chat-completion messages slice: optional
// system message first, then any prior turns, then the new user prompt.
func buildMessages(system string, history []chatMessage, userPrompt string) []chatMessage {
	out := make([]chatMessage, 0, len(history)+2)
	if system != "" {
		out = append(out, chatMessage{Role: "system", Content: system})
	}
	out = append(out, history...)
	out = append(out, chatMessage{Role: "user", Content: userPrompt})
	return out
}

// runREPL reads stdin line-by-line and chats with the endpoint until EOF.
// Conversation history is preserved across turns.
func runREPL(ctx context.Context, r runOverrides) error {
	fmt.Fprintln(os.Stderr, "mdemg model run — interactive REPL (Ctrl-D to exit)")
	scanner := bufio.NewScanner(os.Stdin)
	// Bump buffer for multi-line pasted prompts.
	scanner.Buffer(make([]byte, 4*1024), 1024*1024)

	var history []chatMessage
	for {
		fmt.Fprint(os.Stderr, "> ")
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return fmt.Errorf("read stdin: %w", err)
			}
			fmt.Fprintln(os.Stderr) // newline after EOF
			return nil
		}
		userPrompt := strings.TrimSpace(scanner.Text())
		if userPrompt == "" {
			continue
		}
		msgs := buildMessages(r.system, history, userPrompt)
		resp, err := callChat(ctx, r.endpoint, chatRequest{
			Model:       r.model,
			Messages:    msgs,
			Temperature: r.temperature,
			MaxTokens:   r.maxTokens,
		}, r.timeout)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			continue
		}
		answer := resp.Choices[0].Message.Content
		fmt.Println(answer)
		// Append both sides to history so context carries forward.
		history = append(history,
			chatMessage{Role: "user", Content: userPrompt},
			chatMessage{Role: "assistant", Content: answer},
		)
	}
}
