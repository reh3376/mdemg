// Sprint JIMINY-METRIC-PARTITION-001 (task #158, 2026-09-10) —
// `mdemg jiminy constraint {set-class,list-classes,seed-classes}` CLI for
// managing the verifiability_class Neo4j property on constraint/correction
// nodes.
//
// The property is checked at RecordOutcome time (service.go): outcomes on
// rules classed as `process` or `human` are NOT recorded to constraint_outcomes
// (they route to their own grader pipelines shipped by JIMINY-PROCESS-
// OBSERVER-{01..06} and JIMINY-HITL-HUMAN-CLASS-INTEGRATION-001 respectively).
// Outcomes on `classifier` (default) or `hybrid` are recorded as today, but
// with a class-tagged column so per-class metric gauges can partition them.
//
// Talks to Neo4j directly (mirrors the `mark` CLI); the operation is a local
// operator-authorized property flip.

package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/spf13/cobra"

	"mdemg/internal/jiminy"
)

// jiminySeedClassMap is the static seed produced by JIMINY-METRIC-DENOMINATOR-
// DESIGN-001 §Phase A (the 33-rule taxonomy audit). Applied by `seed-classes`
// against the operator-supplied space. Static const map so ANY change to the
// classification is a code diff (reviewable) not a silent live mutation.
//
// Add new rules here as they're introduced to `mdemg-dev`. Missing codes on
// live nodes will be reported by seed-classes as "unknown code — not seeded";
// missing rules in the map will not fail the seed run.
var jiminySeedClassMap = map[string]jiminy.VerifiabilityClass{
	// Classifier-verifiable (9) — evidence in action-text
	"auto-29156377a1de":                    jiminy.VerifiabilityClassifier, // NEVER use mdemg db start
	"auto-a4a36173bff8":                    jiminy.VerifiabilityClassifier, // MDEMG codebase ingest with space_id=mdemg-dev
	"must-use-cuid2":                       jiminy.VerifiabilityClassifier,
	"never-hardcode-config":                jiminy.VerifiabilityClassifier,
	"never-direct-alter-schema":            jiminy.VerifiabilityClassifier,
	"no-stash-for-release":                 jiminy.VerifiabilityClassifier,
	"no-direct-main-commits":               jiminy.VerifiabilityClassifier,
	"openai-max-completion-tokens":         jiminy.VerifiabilityClassifier,
	"project-planning-docs-in-repo-only":   jiminy.VerifiabilityClassifier,

	// Process-verifiable (4) — evidence in process-observation events
	"lint-before-commit":         jiminy.VerifiabilityProcess,
	"never-haiku-for-planning":   jiminy.VerifiabilityProcess,
	"sequential-epics":           jiminy.VerifiabilityProcess,
	"query-mdemg-cms-file-paths": jiminy.VerifiabilityProcess,

	// Hybrid (2) — classifier grades shape / process grades act
	"must-use-uxts-frameworks-consistently": jiminy.VerifiabilityHybrid,
	"unit-integration-e2e-docs":             jiminy.VerifiabilityHybrid,

	// Human-verifiable (18) — no automated grade possible; route to HITL
	// (these are also the Path 1 informational set; seeding class=human here
	// documents WHY they're informational + prepares HITL routing)
	"agent-handoff-requirement-guardrail":                jiminy.VerifiabilityHuman,
	"auto-build-restart-after-feature":                   jiminy.VerifiabilityHuman,
	"end-with-docs-accessed":                             jiminy.VerifiabilityHuman,
	"iterate-break-fix-verify":                           jiminy.VerifiabilityHuman,
	"live-testing-tier-required":                         jiminy.VerifiabilityHuman,
	"mandatory-feature-docs":                             jiminy.VerifiabilityHuman,
	"markdown-mermaid-tables-and-charts":                 jiminy.VerifiabilityHuman,
	"mdemg-cms-memory-only":                              jiminy.VerifiabilityHuman,
	"memory-preservation-backup-integrity":               jiminy.VerifiabilityHuman,
	"must-comment-sprint-summary-on-pr":                  jiminy.VerifiabilityHuman,
	"must-enforce-jiminy-constraints":                    jiminy.VerifiabilityHuman,
	"must-follow-12-section-format":                      jiminy.VerifiabilityHuman,
	"must-master-data-pipelines":                         jiminy.VerifiabilityHuman,
	"must-validate-all-claims-before-commit":             jiminy.VerifiabilityHuman,
	"never-classify-policy-docs-as-constraint":           jiminy.VerifiabilityHuman,
	"never-skip-discovered-issues":                       jiminy.VerifiabilityHuman,
	"plan-mode-before-change":                            jiminy.VerifiabilityHuman,
	"trust-signal-must-be-persisted-never-ignore-honest": jiminy.VerifiabilityHuman,
}

func newJiminyConstraintSetClassCmd() *cobra.Command {
	var (
		spaceID string
		code    string
		class   string
		dryRun  bool
	)
	cmd := &cobra.Command{
		Use:   "set-class",
		Short: "Set verifiability_class on a constraint by its constraint_code",
		Long: `Set the verifiability_class property on the constraint/correction node
with the given constraint_code. Class must be one of:
  classifier — evidence in action-text (LLM classifier grades)
  process    — evidence in process-observation events (Path 2 grader)
  hybrid     — classifier grades shape + process grades act
  human      — no automated grade possible (HITL grader)

Reversible via --class classifier (the default class).

Example:
  mdemg jiminy constraint set-class --code lint-before-commit --space-id mdemg-dev --class process
  mdemg jiminy constraint set-class --code lint-before-commit --space-id mdemg-dev --class classifier`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if code == "" {
				return fmt.Errorf("--code is required")
			}
			if spaceID == "" {
				return fmt.Errorf("--space-id is required (never mutate on an unbounded space)")
			}
			if !jiminy.IsValidVerifiabilityClass(class) {
				return fmt.Errorf("--class must be one of: classifier, process, hybrid, human (got %q)", class)
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()
			return runJiminyConstraintSetClass(ctx, cmd, spaceID, code, class, dryRun)
		},
	}
	cmd.Flags().StringVar(&code, "code", "", "constraint_code to set class on (required)")
	cmd.Flags().StringVar(&spaceID, "space-id", "", "space_id (required)")
	cmd.Flags().StringVar(&class, "class", "", "verifiability class: classifier|process|hybrid|human (required)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would change without writing")
	return cmd
}

func newJiminyConstraintListClassesCmd() *cobra.Command {
	var (
		spaceID   string
		classOnly string
	)
	cmd := &cobra.Command{
		Use:   "list-classes",
		Short: "List constraint/correction nodes with their verifiability_class",
		RunE: func(cmd *cobra.Command, args []string) error {
			if spaceID == "" {
				return fmt.Errorf("--space-id is required")
			}
			if classOnly != "" && !jiminy.IsValidVerifiabilityClass(classOnly) {
				return fmt.Errorf("--class filter must be empty or one of: classifier, process, hybrid, human (got %q)", classOnly)
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
			defer cancel()
			return runJiminyConstraintListClasses(ctx, cmd, spaceID, classOnly)
		},
	}
	cmd.Flags().StringVar(&spaceID, "space-id", "", "space_id (required)")
	cmd.Flags().StringVar(&classOnly, "class", "", "filter to a single class (optional)")
	return cmd
}

func newJiminyConstraintSeedClassesCmd() *cobra.Command {
	var (
		spaceID string
		dryRun  bool
		force   bool
	)
	cmd := &cobra.Command{
		Use:   "seed-classes",
		Short: "Apply the static per-code verifiability_class seed (JIMINY-METRIC-DENOMINATOR-DESIGN-001)",
		Long: `Apply the static per-code verifiability_class seed produced by
JIMINY-METRIC-DENOMINATOR-DESIGN-001 §Phase A. Idempotent: only writes to
nodes whose current class doesn't match the seed value (unless --force).

The seed covers all 33 rules known to the design sprint at ship time (2026-09-07):
  - 9 classifier-verifiable
  - 4 process-verifiable
  - 2 hybrid
  - 18 human-verifiable

New rules not in the seed default to classifier at RecordOutcome time (safe
default). To adjust: use ` + "`mdemg jiminy constraint set-class`" + ` for
individual codes, or edit jiminySeedClassMap in a code review.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if spaceID == "" {
				return fmt.Errorf("--space-id is required")
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 60*time.Second)
			defer cancel()
			return runJiminyConstraintSeedClasses(ctx, cmd, spaceID, dryRun, force)
		},
	}
	cmd.Flags().StringVar(&spaceID, "space-id", "", "space_id (required)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would change without writing")
	cmd.Flags().BoolVar(&force, "force", false, "re-apply seed even when node's current class matches")
	return cmd
}

func runJiminyConstraintSetClass(ctx context.Context, cmd *cobra.Command, spaceID, code, class string, dryRun bool) error {
	drv, err := neo4jDriverFromEnv(ctx)
	if err != nil {
		return err
	}
	defer drv.Close(ctx) //nolint:errcheck

	fmt.Fprintf(cmd.OutOrStdout(), "MDEMG Jiminy Constraint Set-Class\n")
	fmt.Fprintf(cmd.OutOrStdout(), "Space:            %s\n", spaceID)
	fmt.Fprintf(cmd.OutOrStdout(), "Constraint code:  %s\n", code)
	fmt.Fprintf(cmd.OutOrStdout(), "New value:        verifiability_class = %s\n", class)
	if dryRun {
		fmt.Fprintf(cmd.OutOrStdout(), "Mode:             DRY RUN (no writes)\n")
	}

	sess := drv.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	res, err := sess.Run(ctx, `
		MATCH (c:MemoryNode)
		WHERE c.space_id = $space AND c.constraint_code = $code
		  AND c.role_type IN ['constraint','correction']
		  AND NOT coalesce(c.is_archived, false)
		RETURN c.node_id AS nid, c.name AS name,
		       coalesce(c.verifiability_class, 'classifier') AS cur
	`, map[string]any{"space": spaceID, "code": code})
	if err != nil {
		_ = sess.Close(ctx)
		return fmt.Errorf("scan matching nodes: %w", err)
	}
	type match struct {
		nid, name, cur string
	}
	var matches []match
	for res.Next(ctx) {
		rec := res.Record()
		nid, _ := rec.Get("nid")
		name, _ := rec.Get("name")
		cur, _ := rec.Get("cur")
		matches = append(matches, match{
			nid:  asStr(nid),
			name: asStr(name),
			cur:  asStr(cur),
		})
	}
	_ = sess.Close(ctx)
	if err := res.Err(); err != nil {
		return fmt.Errorf("scan matching nodes: %w", err)
	}
	if len(matches) == 0 {
		return fmt.Errorf("no live constraint/correction node found with constraint_code=%q in space=%q", code, spaceID)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\nMatched %d node(s):\n", len(matches))
	for _, m := range matches {
		fmt.Fprintf(cmd.OutOrStdout(), "  node_id=%s  name=%q  current=%s\n", m.nid, truncateForCLI(m.name, 70), m.cur)
	}

	if dryRun {
		fmt.Fprintln(cmd.OutOrStdout(), "\n(dry-run — no writes; re-run without --dry-run to apply)")
		return nil
	}

	writeSess := drv.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer writeSess.Close(ctx) //nolint:errcheck
	_, err = writeSess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		return tx.Run(ctx, `
			MATCH (c:MemoryNode)
			WHERE c.space_id = $space AND c.constraint_code = $code
			  AND c.role_type IN ['constraint','correction']
			  AND NOT coalesce(c.is_archived, false)
			SET c.verifiability_class = $class,
			    c.verifiability_class_marked_at = datetime()
			RETURN count(c) AS n
		`, map[string]any{"space": spaceID, "code": code, "class": class})
	})
	if err != nil {
		return fmt.Errorf("apply verifiability_class=%s: %w", class, err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "\n✓ Set verifiability_class=%s on %d node(s) (code=%q, space=%q)\n", class, len(matches), code, spaceID)
	return nil
}

func runJiminyConstraintListClasses(ctx context.Context, cmd *cobra.Command, spaceID, classOnly string) error {
	drv, err := neo4jDriverFromEnv(ctx)
	if err != nil {
		return err
	}
	defer drv.Close(ctx) //nolint:errcheck

	sess := drv.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx) //nolint:errcheck

	query := `
		MATCH (c:MemoryNode)
		WHERE c.space_id = $space
		  AND c.role_type IN ['constraint','correction']
		  AND NOT coalesce(c.is_archived, false)
	`
	if classOnly != "" {
		query += ` AND coalesce(c.verifiability_class, 'classifier') = $class`
	}
	query += `
		RETURN coalesce(c.verifiability_class, 'classifier') AS class,
		       c.role_type AS role,
		       c.constraint_code AS code,
		       c.node_id AS nid,
		       c.name AS name
		ORDER BY class, code
	`
	params := map[string]any{"space": spaceID}
	if classOnly != "" {
		params["class"] = classOnly
	}
	res, err := sess.Run(ctx, query, params)
	if err != nil {
		return fmt.Errorf("query classes: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Verifiability class assignments in space=%q:\n\n", spaceID)
	byClass := map[string]int{}
	for res.Next(ctx) {
		rec := res.Record()
		class, _ := rec.Get("class")
		role, _ := rec.Get("role")
		code, _ := rec.Get("code")
		nid, _ := rec.Get("nid")
		name, _ := rec.Get("name")
		classStr := asStr(class)
		byClass[classStr]++
		fmt.Fprintf(cmd.OutOrStdout(), "  class=%-11s role=%-11s code=%-45s\n",
			classStr, asStr(role), truncateForCLI(asStr(code), 45))
		fmt.Fprintf(cmd.OutOrStdout(), "    node_id=%s  name=%q\n", asStr(nid), truncateForCLI(asStr(name), 60))
	}
	if err := res.Err(); err != nil {
		return fmt.Errorf("iter: %w", err)
	}
	fmt.Fprintln(cmd.OutOrStdout(), "\nSummary by class:")
	total := 0
	for _, c := range jiminy.AllVerifiabilityClasses() {
		n := byClass[string(c)]
		fmt.Fprintf(cmd.OutOrStdout(), "  %-11s %d\n", c, n)
		total += n
	}
	fmt.Fprintf(cmd.OutOrStdout(), "  %-11s %d\n", "TOTAL", total)
	return nil
}

func runJiminyConstraintSeedClasses(ctx context.Context, cmd *cobra.Command, spaceID string, dryRun, force bool) error {
	drv, err := neo4jDriverFromEnv(ctx)
	if err != nil {
		return err
	}
	defer drv.Close(ctx) //nolint:errcheck

	fmt.Fprintf(cmd.OutOrStdout(), "MDEMG Jiminy Constraint Seed-Classes\n")
	fmt.Fprintf(cmd.OutOrStdout(), "Space:            %s\n", spaceID)
	fmt.Fprintf(cmd.OutOrStdout(), "Seed size:        %d codes\n", len(jiminySeedClassMap))
	if force {
		fmt.Fprintf(cmd.OutOrStdout(), "Force:            YES (re-apply even when current matches)\n")
	}
	if dryRun {
		fmt.Fprintf(cmd.OutOrStdout(), "Mode:             DRY RUN (no writes)\n")
	}

	// Snapshot every live rule's current class so we can diff against the seed.
	sess := drv.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	res, err := sess.Run(ctx, `
		MATCH (c:MemoryNode)
		WHERE c.space_id = $space
		  AND c.role_type IN ['constraint','correction']
		  AND NOT coalesce(c.is_archived, false)
		RETURN c.constraint_code AS code, c.node_id AS nid,
		       coalesce(c.verifiability_class, 'classifier') AS cur
	`, map[string]any{"space": spaceID})
	if err != nil {
		_ = sess.Close(ctx)
		return fmt.Errorf("snapshot current classes: %w", err)
	}
	type liveNode struct {
		nid, code, cur string
	}
	var live []liveNode
	for res.Next(ctx) {
		rec := res.Record()
		code, _ := rec.Get("code")
		nid, _ := rec.Get("nid")
		cur, _ := rec.Get("cur")
		if asStr(code) != "" {
			live = append(live, liveNode{
				nid:  asStr(nid),
				code: asStr(code),
				cur:  asStr(cur),
			})
		}
	}
	_ = sess.Close(ctx)
	if err := res.Err(); err != nil {
		return fmt.Errorf("iter live nodes: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Live actionable nodes with constraint_code: %d\n\n", len(live))

	// Categorize each live node relative to the seed map.
	type plan struct {
		nid, code, curClass, newClass string
		action                        string
	}
	var toApply, toSkip, unmapped []plan
	for _, n := range live {
		want, mapped := jiminySeedClassMap[n.code]
		p := plan{nid: n.nid, code: n.code, curClass: n.cur}
		if !mapped {
			p.action = "unmapped-in-seed (leave alone)"
			p.newClass = n.cur
			unmapped = append(unmapped, p)
			continue
		}
		p.newClass = string(want)
		if !force && n.cur == string(want) {
			p.action = "already-matches"
			toSkip = append(toSkip, p)
			continue
		}
		if force && n.cur == string(want) {
			p.action = "force-reapply"
		} else {
			p.action = "apply"
		}
		toApply = append(toApply, p)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Plan:\n")
	fmt.Fprintf(cmd.OutOrStdout(), "  to apply:              %d\n", len(toApply))
	fmt.Fprintf(cmd.OutOrStdout(), "  already-matches skip:  %d\n", len(toSkip))
	fmt.Fprintf(cmd.OutOrStdout(), "  unmapped-in-seed:      %d\n", len(unmapped))

	if len(toApply) > 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "\nTo apply:")
		for _, p := range toApply {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s: %s → %s (code=%s)\n", p.action, p.curClass, p.newClass, p.code)
		}
	}
	if len(unmapped) > 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "\nUnmapped (present on substrate but not in static seed — left at current class):")
		for _, p := range unmapped {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s (current=%s)\n", p.code, p.curClass)
		}
	}

	if dryRun {
		fmt.Fprintln(cmd.OutOrStdout(), "\n(dry-run — no writes; re-run without --dry-run to apply)")
		return nil
	}
	if len(toApply) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "\n(nothing to apply — seed is already in effect)")
		return nil
	}

	writeSess := drv.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer writeSess.Close(ctx) //nolint:errcheck
	applied := 0
	for _, p := range toApply {
		_, err := writeSess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			return tx.Run(ctx, `
				MATCH (c:MemoryNode {node_id: $nid})
				SET c.verifiability_class = $class,
				    c.verifiability_class_marked_at = datetime()
				RETURN c.node_id AS nid
			`, map[string]any{"nid": p.nid, "class": p.newClass})
		})
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "  ERROR applying to %s (%s): %v\n", p.code, p.nid, err)
			continue
		}
		applied++
	}
	fmt.Fprintf(cmd.OutOrStdout(), "\n✓ Applied %d/%d (space=%q)\n", applied, len(toApply), spaceID)
	return nil
}
