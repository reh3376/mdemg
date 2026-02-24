// Package secrets provides a thin wrapper around the system keychain for
// storing and retrieving MDEMG secrets (API keys, passwords, etc.).
// Uses go-keyring for cross-platform support: macOS Keychain, Linux secret-tool,
// Windows Credential Manager.
package secrets

import (
	"errors"
	"os"

	"github.com/zalando/go-keyring"
)

const serviceName = "mdemg"

// ErrNotFound is returned when a secret does not exist in the keychain.
var ErrNotFound = errors.New("secret not found")

// KnownSecrets maps a human-friendly key name to the environment variable it should populate.
var KnownSecrets = map[string]string{ //nolint:gosec // Not credentials — this is a key-to-envvar mapping
	"neo4j-password":  "NEO4J_PASS",
	"openai-api-key":  "OPENAI_API_KEY",
	"jwt-secret":      "AUTH_JWT_SECRET",
	"linear-webhook":  "LINEAR_WEBHOOK_SECRET",
}

// Set stores a secret in the system keychain under the mdemg service.
func Set(key, value string) error {
	return keyring.Set(serviceName, key, value)
}

// Get retrieves a secret from the system keychain. Returns ErrNotFound if the
// key does not exist.
func Get(key string) (string, error) {
	val, err := keyring.Get(serviceName, key)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", ErrNotFound
		}
		return "", err
	}
	return val, nil
}

// Delete removes a secret from the system keychain. Returns ErrNotFound if the
// key does not exist.
func Delete(key string) error {
	err := keyring.Delete(serviceName, key)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

// IsKnown returns true if the key is a recognized secret name.
func IsKnown(key string) bool {
	_, ok := KnownSecrets[key]
	return ok
}

// ResolveSecrets iterates all KnownSecrets, looks up each in the keychain,
// and sets the corresponding env var only if it is not already set.
// Errors are silently ignored — keychain is opportunistic.
func ResolveSecrets() {
	for key, envVar := range KnownSecrets {
		// Skip if env var is already set (env > keychain priority)
		if os.Getenv(envVar) != "" {
			continue
		}
		val, err := Get(key)
		if err != nil || val == "" {
			continue
		}
		_ = os.Setenv(envVar, val)
	}
}
