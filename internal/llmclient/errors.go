package llmclient

import "errors"

// ErrMLXDown is returned by doWithRetry when the watchdog (mlxprobe) reports
// the configured LLM endpoint is in StateDown AND fast-fail is enabled. It
// short-circuits the 6-attempt × ~30 s retry loop that would otherwise
// fan-out across 16 LLM call sites and produce the retry-storm pattern
// observed during Phase 12 (1642% CPU, load avg 30+).
//
// Callers can disambiguate "exhausted retries" from "fast-failed because
// down" via errors.Is(err, ErrMLXDown).
var ErrMLXDown = errors.New("llmclient: mlx endpoint is down (watchdog fast-fail)")
