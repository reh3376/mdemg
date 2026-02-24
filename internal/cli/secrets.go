package cli

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/term"
	"mdemg/internal/secrets"
)

func newConfigSetSecretCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set-secret <key> [value]",
		Short: "Store a secret in the system keychain",
		Long: `Store a secret in the system keychain (macOS Keychain, Linux secret-tool, Windows Credential Manager).

Known secret keys (auto-mapped to environment variables):
  neo4j-password   -> NEO4J_PASS
  openai-api-key   -> OPENAI_API_KEY
  jwt-secret       -> AUTH_JWT_SECRET
  linear-webhook   -> LINEAR_WEBHOOK_SECRET

Arbitrary keys are also accepted but will not be auto-resolved.

If value is omitted, you will be prompted for hidden input.

Examples:
  mdemg config set-secret neo4j-password
  mdemg config set-secret openai-api-key sk-abc123`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			var value string

			if len(args) == 2 {
				value = args[1]
			} else {
				// Prompt for hidden input
				fmt.Fprintf(os.Stderr, "Enter value for '%s': ", key)
				raw, err := term.ReadPassword(int(syscall.Stdin))
				fmt.Fprintln(os.Stderr) // newline after hidden input
				if err != nil {
					return fmt.Errorf("read password: %w", err)
				}
				value = strings.TrimSpace(string(raw))
			}

			if value == "" {
				return fmt.Errorf("value cannot be empty")
			}

			if !secrets.IsKnown(key) {
				fmt.Fprintf(os.Stderr, "Warning: '%s' is not a known secret key (will not be auto-resolved)\n", key)
			}

			if err := secrets.Set(key, value); err != nil {
				return fmt.Errorf("store secret: %w", err)
			}

			fmt.Printf("Secret '%s' stored in system keychain\n", key)
			return nil
		},
	}

	return cmd
}

func newConfigGetSecretCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get-secret <key>",
		Short: "Retrieve a secret from the system keychain",
		Long: `Retrieve a secret from the system keychain and print it to stdout.

Exits with code 1 if the secret is not found (useful for scripting).

Examples:
  mdemg config get-secret neo4j-password
  export NEO4J_PASS=$(mdemg config get-secret neo4j-password)`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]

			val, err := secrets.Get(key)
			if err != nil {
				if err == secrets.ErrNotFound {
					fmt.Fprintf(os.Stderr, "Secret '%s' not found in keychain\n", key)
					os.Exit(1)
				}
				return fmt.Errorf("get secret: %w", err)
			}

			fmt.Print(val)
			return nil
		},
	}

	return cmd
}

func newConfigListSecretsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list-secrets",
		Short: "List known secrets and their keychain status",
		Long: `List all known MDEMG secret keys and whether they are stored in the system keychain.

Values are never printed.

Examples:
  mdemg config list-secrets`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Sort keys for deterministic output
			keys := make([]string, 0, len(secrets.KnownSecrets))
			for k := range secrets.KnownSecrets {
				keys = append(keys, k)
			}
			sort.Strings(keys)

			fmt.Printf("%-20s %-25s %s\n", "KEY", "ENV VAR", "STATUS")
			fmt.Printf("%-20s %-25s %s\n", "---", "-------", "------")

			for _, key := range keys {
				envVar := secrets.KnownSecrets[key]
				status := "not set"

				_, err := secrets.Get(key)
				if err == nil {
					status = "stored"
				}

				fmt.Printf("%-20s %-25s %s\n", key, envVar, status)
			}

			return nil
		},
	}

	return cmd
}
