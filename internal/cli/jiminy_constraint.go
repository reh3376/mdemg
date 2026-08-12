// Sprint JIMINY-INFORMATIONAL-CATEGORY-001 (2026-08-12) —
// `mdemg jiminy constraint mark` CLI for flipping the is_informational
// property on constraint/correction nodes.
//
// The property is checked at RecordOutcome time (service.go): items whose
// source node is marked informational get their outcome overridden to
// OutcomeNotApplicable, which the shipped writer gates (service.go:1902,
// 1927, 1954, 1986, 2090) already exclude from actionable follow-rate
// accounting. Fully reversible via --informational=false.
//
// Talks to Neo4j directly (not through the HTTP API) — the operation is a
// local operator-authorized property flip; making it an API endpoint would
// require auth wiring that's not warranted for this scope.

package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/spf13/cobra"

	"mdemg/internal/config"
)

func newJiminyConstraintCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "constraint",
		Short: "Manage constraint/correction node properties (JIMINY-INFORMATIONAL-CATEGORY-001)",
		Long: `Operate on individual constraint / correction nodes in Neo4j.

Subcommands:
  mark    — flip the is_informational property on a constraint by its constraint_code

Informational-marked constraints still surface via retrieval / Lever C,
but their outcomes are overridden to not_applicable at RecordOutcome time
so they don't count against the actionable follow-rate metric. Use for
meta / epistemic / preference rules that should be visible but aren't
per-action directives.`,
	}
	cmd.AddCommand(newJiminyConstraintMarkCmd())
	cmd.AddCommand(newJiminyConstraintListInformationalCmd())
	return cmd
}

func newJiminyConstraintMarkCmd() *cobra.Command {
	var (
		spaceID        string
		code           string
		informational  bool
		unmark         bool
		dryRun         bool
	)
	cmd := &cobra.Command{
		Use:   "mark",
		Short: "Mark a constraint informational (or reverse via --unmark)",
		Long: `Set is_informational=true (default) or false (--unmark) on the
constraint/correction node with the given constraint_code in the given
space. Reversible — pass --unmark to clear the flag.

Example:
  mdemg jiminy constraint mark --code trust-signal-must-be-persisted-never-ignore-honest --space-id mdemg-dev
  mdemg jiminy constraint mark --code trust-signal-must-be-persisted-never-ignore-honest --space-id mdemg-dev --unmark`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if code == "" {
				return fmt.Errorf("--code is required")
			}
			if spaceID == "" {
				return fmt.Errorf("--space-id is required (never mutate on an unbounded space)")
			}
			if unmark {
				informational = false
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()
			return runJiminyConstraintMark(ctx, cmd, spaceID, code, informational, dryRun)
		},
	}
	cmd.Flags().StringVar(&code, "code", "", "constraint_code to mark (required)")
	cmd.Flags().StringVar(&spaceID, "space-id", "", "space_id (required — never mutate on an unbounded space)")
	cmd.Flags().BoolVar(&informational, "informational", true, "value for is_informational (default true)")
	cmd.Flags().BoolVar(&unmark, "unmark", false, "convenience alias for --informational=false")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would change without writing")
	return cmd
}

func newJiminyConstraintListInformationalCmd() *cobra.Command {
	var spaceID string
	cmd := &cobra.Command{
		Use:   "list-informational",
		Short: "List all constraints/corrections currently marked is_informational=true",
		RunE: func(cmd *cobra.Command, args []string) error {
			if spaceID == "" {
				return fmt.Errorf("--space-id is required")
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
			defer cancel()
			return runJiminyConstraintListInformational(ctx, cmd, spaceID)
		},
	}
	cmd.Flags().StringVar(&spaceID, "space-id", "", "space_id (required)")
	return cmd
}

// neo4jDriverFromEnv opens a driver using the shipped Config env-parse.
// Kept local rather than in a shared helper because no other CLI subcommand
// currently needs a raw Neo4j driver — everything else routes through the
// HTTP API. If a second consumer appears, promote this to a shared helper.
func neo4jDriverFromEnv(ctx context.Context) (neo4j.DriverWithContext, error) {
	cfg, err := config.FromEnv()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	drv, err := neo4j.NewDriverWithContext(cfg.Neo4jURI,
		neo4j.BasicAuth(cfg.Neo4jUser, cfg.Neo4jPass, ""))
	if err != nil {
		return nil, fmt.Errorf("open neo4j driver: %w", err)
	}
	if err := drv.VerifyConnectivity(ctx); err != nil {
		_ = drv.Close(ctx)
		return nil, fmt.Errorf("verify neo4j connectivity: %w", err)
	}
	return drv, nil
}

func runJiminyConstraintMark(ctx context.Context, cmd *cobra.Command, spaceID, code string, informational, dryRun bool) error {
	drv, err := neo4jDriverFromEnv(ctx)
	if err != nil {
		return err
	}
	defer drv.Close(ctx) //nolint:errcheck

	fmt.Fprintf(cmd.OutOrStdout(), "MDEMG Jiminy Constraint Mark\n")
	fmt.Fprintf(cmd.OutOrStdout(), "Space:            %s\n", spaceID)
	fmt.Fprintf(cmd.OutOrStdout(), "Constraint code:  %s\n", code)
	fmt.Fprintf(cmd.OutOrStdout(), "New value:        is_informational = %v\n", informational)
	if dryRun {
		fmt.Fprintf(cmd.OutOrStdout(), "Mode:             DRY RUN (no writes)\n")
	}

	// First, resolve which nodes match — helps operator confirm intent + surfaces
	// duplicate-code cases (e.g. must-follow-branch-protection-policy had two
	// copies pre-JIMINY-CORPUS-003; report them here).
	sess := drv.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	res, err := sess.Run(ctx, `
		MATCH (c:MemoryNode)
		WHERE c.space_id = $space AND c.constraint_code = $code
		  AND c.role_type IN ['constraint','correction']
		  AND NOT coalesce(c.is_archived, false)
		RETURN c.node_id AS nid, c.name AS name, coalesce(c.is_informational, false) AS cur
	`, map[string]any{"space": spaceID, "code": code})
	if err != nil {
		_ = sess.Close(ctx)
		return fmt.Errorf("scan matching nodes: %w", err)
	}
	type match struct {
		nid, name string
		cur       bool
	}
	var matches []match
	for res.Next(ctx) {
		rec := res.Record()
		nid, _ := rec.Get("nid")
		name, _ := rec.Get("name")
		cur, _ := rec.Get("cur")
		nidStr, _ := nid.(string)
		nameStr, _ := name.(string)
		curBool, _ := cur.(bool)
		matches = append(matches, match{nid: nidStr, name: nameStr, cur: curBool})
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
		fmt.Fprintf(cmd.OutOrStdout(), "  node_id=%s  name=%q  current=%v\n", m.nid, truncateForCLI(m.name, 80), m.cur)
	}

	if dryRun {
		fmt.Fprintln(cmd.OutOrStdout(), "\n(dry-run — no writes; re-run without --dry-run to apply)")
		return nil
	}

	// Apply
	writeSess := drv.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer writeSess.Close(ctx) //nolint:errcheck
	_, err = writeSess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		return tx.Run(ctx, `
			MATCH (c:MemoryNode)
			WHERE c.space_id = $space AND c.constraint_code = $code
			  AND c.role_type IN ['constraint','correction']
			  AND NOT coalesce(c.is_archived, false)
			SET c.is_informational = $val,
			    c.informational_marked_at = datetime()
			RETURN count(c) AS n
		`, map[string]any{"space": spaceID, "code": code, "val": informational})
	})
	if err != nil {
		return fmt.Errorf("apply is_informational=%v: %w", informational, err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "\n✓ Set is_informational=%v on %d node(s) (code=%q, space=%q)\n", informational, len(matches), code, spaceID)
	return nil
}

func runJiminyConstraintListInformational(ctx context.Context, cmd *cobra.Command, spaceID string) error {
	drv, err := neo4jDriverFromEnv(ctx)
	if err != nil {
		return err
	}
	defer drv.Close(ctx) //nolint:errcheck

	sess := drv.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx) //nolint:errcheck
	res, err := sess.Run(ctx, `
		MATCH (c:MemoryNode)
		WHERE c.space_id = $space
		  AND coalesce(c.is_informational, false) = true
		  AND NOT coalesce(c.is_archived, false)
		RETURN c.constraint_code AS code, c.role_type AS role, c.name AS name,
		       c.node_id AS nid, c.informational_marked_at AS marked_at
		ORDER BY c.constraint_code
	`, map[string]any{"space": spaceID})
	if err != nil {
		return fmt.Errorf("query informational nodes: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Informational-marked constraints/corrections in space=%q:\n\n", spaceID)
	count := 0
	for res.Next(ctx) {
		rec := res.Record()
		code, _ := rec.Get("code")
		role, _ := rec.Get("role")
		name, _ := rec.Get("name")
		nid, _ := rec.Get("nid")
		markedAt, _ := rec.Get("marked_at")
		fmt.Fprintf(cmd.OutOrStdout(), "  code=%-45s role=%-11s marked=%v  name=%q\n",
			asStr(code), asStr(role), markedAt, truncateForCLI(asStr(name), 60))
		fmt.Fprintf(cmd.OutOrStdout(), "    node_id=%s\n", asStr(nid))
		count++
	}
	if err := res.Err(); err != nil {
		return fmt.Errorf("iter: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "\nTotal: %d\n", count)
	return nil
}

func asStr(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

func truncateForCLI(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
