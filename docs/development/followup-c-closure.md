# Follow-up C — Closure (NON-ISSUE, no fix required)

**Date:** 2026-06-08
**Origin:** RRF-SCALE-001 live-smoke triage, Follow-up C — *"`/v1/jiminy/latest` JSON control-char escaping breaks the `jq` in `prompt-context.sh`."*
**Outcome:** **Investigated and closed as a non-issue.** No code change. Shipping a "fix" would invent a problem that does not exist.

## Why it looked like a bug

During JIMINY-OUTCOME-001 verification, ad-hoc shell commands parsing `/v1/jiminy/latest` failed with:
- `jq: Invalid string: control characters from U+0000 through U+001F must be escaped`
- `python json: Invalid control character at: line 1 column 2683`

These co-occurred with the session's recurring shell errors (`failed to change group ID: operation not permitted`) and ad-hoc `LATEST=$(curl …)` variable capture / direct-`python -c` piping — i.e. **client-side handling artifacts**, not the server's bytes.

## Why it is NOT a server bug (evidence)

1. **The server emits valid JSON by construction.** `handleJiminyLatest` → `writeJSON` → `json.NewEncoder(w).Encode(v)` (standard `encoding/json`). `encoding/json` **always** escapes control characters (U+0000–U+001F → `\uXXXX` / `\n` / `\t` / `\r`). There is no `w.Write([]byte(json))` / `fmt.Fprint` bypass in `handlers_jiminy.go`. No custom `MarshalJSON` on the guidance/response types. ⇒ the response cannot contain raw unescaped control chars.
2. **The synthesized narrative is double-sanitized.** `synthesizer.go:127` returns `sanitize.StripControlChars(narrative)`, and `service.go:1116` wraps it again in `StripControlChars` before it becomes `prompt_augmentation`. `StripControlChars` drops every `r < 0x20` except `\t`/`\n`/`\r`.
3. **The hook already strips control chars defensively, before `jq`.** `prompt-context.sh`:
   ```bash
   curl -sf ".../v1/jiminy/latest" | perl -pe 's/[\x00-\x08\x0b\x0c\x0e-\x1f]//g' > "$GUIDANCE_TMP"
   ...
   GUIDANCE_ID=$(jq -r '.data.guidance_id // empty' "$GUIDANCE_TMP" 2>/dev/null)
   ```
   It writes to a temp file, strips control chars via `perl`, parses with `jq` (with `2>/dev/null` + `// empty` fallbacks). Even a hypothetical malformed response could not break the `guidance_id` capture.

## Live verification

- The hook's **exact** `guidance_id` extraction against the live endpoint returns the ID: `jq -r '.data.guidance_id // empty'` → `ct1vegyeevxcmtwhozw6vfis`.
- 5 rapid `/v1/jiminy/latest` fetches → all parse as **valid JSON** under strict `python json.load`.
- 0 raw control chars (excluding tab/newline/CR) in the response bytes.

## Conclusion

The guidance feedback loop's `guidance_id` capture is **not** at risk from control-char escaping. The server produces valid JSON, the narrative is sanitized, and the hook strips control chars regardless. Follow-up C is closed with no code change.

**This closes the entire RRF-SCALE-001 live-smoke triage:** Follow-up A (Neo4j `GUIDANCE_OUTCOME` sink → JIMINY-OUTCOME-001) ✅, Follow-up B (synthesis timeout → GUIDANCE-SYNTH-001) ✅, Follow-up C (this, non-issue) ✅. The guidance→feedback→outcome loop is fully functional end-to-end: surfacing (RRF-SCALE-001), constraint-code attachment + both outcome sinks (JIMINY-OUTCOME-001), and synthesis (GUIDANCE-SYNTH-001).

## Documents Accessed
- `internal/api/server.go` — `writeJSON` (`json.NewEncoder().Encode`)
- `internal/api/handlers_jiminy.go` — `handleJiminyLatest` (uses `writeJSON`; no raw-write bypass)
- `internal/jiminy/synthesizer.go` (127) + `internal/jiminy/service.go` (1116) — double `StripControlChars`
- `internal/sanitize/*.go` — `StripControlChars` (drops `r < 0x20` except `\t\n\r`)
- `.claude/hooks/prompt-context.sh` — the `perl`-strip + `jq` `guidance_id` capture
- Live: hook `jq` returns `guidance_id`; 5× strict-JSON parse all valid
