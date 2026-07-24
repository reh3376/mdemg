// FT-RECURSIVE-003 E7: the class-5 escape hatch — "the system knows
// something is wrong it cannot fix."
//
// Spec §3a: repeated class-4 failures on the same fingerprint escalate to
// (1) a CapabilityGap in the existing internal/gaps store — ONE escalation
// path for self-known deficiencies — and (2) a GitHub issue via `gh`,
// fingerprint-idempotent: an existing open issue with the fingerprint label
// gets a comment, never a duplicate. Filing itself reports through
// jobhealth (`ft-issue-filer`) so a broken filer is never silent.
package ftloop

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"mdemg/internal/gaps"
)

// IssueFilerConfig controls the class-5 escalator.
type IssueFilerConfig struct {
	Enabled         bool   // FT_LOOP_ISSUE_FILER_ENABLED
	RepeatThreshold int    // FT_LOOP_ISSUE_REPEAT_THRESHOLD — distinct failed cycles sharing a fingerprint before filing (default 2)
	LookbackDays    int    // FT_LOOP_ISSUE_LOOKBACK_DAYS (default 30)
	Repo            string // FT_LOOP_ISSUE_REPO (default reh3376/mdemg)
	TokenPath       string // FT_LOOP_ISSUE_TOKEN_PATH — optional fine-grained PAT file; empty = ambient gh auth
	SweepMinutes    int    // FT_LOOP_ISSUE_SWEEP_MIN — min interval between sweeps (default 10)
}

// GapSink is the narrow gaps-store surface the filer needs.
type GapSink interface {
	SaveGap(ctx context.Context, gap gaps.CapabilityGap) error
}

// JobReporter mirrors the jobhealth call the controller already uses.
type JobReporter func(ctx context.Context, jobName string, success bool, latencyMs int64, errMsg string)

// IssueFiler sweeps the cycle ledger for repeated failure fingerprints.
type IssueFiler struct {
	pool      *pgxpool.Pool
	cfg       IssueFilerConfig
	gapSink   GapSink     // nil-safe
	report    JobReporter // nil-safe
	lastSweep time.Time
	acted     map[string]time.Time // fingerprint → last action time (per-process dedupe)
	now       func() time.Time
	runGh     func(ctx context.Context, args ...string) (string, error) // test seam
}

// NewIssueFiler wires the escalator.
func NewIssueFiler(pool *pgxpool.Pool, cfg IssueFilerConfig, gapSink GapSink, report JobReporter) *IssueFiler {
	f := &IssueFiler{pool: pool, cfg: cfg, gapSink: gapSink, report: report,
		acted: map[string]time.Time{}, now: time.Now}
	f.runGh = f.execGh
	return f
}

var (
	hexRunRe   = regexp.MustCompile(`[0-9a-f]{8,}`)
	digitRunRe = regexp.MustCompile(`\d{3,}`)
	pathRe     = regexp.MustCompile(`/[^\s"']+`)
)

// normalizeSignature collapses volatile tokens (cycle ids, paths, big
// numbers) so recurrences of the same defect fingerprint identically.
func normalizeSignature(stage, errText string) string {
	sig := errText
	sig = pathRe.ReplaceAllString(sig, "<path>")
	sig = hexRunRe.ReplaceAllString(sig, "<hex>")
	sig = digitRunRe.ReplaceAllString(sig, "<n>")
	if len(sig) > 160 {
		sig = sig[:160]
	}
	return stage + "|" + strings.TrimSpace(sig)
}

// fingerprint is the stable short id used as the GitHub label.
func fingerprint(normalized string) string {
	sum := sha256.Sum256([]byte(normalized))
	return fmt.Sprintf("%x", sum[:4])
}

// sweepQuery clusters failure fingerprints from BOTH terminal failure
// shapes — status='failed' AND rolled_back events whose stage carries a
// failure class (FT-RECURSIVE-004 E3) — evaluated on each cycle's LATEST
// event only (DISTINCT ON: the ledger is event-sourced, and a superseding
// event ends a cycle's story; reading raw event rows would resurrect
// neutralized/resolved cycles forever — live-caught during this epic).
const sweepQuery = `
		WITH latest AS (
			SELECT DISTINCT ON (cycle_id)
			       cycle_id, status, COALESCE(stage,'') AS stage,
			       COALESCE(error,'') AS error, time
			FROM ft_training_cycles
			WHERE time > now() - ($1 || ' days')::interval
			ORDER BY cycle_id, time DESC
		)
		SELECT cycle_id, stage, error, time FROM latest
		WHERE (status = 'failed'
		       OR (status = 'rolled_back' AND stage LIKE '%\_failed'))
		  AND error <> ''
		ORDER BY time`

// sweepQueryForTest exposes the sweep SQL for shape pins.
func sweepQueryForTest() string { return sweepQuery }

// failureGroup is one repeated-failure cluster.
type failureGroup struct {
	Stage       string
	Signature   string
	Fingerprint string
	Count       int
	CycleIDs    []string
	LastError   string
	LastAt      time.Time
}

// Sweep evaluates the ledger and files/comments as needed. Rate-limited by
// SweepMinutes; safe to call every controller tick.
func (f *IssueFiler) Sweep(ctx context.Context) {
	if f == nil || !f.cfg.Enabled || f.pool == nil {
		return
	}
	minGap := time.Duration(f.cfg.SweepMinutes) * time.Minute
	if minGap <= 0 {
		minGap = 10 * time.Minute
	}
	if f.now().Sub(f.lastSweep) < minGap {
		return
	}
	f.lastSweep = f.now()

	groups, err := f.collectGroups(ctx)
	if err != nil {
		slog.Warn("ft-issue-filer: ledger sweep failed", "error", err)
		return
	}
	for _, g := range groups {
		if last, ok := f.acted[g.Fingerprint]; ok && !g.LastAt.After(last) {
			continue // nothing new since we last acted on this fingerprint
		}
		start := f.now()
		err := f.escalate(ctx, g)
		if f.report != nil {
			msg := ""
			if err != nil {
				msg = err.Error()
			}
			f.report(ctx, "ft-issue-filer", err == nil, f.now().Sub(start).Milliseconds(), msg)
		}
		if err != nil {
			slog.Error("ft-issue-filer: escalation failed", "fingerprint", g.Fingerprint, "error", err)
			continue
		}
		f.acted[g.Fingerprint] = g.LastAt
	}
}

// collectGroups fetches failed cycles in the lookback and clusters them by
// normalized signature, keeping clusters at/over the repeat threshold.
func (f *IssueFiler) collectGroups(ctx context.Context) ([]failureGroup, error) {
	lookback := f.cfg.LookbackDays
	if lookback <= 0 {
		lookback = 30
	}
	threshold := f.cfg.RepeatThreshold
	if threshold <= 0 {
		threshold = 2
	}
	// FT-RECURSIVE-004 E3 (003's disclosed gap): failure fingerprints also
	// live in rolled_back-terminal events whose stage carries a failure class
	// (canary_failed / promote_failed / *_failed) — cluster those too, not
	// just status='failed'.
	rows, err := f.pool.Query(ctx, sweepQuery, fmt.Sprintf("%d", lookback))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byFp := map[string]*failureGroup{}
	for rows.Next() {
		var cycleID, stage, errText string
		var at time.Time
		if err := rows.Scan(&cycleID, &stage, &errText, &at); err != nil {
			return nil, err
		}
		sig := normalizeSignature(stage, errText)
		fp := fingerprint(sig)
		g, ok := byFp[fp]
		if !ok {
			g = &failureGroup{Stage: stage, Signature: sig, Fingerprint: fp}
			byFp[fp] = g
		}
		g.Count++
		g.CycleIDs = append(g.CycleIDs, cycleID)
		g.LastError = errText
		if at.After(g.LastAt) {
			g.LastAt = at
		}
	}
	var out []failureGroup
	for _, g := range byFp {
		if g.Count >= threshold {
			out = append(out, *g)
		}
	}
	return out, rows.Err()
}

// escalate records the CapabilityGap and files/comments the GitHub issue.
func (f *IssueFiler) escalate(ctx context.Context, g failureGroup) error {
	label := "ftloop-fp-" + g.Fingerprint
	title := fmt.Sprintf("[ft-loop] repeated %s failure (%d cycles): %s",
		g.Stage, g.Count, truncateForTitle(g.LastError))
	body := fmt.Sprintf(
		"Automated class-5 escalation (FT-RECURSIVE-003 E7).\n\n"+
			"- **Stage:** %s\n- **Taxonomy class:** 5 (repeated class-4 on one fingerprint)\n"+
			"- **Fingerprint:** `%s`\n- **Occurrences (lookback):** %d\n"+
			"- **Cycles:** `%s`\n- **Latest error:**\n\n```\n%s\n```\n\n"+
			"Repro: inspect `ft_training_cycles` for the cycle ids above; "+
			"re-drive via `mdemg ft-loop` after fixing the underlying cause.\n",
		g.Stage, g.Fingerprint, g.Count, strings.Join(g.CycleIDs, "`, `"), g.LastError)

	// Gap record first — the internal escalation path is primary.
	if f.gapSink != nil {
		if err := f.gapSink.SaveGap(ctx, gaps.CapabilityGap{
			ID:              "ftloop-" + g.Fingerprint,
			Type:            gaps.GapTypeReasoning,
			Description:     title,
			Evidence:        []string{g.Signature, "cycles: " + strings.Join(g.CycleIDs, ",")},
			Priority:        0.8,
			DetectedAt:      f.now(),
			UpdatedAt:       f.now(),
			Status:          gaps.GapStatusOpen,
			OccurrenceCount: g.Count,
		}); err != nil {
			slog.Warn("ft-issue-filer: gap record failed (continuing to issue)", "error", err)
		}
	}

	// Idempotency: open issue with the fingerprint label → comment.
	existing, err := f.runGh(ctx, "issue", "list", "--repo", f.cfg.Repo,
		"--label", label, "--state", "open", "--json", "number", "--jq", ".[0].number")
	if err != nil {
		return fmt.Errorf("gh issue list: %w", err)
	}
	existing = strings.TrimSpace(existing)
	if existing != "" && existing != "null" {
		_, err := f.runGh(ctx, "issue", "comment", existing, "--repo", f.cfg.Repo,
			"--body", fmt.Sprintf("Recurred: now %d cycles (latest `%s` at %s).",
				g.Count, g.CycleIDs[len(g.CycleIDs)-1], g.LastAt.UTC().Format(time.RFC3339)))
		if err != nil {
			return fmt.Errorf("gh issue comment: %w", err)
		}
		slog.Info("ft-issue-filer: commented on existing issue", "issue", existing, "fingerprint", g.Fingerprint)
		return nil
	}

	// Ensure the labels exist (idempotent; ignore errors on pre-existing).
	_, _ = f.runGh(ctx, "label", "create", label, "--repo", f.cfg.Repo,
		"--description", "ft-loop failure fingerprint", "--color", "B60205")
	_, _ = f.runGh(ctx, "label", "create", "ftloop", "--repo", f.cfg.Repo,
		"--description", "recursive-retrain loop", "--color", "1D76DB")

	out, err := f.runGh(ctx, "issue", "create", "--repo", f.cfg.Repo,
		"--title", title, "--body", body, "--label", label+",ftloop")
	if err != nil {
		return fmt.Errorf("gh issue create: %w", err)
	}
	slog.Info("ft-issue-filer: issue filed", "fingerprint", g.Fingerprint, "url", strings.TrimSpace(out))
	return nil
}

// execGh runs the gh CLI (resolveTool for launchd PATH), honoring TokenPath.
func (f *IssueFiler) execGh(ctx context.Context, args ...string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, resolveTool("gh", ""), args...) //nolint:gosec // G204: config/loop-constructed
	if f.cfg.TokenPath != "" {
		if tok, err := os.ReadFile(f.cfg.TokenPath); err == nil {
			cmd.Env = append(os.Environ(), "GH_TOKEN="+strings.TrimSpace(string(tok)))
		} else {
			slog.Warn("ft-issue-filer: token file unreadable, using ambient auth", "path", f.cfg.TokenPath, "error", err)
		}
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%w (%s)", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func truncateForTitle(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 80 {
		cut := 80
		for cut > 0 && s[cut]&0xC0 == 0x80 { // rune-safe (TSDB-WRITER-UTF8-001 class)
			cut--
		}
		return s[:cut] + "…"
	}
	return s
}
