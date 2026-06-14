// Package cli provides CLI commands for the unified MDEMG CLI.
package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	devspacepb "mdemg/api/devspacepb"
	pb "mdemg/api/transferpb"
	"mdemg/internal/api"
	"mdemg/internal/config"
	"mdemg/internal/devspace"
	"mdemg/internal/embeddings"
	"mdemg/internal/hidden"
	"mdemg/internal/transfer"
)

// newSpaceCmd returns the parent "space" command with all subcommands.
func newSpaceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "space",
		Short: "Manage MDEMG spaces (export, import, list, delete, rename, copy)",
		Long: `Manage MDEMG space graphs: export/import as .mdemg files, list, delete,
rename, copy, and transfer via gRPC.
Requires NEO4J_URI, NEO4J_USER, NEO4J_PASS for Neo4j operations.`,
	}

	cmd.AddCommand(newSpaceExportCmd())
	cmd.AddCommand(newSpaceImportCmd())
	cmd.AddCommand(newSpaceListCmd())
	cmd.AddCommand(newSpaceInfoCmd())
	cmd.AddCommand(newSpaceServeCmd())
	cmd.AddCommand(newSpacePullCmd())
	cmd.AddCommand(newSpaceDeleteCmd())
	cmd.AddCommand(newSpaceRenameCmd())
	cmd.AddCommand(newSpaceCopyCmd())

	return cmd
}

// exportConfig holds all flags for the export subcommand.
type exportConfig struct {
	spaceID         string
	output          string
	profile         string
	repoDir         string
	skipGitCheck    bool
	chunkSize       int
	noEmbeddings    bool
	noObservations  bool
	noSymbols       bool
	noLearnedEdges  bool
	minLayer        int
	maxLayer        int
	sinceTimestamp  string
	sinceCursor     string
	obsTypes        string // comma-separated obs_type filter
	tags            string // comma-separated tag filter
	excludeVolatile bool
	onlyPinned      bool
}

func newSpaceExportCmd() *cobra.Command {
	cfg := &exportConfig{}

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export a space to a .mdemg file",
		Long: `Export a MDEMG space to a .mdemg file.
Supports selective export via profiles and filters.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if cfg.spaceID == "" {
				cfg.spaceID = resolveSpaceID(cmd)
			}
			return runSpaceExport(cmd.Context(), cfg)
		},
	}

	cmd.Flags().StringVar(&cfg.spaceID, "space-id", "", "Space ID to export (or set MDEMG_SPACE_ID)")
	cmd.Flags().StringVar(&cfg.output, "output", "", "Output .mdemg file path (default: <space-id>.mdemg)")
	cmd.Flags().StringVar(&cfg.profile, "profile", "full", "Export profile: full | codebase | cms | learned | metadata | shareable")
	cmd.Flags().StringVar(&cfg.repoDir, "repo", "", "Git repo path; if set, export fails unless repo is clean and up to date with origin/main")
	cmd.Flags().BoolVar(&cfg.skipGitCheck, "skip-git-check", false, "Skip pre-export git check even when -repo is set")
	cmd.Flags().IntVar(&cfg.chunkSize, "chunk-size", 500, "Nodes per chunk")
	cmd.Flags().BoolVar(&cfg.noEmbeddings, "no-embeddings", false, "Exclude embedding vectors to reduce size")
	cmd.Flags().BoolVar(&cfg.noObservations, "no-observations", false, "Exclude observations")
	cmd.Flags().BoolVar(&cfg.noSymbols, "no-symbols", false, "Exclude symbol nodes")
	cmd.Flags().BoolVar(&cfg.noLearnedEdges, "no-learned-edges", false, "Exclude CO_ACTIVATED_WITH edges")
	cmd.Flags().IntVar(&cfg.minLayer, "min-layer", 0, "Minimum layer to export (0 = all)")
	cmd.Flags().IntVar(&cfg.maxLayer, "max-layer", 0, "Maximum layer to export (0 = all)")
	cmd.Flags().StringVar(&cfg.sinceTimestamp, "since-timestamp", "", "Phase 4: export only entities updated after this (ISO8601)")
	cmd.Flags().StringVar(&cfg.sinceCursor, "since-cursor", "", "Phase 4: opaque cursor from prior export next_cursor (used if -since-timestamp empty)")
	cmd.Flags().StringVar(&cfg.obsTypes, "obs-types", "", "Filter L0 nodes by obs_type (comma-separated, e.g. learning,decision,correction)")
	cmd.Flags().StringVar(&cfg.tags, "tags", "", "Filter nodes by tag presence (comma-separated, any match)")
	cmd.Flags().BoolVar(&cfg.excludeVolatile, "exclude-volatile", false, "Skip volatile/ungraduated observations (stability < 0.8)")
	cmd.Flags().BoolVar(&cfg.onlyPinned, "only-pinned", false, "Only export pinned observations (L1+ always included)")

	return cmd
}

func runSpaceExport(ctx context.Context, cfg *exportConfig) error {
	if cfg.spaceID == "" {
		return fmt.Errorf("--space-id is required (or set MDEMG_SPACE_ID)")
	}
	outPath := cfg.output
	if outPath == "" {
		outPath = cfg.spaceID + ".mdemg"
	}
	if !strings.HasSuffix(outPath, ".mdemg") {
		outPath = outPath + ".mdemg"
	}

	if cfg.repoDir != "" && !cfg.skipGitCheck {
		if err := preExportGitCheck(cfg.repoDir); err != nil {
			return fmt.Errorf("pre-export git check failed: %w", err)
		}
	}

	driver, err := newDriver()
	if err != nil {
		return fmt.Errorf("neo4j config: %w", err)
	}
	defer driver.Close(ctx)
	if err := driver.VerifyConnectivity(ctx); err != nil {
		return fmt.Errorf("neo4j connect: %w", err)
	}

	exportCfg, err := transfer.ExportConfigForProfile(cfg.spaceID, cfg.profile)
	if err != nil {
		return fmt.Errorf("profile: %w", err)
	}
	exportCfg.ChunkSize = cfg.chunkSize
	exportCfg.MinLayer = cfg.minLayer
	exportCfg.MaxLayer = cfg.maxLayer
	if cfg.noEmbeddings {
		exportCfg.IncludeEmbeddings = false
	}
	if cfg.noObservations {
		exportCfg.IncludeObservations = false
	}
	if cfg.noSymbols {
		exportCfg.IncludeSymbols = false
	}
	if cfg.noLearnedEdges {
		exportCfg.IncludeLearnedEdges = false
		if exportCfg.OnlyLearnedEdges {
			exportCfg.OnlyLearnedEdges = false
		}
	}
	exportCfg.SinceTimestamp = cfg.sinceTimestamp
	exportCfg.SinceCursor = cfg.sinceCursor
	if exportCfg.SinceTimestamp == "" && exportCfg.SinceCursor != "" {
		exportCfg.SinceTimestamp = exportCfg.SinceCursor
	}
	// Shareable filters compose with profile defaults
	if cfg.obsTypes != "" {
		exportCfg.ObsTypes = strings.Split(cfg.obsTypes, ",")
	}
	if cfg.tags != "" {
		exportCfg.Tags = strings.Split(cfg.tags, ",")
	}
	if cfg.excludeVolatile {
		exportCfg.ExcludeVolatile = true
	}
	if cfg.onlyPinned {
		exportCfg.OnlyPinned = true
	}
	exportCfg.ProgressFunc = func(phase string, done, total int64) {
		if total > 0 {
			fmt.Fprintf(os.Stderr, "  %s: %d/%d\n", phase, done, total)
		} else {
			fmt.Fprintf(os.Stderr, "  %s: %d\n", phase, done)
		}
	}

	ex := transfer.NewExporter(driver)
	result, err := ex.Export(ctx, exportCfg)
	if err != nil {
		return fmt.Errorf("export failed: %w", err)
	}

	if err := transfer.WriteFile(outPath, result); err != nil {
		return fmt.Errorf("write file failed: %w", err)
	}

	fmt.Printf("Exported %s to %s (%d chunks)\n", cfg.spaceID, outPath, len(result.Chunks))
	if len(result.Chunks) > 0 {
		if sum := result.Chunks[len(result.Chunks)-1].GetSummary(); sum != nil && sum.NextCursor != "" {
			fmt.Fprintf(os.Stderr, "Next cursor for delta: %s\n", sum.NextCursor)
		}
	}
	return nil
}

// importConfig holds all flags for the import subcommand.
type importConfig struct {
	input       string
	conflict    string
	consolidate bool
	reEmbed     bool
	targetSpace string
}

func newSpaceImportCmd() *cobra.Command {
	cfg := &importConfig{}

	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import a .mdemg file into Neo4j",
		Long:  `Import a MDEMG space from a .mdemg file into Neo4j.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSpaceImport(cmd.Context(), cfg)
		},
	}

	cmd.Flags().StringVar(&cfg.input, "input", "", "Input .mdemg file path (required)")
	cmd.Flags().StringVar(&cfg.conflict, "conflict", "skip", "On node collision: skip | overwrite | error")
	cmd.Flags().BoolVar(&cfg.consolidate, "consolidate", false, "Run hidden layer consolidation after import")
	cmd.Flags().BoolVar(&cfg.reEmbed, "re-embed", false, "Re-generate embeddings for imported nodes (requires embedding provider)")
	cmd.Flags().StringVar(&cfg.targetSpace, "target-space", "", "Import into a different space_id (remaps all space_id values)")

	_ = cmd.MarkFlagRequired("input")

	return cmd
}

func runSpaceImport(ctx context.Context, cfg *importConfig) error {
	var mode pb.ConflictMode
	switch cfg.conflict {
	case "skip":
		mode = pb.ConflictMode_CONFLICT_SKIP
	case "overwrite":
		mode = pb.ConflictMode_CONFLICT_OVERWRITE
	case "error":
		mode = pb.ConflictMode_CONFLICT_ERROR
	default:
		return fmt.Errorf("invalid -conflict %q; use skip, overwrite, or error", cfg.conflict)
	}

	chunks, err := transfer.ReadFile(cfg.input)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	// Remap space_id if --target-space is set
	if cfg.targetSpace != "" {
		for _, chunk := range chunks {
			chunk.SpaceId = cfg.targetSpace
			if chunk.Metadata != nil {
				chunk.Metadata.SpaceId = cfg.targetSpace
			}
			if chunk.Nodes != nil {
				for _, n := range chunk.Nodes.Nodes {
					n.SpaceId = cfg.targetSpace
				}
			}
			if chunk.Edges != nil {
				for _, e := range chunk.Edges.Edges {
					e.SpaceId = cfg.targetSpace
				}
			}
			if chunk.Observations != nil {
				for _, o := range chunk.Observations.Observations {
					o.SpaceId = cfg.targetSpace
				}
			}
			if chunk.Symbols != nil {
				for _, s := range chunk.Symbols.Symbols {
					s.SpaceId = cfg.targetSpace
				}
			}
		}
	}

	driver, err := newDriver()
	if err != nil {
		return fmt.Errorf("neo4j config: %w", err)
	}
	defer driver.Close(ctx)
	if err := driver.VerifyConnectivity(ctx); err != nil {
		return fmt.Errorf("neo4j connect: %w", err)
	}

	if err := transfer.ValidateImport(ctx, driver, chunks); err != nil {
		return fmt.Errorf("import validation failed: %w", err)
	}

	imp := transfer.NewImporter(driver, mode)
	imp.ProgressFunc = func(phase string, done, total int64) {
		if total > 0 {
			fmt.Fprintf(os.Stderr, "  %s: %d/%d\n", phase, done, total)
		} else {
			fmt.Fprintf(os.Stderr, "  %s: %d\n", phase, done)
		}
	}
	result, err := imp.Import(ctx, chunks)
	if err != nil {
		return fmt.Errorf("import failed: %w", err)
	}

	for _, w := range result.Warnings {
		fmt.Fprintln(os.Stderr, "Warning:", w)
	}
	fmt.Printf("Import complete: nodes created=%d skipped=%d overwritten=%d edges=%d obs=%d symbols=%d (duration %v)\n",
		result.NodesCreated, result.NodesSkipped, result.NodesOverwritten,
		result.EdgesCreated, result.ObservationsCreated, result.SymbolsCreated, result.Duration)

	// Determine effective space ID for post-import actions
	spaceID := cfg.targetSpace
	if spaceID == "" && len(chunks) > 0 {
		spaceID = chunks[0].SpaceId
	}

	// Post-import: re-embed
	if cfg.reEmbed && spaceID != "" {
		fmt.Fprintln(os.Stderr, "Re-embedding imported nodes...")
		if err := reEmbedNodes(ctx, driver, spaceID); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: re-embed failed: %v\n", err)
		}
	}

	// Post-import: consolidate
	if cfg.consolidate && spaceID != "" {
		fmt.Fprintln(os.Stderr, "Running post-import consolidation...")
		if err := runPostImportConsolidation(ctx, driver, spaceID); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: consolidation failed: %v\n", err)
		}
	}

	return nil
}

// runPostImportConsolidation runs the hidden layer pipeline to create L1+ abstractions.
func runPostImportConsolidation(ctx context.Context, driver neo4j.DriverWithContext, spaceID string) error {
	appCfg, err := config.FromEnv()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	svc := hidden.NewService(appCfg, driver, nil)
	result, err := svc.RunNodeCreationPipeline(ctx, spaceID, appCfg.EmergenceEnabled)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Consolidation complete: %d steps, %d nodes created\n",
		len(result.Steps), result.TotalNodes)
	return nil
}

// reEmbedNodes re-generates embeddings for nodes with content but missing embeddings.
func reEmbedNodes(ctx context.Context, driver neo4j.DriverWithContext, spaceID string) error {
	appCfg, err := config.FromEnv()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	embCfg := embeddings.Config{
		Provider:         appCfg.EmbeddingProvider,
		OpenAIAPIKey:     appCfg.OpenAIAPIKey,
		OpenAIModel:      appCfg.OpenAIModel,
		OpenAIEndpoint:   appCfg.OpenAIEndpoint,
		OllamaEndpoint:   appCfg.OllamaEndpoint,
		OllamaModel:      appCfg.OllamaModel,
		TargetDimensions: appCfg.EmbeddingTargetDims,
	}
	embedder, err := embeddings.New(embCfg)
	if err != nil {
		return fmt.Errorf("init embedder: %w", err)
	}

	type nodeContent struct {
		ID      string
		Content string
	}

	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	res, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		r, err := tx.Run(ctx, `
MATCH (n:MemoryNode {space_id: $spaceId})
WHERE n.content IS NOT NULL AND n.content <> '' AND n.embedding IS NULL
RETURN n.node_id AS nodeId, n.content AS content
LIMIT 1000`, map[string]any{"spaceId": spaceID})
		if err != nil {
			return nil, err
		}
		var nodes []nodeContent
		for r.Next(ctx) {
			rec := r.Record()
			nid, _ := rec.Get("nodeId")
			cnt, _ := rec.Get("content")
			nodes = append(nodes, nodeContent{ID: nid.(string), Content: cnt.(string)})
		}
		return nodes, r.Err()
	})
	sess.Close(ctx)
	if err != nil {
		return fmt.Errorf("query nodes: %w", err)
	}

	nodes := res.([]nodeContent)
	if len(nodes) == 0 {
		fmt.Fprintln(os.Stderr, "No nodes need re-embedding.")
		return nil
	}

	// Batch embed
	const batchSize = 50
	var totalEmbedded int
	for i := 0; i < len(nodes); i += batchSize {
		end := i + batchSize
		if end > len(nodes) {
			end = len(nodes)
		}
		batch := nodes[i:end]

		texts := make([]string, len(batch))
		for j, n := range batch {
			texts[j] = n.Content
		}

		vecs, err := embedder.EmbedBatch(ctx, texts)
		if err != nil {
			return fmt.Errorf("embed batch: %w", err)
		}

		wSess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		for j, n := range batch {
			emb64 := make([]float64, len(vecs[j]))
			for k, v := range vecs[j] {
				emb64[k] = float64(v)
			}
			_, err := wSess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
				_, err := tx.Run(ctx, `
MATCH (n:MemoryNode {space_id: $spaceId, node_id: $nodeId})
SET n.embedding = $embedding`, map[string]any{
					"spaceId":   spaceID,
					"nodeId":    n.ID,
					"embedding": emb64,
				})
				return nil, err
			})
			if err != nil {
				wSess.Close(ctx)
				return fmt.Errorf("write embedding for %s: %w", n.ID, err)
			}
		}
		wSess.Close(ctx)
		totalEmbedded += len(batch)
		fmt.Fprintf(os.Stderr, "  Re-embedded %d/%d nodes\n", totalEmbedded, len(nodes))
	}

	return nil
}

// listConfig holds all flags for the list subcommand.
type listConfig struct{}

func newSpaceListCmd() *cobra.Command {
	cfg := &listConfig{}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all spaces (by node count)",
		Long:  `List all MDEMG spaces in the Neo4j database.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSpaceList(cmd.Context(), cfg)
		},
	}

	return cmd
}

func runSpaceList(ctx context.Context, cfg *listConfig) error {
	driver, err := newDriver()
	if err != nil {
		return fmt.Errorf("neo4j config: %w", err)
	}
	defer driver.Close(ctx)
	if err := driver.VerifyConnectivity(ctx); err != nil {
		return fmt.Errorf("neo4j connect: %w", err)
	}

	spaces, err := transfer.ListSpaces(ctx, driver)
	if err != nil {
		return fmt.Errorf("list spaces: %w", err)
	}
	if len(spaces) == 0 {
		fmt.Println("No spaces found.")
		return nil
	}
	fmt.Println("Space ID          | Nodes   | Max layer")
	fmt.Println("------------------+---------+----------")
	for _, s := range spaces {
		fmt.Printf("%-17s | %7d | %d\n", s.SpaceId, s.NodeCount, s.MaxLayer)
	}

	return nil
}

// infoConfig holds all flags for the info subcommand.
type infoConfig struct {
	spaceID string
}

func newSpaceInfoCmd() *cobra.Command {
	cfg := &infoConfig{}

	cmd := &cobra.Command{
		Use:   "info",
		Short: "Show detailed info for one space",
		Long:  `Show detailed information for a specific MDEMG space.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if cfg.spaceID == "" {
				cfg.spaceID = resolveSpaceID(cmd)
			}
			return runSpaceInfo(cmd.Context(), cfg)
		},
	}

	cmd.Flags().StringVar(&cfg.spaceID, "space-id", "", "Space ID (or set MDEMG_SPACE_ID)")

	return cmd
}

func runSpaceInfo(ctx context.Context, cfg *infoConfig) error {
	if cfg.spaceID == "" {
		return fmt.Errorf("--space-id is required (or set MDEMG_SPACE_ID)")
	}
	driver, err := newDriver()
	if err != nil {
		return fmt.Errorf("neo4j config: %w", err)
	}
	defer driver.Close(ctx)
	if err := driver.VerifyConnectivity(ctx); err != nil {
		return fmt.Errorf("neo4j connect: %w", err)
	}

	info, err := transfer.GetSpaceInfo(ctx, driver, cfg.spaceID)
	if err != nil {
		return fmt.Errorf("space info: %w", err)
	}
	sum := info.Summary
	fmt.Printf("Space ID:    %s\n", sum.SpaceId)
	fmt.Printf("Nodes:       %d\n", sum.NodeCount)
	fmt.Printf("Edges:       %d\n", sum.EdgeCount)
	fmt.Printf("Observations: %d\n", sum.ObservationCount)
	fmt.Printf("Symbols:     %d\n", sum.SymbolCount)
	fmt.Printf("Max layer:   %d\n", sum.MaxLayer)
	fmt.Printf("Schema:      %d\n", info.SchemaVersion)
	fmt.Printf("Embed dims:  %d\n", info.EmbeddingDimensions)
	if sum.LastUpdated != "" {
		fmt.Printf("Last updated: %s\n", sum.LastUpdated)
	}
	if len(info.EdgeTypes) > 0 {
		fmt.Printf("Edge types:  %s\n", strings.Join(info.EdgeTypes, ", "))
	}

	return nil
}

// serveConfig holds all flags for the serve subcommand.
type serveConfig struct {
	port            int
	enableDevSpace  bool
	devSpaceDataDir string
}

func newSpaceServeCmd() *cobra.Command {
	cfg := &serveConfig{}

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run gRPC server for remote pull (default port 50051)",
		Long:  `Run a gRPC server for remote space export/import operations.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSpaceServe(cmd.Context(), cfg)
		},
	}

	cmd.Flags().IntVar(&cfg.port, "port", 50051, "gRPC listen port")
	cmd.Flags().BoolVar(&cfg.enableDevSpace, "enable-devspace", false, "Enable DevSpace hub (RegisterAgent, ListExports, PublishExport, PullExport)")
	cmd.Flags().StringVar(&cfg.devSpaceDataDir, "devspace-data-dir", ".devspace/data", "Directory for DevSpace export files (used when -enable-devspace)")

	return cmd
}

func runSpaceServe(ctx context.Context, cfg *serveConfig) error {
	driver, err := newDriver()
	if err != nil {
		return fmt.Errorf("neo4j config: %w", err)
	}
	defer driver.Close(ctx)
	if err := driver.VerifyConnectivity(ctx); err != nil {
		return fmt.Errorf("neo4j connect: %w", err)
	}

	lis, err := net.Listen("tcp", ":"+strconv.Itoa(cfg.port))
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	defer func() { _ = lis.Close() }()

	grpcServer := grpc.NewServer()
	pb.RegisterSpaceTransferServer(grpcServer, transfer.NewGRPCServer(driver))
	if cfg.enableDevSpace {
		catalog, err := devspace.NewCatalog(cfg.devSpaceDataDir)
		if err != nil {
			return fmt.Errorf("devspace catalog: %w", err)
		}
		devspacepb.RegisterDevSpaceServer(grpcServer, devspace.NewServer(catalog, devspace.NewBroker()))
		slog.Info("DevSpace hub enabled", "data_dir", cfg.devSpaceDataDir)
	}
	slog.Info("SpaceTransfer gRPC listening", "port", cfg.port)
	if err := grpcServer.Serve(lis); err != nil {
		return fmt.Errorf("serve: %w", err)
	}

	return nil
}

// pullConfig holds all flags for the pull subcommand.
type pullConfig struct {
	remote  string
	spaceID string
	output  string
}

func newSpacePullCmd() *cobra.Command {
	cfg := &pullConfig{}

	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Pull a space from a remote gRPC server to a .mdemg file",
		Long:  `Pull a MDEMG space from a remote gRPC server and save it to a .mdemg file.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if cfg.spaceID == "" {
				cfg.spaceID = resolveSpaceID(cmd)
			}
			return runSpacePull(cmd.Context(), cfg)
		},
	}

	cmd.Flags().StringVar(&cfg.remote, "remote", "", "Remote gRPC address (host:port, required)")
	cmd.Flags().StringVar(&cfg.spaceID, "space-id", "", "Space ID to pull (or set MDEMG_SPACE_ID)")
	cmd.Flags().StringVar(&cfg.output, "output", "", "Output .mdemg file path (default: <space-id>.mdemg)")

	_ = cmd.MarkFlagRequired("remote")

	return cmd
}

func runSpacePull(ctx context.Context, cfg *pullConfig) error {
	if cfg.spaceID == "" {
		return fmt.Errorf("--space-id is required (or set MDEMG_SPACE_ID)")
	}
	outPath := cfg.output
	if outPath == "" {
		outPath = cfg.spaceID + ".mdemg"
	}
	if !strings.HasSuffix(outPath, ".mdemg") {
		outPath = outPath + ".mdemg"
	}

	conn, err := grpc.NewClient(cfg.remote, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("dial %s: %w", cfg.remote, err)
	}
	defer conn.Close()

	client := pb.NewSpaceTransferClient(conn)
	stream, err := client.Export(ctx, &pb.ExportRequest{SpaceId: cfg.spaceID})
	if err != nil {
		return fmt.Errorf("export: %w", err)
	}

	var chunks []*pb.SpaceChunk
	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("recv: %w", err)
		}
		chunks = append(chunks, chunk)
	}

	result := &transfer.ExportResult{Chunks: chunks}
	if err := transfer.WriteFile(outPath, result); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	fmt.Printf("Pulled %s from %s to %s (%d chunks)\n", cfg.spaceID, cfg.remote, outPath, len(chunks))
	return nil
}

// Helper functions (shared by all space subcommands).

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func newDriver() (neo4j.DriverWithContext, error) {
	// Load config (YAML → .env → env vars)
	if cfgPath := config.FindConfigFile(); cfgPath != "" {
		loadYAMLConfigOrWarn(cfgPath)
	}
	_ = godotenv.Load()

	uri := getEnv("NEO4J_URI", "")
	user := getEnv("NEO4J_USER", "")
	pass := getEnv("NEO4J_PASS", "")
	if uri == "" || user == "" || pass == "" {
		return nil, fmt.Errorf("NEO4J_URI, NEO4J_USER, NEO4J_PASS are required")
	}
	return neo4j.NewDriverWithContext(uri, neo4j.BasicAuth(user, pass, ""))
}

// preExportGitCheck ensures the repo at dir has a clean working tree and is
// not behind origin/main. Used when sharing spaces from a shared codebase.
func preExportGitCheck(dir string) error {
	// Working tree clean
	out, err := exec.CommandContext(context.Background(), "git", "-C", dir, "status", "--porcelain").Output()
	if err != nil {
		return fmt.Errorf("git status: %w", err)
	}
	if len(out) > 0 {
		return fmt.Errorf("working tree not clean in %s (commit or stash changes before export)", dir)
	}
	// Fetch origin so we can compare with origin/main
	_ = exec.CommandContext(context.Background(), "git", "-C", dir, "fetch", "origin", "main").Run()
	// Not behind origin/main
	cmd := exec.CommandContext(context.Background(), "git", "-C", dir, "rev-list", "HEAD..origin/main", "--count")
	revOut, err := cmd.Output()
	if err != nil {
		// origin/main may not exist (e.g. no remote or branch not pushed)
		return fmt.Errorf("cannot compare with origin/main: %w", err)
	}
	count := strings.TrimSpace(string(revOut))
	if count != "" && count != "0" {
		return fmt.Errorf("branch is behind origin/main by %s commit(s); pull before export", count)
	}
	return nil
}

// --- delete subcommand ---

type deleteConfig struct {
	spaceID    string
	skipConfirm bool
}

func newSpaceDeleteCmd() *cobra.Command {
	cfg := &deleteConfig{}

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete all nodes in a space",
		Long: `Delete all nodes and edges belonging to a space from Neo4j.

Protected spaces (e.g., mdemg-dev) cannot be deleted.
Deletion is performed in batches and is irreversible.

Examples:
  mdemg space delete --space-id my-test-space
  mdemg space delete --space-id my-test-space --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if cfg.spaceID == "" {
				cfg.spaceID = resolveSpaceID(cmd)
			}
			return runSpaceDelete(cmd.Context(), cfg)
		},
	}

	cmd.Flags().StringVar(&cfg.spaceID, "space-id", "", "Space ID to delete (or set MDEMG_SPACE_ID)")
	cmd.Flags().BoolVarP(&cfg.skipConfirm, "yes", "y", false, "Skip confirmation prompt")

	return cmd
}

func runSpaceDelete(ctx context.Context, cfg *deleteConfig) error {
	if cfg.spaceID == "" {
		return fmt.Errorf("--space-id is required (or set MDEMG_SPACE_ID)")
	}

	// Protected space check
	if api.IsProtectedSpace(cfg.spaceID) {
		return fmt.Errorf("space %q is protected and cannot be deleted", cfg.spaceID)
	}

	driver, err := newDriver()
	if err != nil {
		return fmt.Errorf("neo4j config: %w", err)
	}
	defer driver.Close(ctx)
	if err := driver.VerifyConnectivity(ctx); err != nil {
		return fmt.Errorf("neo4j connect: %w", err)
	}

	// Count nodes first — count ALL labels, not just MemoryNode. The delete
	// below is label-agnostic (MATCH (n {space_id})), so gating the pre-check
	// on MemoryNode alone silently skipped spaces holding only SymbolNodes /
	// Observations (e.g. codebase-symbol test spaces) — they reported "no
	// nodes" while their nodes survived. Match the pre-check to the delete.
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	result, err := sess.Run(ctx,
		"MATCH (n {space_id: $sid}) RETURN count(n) as cnt",
		map[string]any{"sid": cfg.spaceID})
	if err != nil {
		sess.Close(ctx)
		return fmt.Errorf("count query: %w", err)
	}
	var nodeCount int64
	if result.Next(ctx) {
		nodeCount = result.Record().Values[0].(int64)
	}
	sess.Close(ctx)

	if nodeCount == 0 {
		fmt.Printf("Space %q has no nodes. Nothing to delete.\n", cfg.spaceID)
		return nil
	}

	// Confirmation
	if !cfg.skipConfirm {
		fmt.Printf("Delete space %q? This will remove %d nodes and all their edges.\n", cfg.spaceID, nodeCount)
		fmt.Print("Type 'yes' to confirm: ")
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			if strings.TrimSpace(scanner.Text()) != "yes" {
				fmt.Println("Aborted.")
				return nil
			}
		}
	}

	// Batch delete
	const batchSize = 1000
	var totalDeleted int64

	for {
		sess = driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		result, err = sess.Run(ctx,
			"MATCH (n {space_id: $sid}) WITH n LIMIT $batch DETACH DELETE n RETURN count(*) as deleted",
			map[string]any{"sid": cfg.spaceID, "batch": batchSize})
		if err != nil {
			sess.Close(ctx)
			return fmt.Errorf("delete batch: %w", err)
		}

		var deleted int64
		if result.Next(ctx) {
			deleted = result.Record().Values[0].(int64)
		}
		sess.Close(ctx)

		totalDeleted += deleted
		if deleted > 0 {
			fmt.Printf("[%s] Deleted %d nodes (total: %d)\n", time.Now().Format("15:04:05"), deleted, totalDeleted)
		}
		if deleted == 0 {
			break
		}
	}

	fmt.Printf("Deleted space %q: %d nodes removed.\n", cfg.spaceID, totalDeleted)
	return nil
}

// --- rename subcommand ---

type renameConfig struct {
	from string
	to   string
}

func newSpaceRenameCmd() *cobra.Command {
	cfg := &renameConfig{}

	cmd := &cobra.Command{
		Use:   "rename",
		Short: "Rename a space (change space_id on all nodes)",
		Long: `Rename a space by updating the space_id property on all its nodes.

Protected spaces (e.g., mdemg-dev) cannot be renamed.
The target space name must not already exist.

Examples:
  mdemg space rename --from old-project --to new-project`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSpaceRename(cmd.Context(), cfg)
		},
	}

	cmd.Flags().StringVar(&cfg.from, "from", "", "Current space ID (required)")
	cmd.Flags().StringVar(&cfg.to, "to", "", "New space ID (required)")
	_ = cmd.MarkFlagRequired("from")
	_ = cmd.MarkFlagRequired("to")

	return cmd
}

func runSpaceRename(ctx context.Context, cfg *renameConfig) error {
	if cfg.from == cfg.to {
		return fmt.Errorf("--from and --to cannot be the same")
	}

	// Protected space check
	if api.IsProtectedSpace(cfg.from) {
		return fmt.Errorf("space %q is protected and cannot be renamed", cfg.from)
	}

	driver, err := newDriver()
	if err != nil {
		return fmt.Errorf("neo4j config: %w", err)
	}
	defer driver.Close(ctx)
	if err := driver.VerifyConnectivity(ctx); err != nil {
		return fmt.Errorf("neo4j connect: %w", err)
	}

	// Check target doesn't exist
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	result, err := sess.Run(ctx, "MATCH (n:MemoryNode {space_id: $sid}) RETURN count(n) as cnt", map[string]any{"sid": cfg.to})
	if err != nil {
		sess.Close(ctx)
		return fmt.Errorf("check target: %w", err)
	}
	if result.Next(ctx) {
		if cnt := result.Record().Values[0].(int64); cnt > 0 {
			sess.Close(ctx)
			return fmt.Errorf("target space %q already exists (%d nodes)", cfg.to, cnt)
		}
	}
	sess.Close(ctx)

	// Batch rename
	const batchSize = 1000
	var totalRenamed int64

	for {
		sess = driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		result, err = sess.Run(ctx,
			"MATCH (n {space_id: $from}) WITH n LIMIT $batch SET n.space_id = $to RETURN count(*) as updated",
			map[string]any{"from": cfg.from, "to": cfg.to, "batch": batchSize})
		if err != nil {
			sess.Close(ctx)
			return fmt.Errorf("rename batch: %w", err)
		}

		var updated int64
		if result.Next(ctx) {
			updated = result.Record().Values[0].(int64)
		}
		sess.Close(ctx)

		totalRenamed += updated
		if updated > 0 {
			fmt.Printf("[%s] Renamed %d nodes (total: %d)\n", time.Now().Format("15:04:05"), updated, totalRenamed)
		}
		if updated == 0 {
			break
		}
	}

	fmt.Printf("Renamed space %q → %q: %d nodes updated.\n", cfg.from, cfg.to, totalRenamed)
	return nil
}

// --- copy subcommand ---

type copyConfig struct {
	from string
	to   string
}

func newSpaceCopyCmd() *cobra.Command {
	cfg := &copyConfig{}

	cmd := &cobra.Command{
		Use:   "copy",
		Short: "Copy a space (duplicate all nodes with a new space_id)",
		Long: `Copy a space by duplicating all its nodes with a new space_id.

Edges between nodes within the space are also duplicated.
The target space name must not already exist.
New nodes receive fresh node_id values to avoid collisions.

Examples:
  mdemg space copy --from production --to staging`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSpaceCopy(cmd.Context(), cfg)
		},
	}

	cmd.Flags().StringVar(&cfg.from, "from", "", "Source space ID (required)")
	cmd.Flags().StringVar(&cfg.to, "to", "", "Target space ID (required)")
	_ = cmd.MarkFlagRequired("from")
	_ = cmd.MarkFlagRequired("to")

	return cmd
}

func runSpaceCopy(ctx context.Context, cfg *copyConfig) error {
	if cfg.from == cfg.to {
		return fmt.Errorf("--from and --to cannot be the same")
	}

	driver, err := newDriver()
	if err != nil {
		return fmt.Errorf("neo4j config: %w", err)
	}
	defer driver.Close(ctx)
	if err := driver.VerifyConnectivity(ctx); err != nil {
		return fmt.Errorf("neo4j connect: %w", err)
	}

	// Check source exists
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	result, err := sess.Run(ctx, "MATCH (n:MemoryNode {space_id: $sid}) RETURN count(n) as cnt", map[string]any{"sid": cfg.from})
	if err != nil {
		sess.Close(ctx)
		return fmt.Errorf("check source: %w", err)
	}
	var sourceCount int64
	if result.Next(ctx) {
		sourceCount = result.Record().Values[0].(int64)
	}
	sess.Close(ctx)

	if sourceCount == 0 {
		return fmt.Errorf("source space %q has no nodes", cfg.from)
	}

	// Check target doesn't exist
	sess = driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	result, err = sess.Run(ctx, "MATCH (n:MemoryNode {space_id: $sid}) RETURN count(n) as cnt", map[string]any{"sid": cfg.to})
	if err != nil {
		sess.Close(ctx)
		return fmt.Errorf("check target: %w", err)
	}
	if result.Next(ctx) {
		if cnt := result.Record().Values[0].(int64); cnt > 0 {
			sess.Close(ctx)
			return fmt.Errorf("target space %q already exists (%d nodes)", cfg.to, cnt)
		}
	}
	sess.Close(ctx)

	fmt.Printf("Copying space %q (%d nodes) → %q...\n", cfg.from, sourceCount, cfg.to)

	// Step 1: Collect all source node IDs upfront
	sess = driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	result, err = sess.Run(ctx,
		"MATCH (src:MemoryNode {space_id: $from}) RETURN src.node_id AS nid",
		map[string]any{"from": cfg.from})
	if err != nil {
		sess.Close(ctx)
		return fmt.Errorf("collect source IDs: %w", err)
	}

	var sourceIDs []string
	for result.Next(ctx) {
		if nid, ok := result.Record().Values[0].(string); ok {
			sourceIDs = append(sourceIDs, nid)
		}
	}
	sess.Close(ctx)

	if len(sourceIDs) == 0 {
		return fmt.Errorf("source space %q returned no node IDs", cfg.from)
	}

	// Step 2: Copy nodes in batches by explicit ID list (deterministic termination)
	const batchSize = 500
	var totalCopied int64

	for i := 0; i < len(sourceIDs); i += batchSize {
		end := i + batchSize
		if end > len(sourceIDs) {
			end = len(sourceIDs)
		}
		batch := sourceIDs[i:end]

		sess = driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		result, err = sess.Run(ctx, `
			MATCH (src:MemoryNode {space_id: $from})
			WHERE src.node_id IN $ids
			CREATE (dup:MemoryNode)
			SET dup = properties(src),
			    dup.space_id = $to,
			    dup.node_id = src.node_id + '-copy-' + $to,
			    dup.created_at = datetime(),
			    dup._source_node_id = src.node_id
			RETURN count(*) as copied`,
			map[string]any{"from": cfg.from, "to": cfg.to, "ids": batch})
		if err != nil {
			sess.Close(ctx)
			return fmt.Errorf("copy nodes batch: %w", err)
		}

		var copied int64
		if result.Next(ctx) {
			copied = result.Record().Values[0].(int64)
		}
		sess.Close(ctx)

		totalCopied += copied
		fmt.Printf("[%s] Copied %d nodes (total: %d/%d)\n", time.Now().Format("15:04:05"), copied, totalCopied, sourceCount)
	}

	// Step 2: Copy edges between nodes in the space
	sess = driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	result, err = sess.Run(ctx, `
		MATCH (src1:MemoryNode {space_id: $from})-[r]->(src2:MemoryNode {space_id: $from})
		MATCH (dup1:MemoryNode {space_id: $to, _source_node_id: src1.node_id})
		MATCH (dup2:MemoryNode {space_id: $to, _source_node_id: src2.node_id})
		WITH dup1, dup2, type(r) as relType, properties(r) as relProps
		CALL (dup1, dup2, relType, relProps) {
			WITH dup1, dup2, relType, relProps
			CALL db.createRelationship(dup1, relType, relProps, dup2) YIELD rel
			RETURN rel
		}
		RETURN count(*) as edges_copied`,
		map[string]any{"from": cfg.from, "to": cfg.to})
	var edgesCopied int64
	if err == nil && result.Next(ctx) {
		edgesCopied = result.Record().Values[0].(int64)
	}
	sess.Close(ctx)

	// If the dynamic edge copy failed (db.createRelationship not available),
	// fall back to copying known edge types individually
	if err != nil {
		edgesCopied, err = copyEdgesFallback(ctx, driver, cfg.from, cfg.to)
		if err != nil {
			fmt.Printf("Warning: edge copy failed: %v\n", err)
			fmt.Println("Nodes were copied successfully. Edges must be recreated manually.")
		}
	}

	// Step 3: Clean up temporary _source_node_id markers
	sess = driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	_, _ = sess.Run(ctx,
		"MATCH (n:MemoryNode {space_id: $to}) WHERE n._source_node_id IS NOT NULL REMOVE n._source_node_id",
		map[string]any{"to": cfg.to})
	sess.Close(ctx)

	fmt.Printf("Copied space %q → %q: %d nodes, %d edges.\n", cfg.from, cfg.to, totalCopied, edgesCopied)
	return nil
}

// copyEdgesFallback copies edges by known relationship types when db.createRelationship
// is not available. Covers the standard MDEMG edge types.
func copyEdgesFallback(ctx context.Context, driver neo4j.DriverWithContext, from, to string) (int64, error) {
	edgeTypes := []string{
		"CO_ACTIVATED_WITH",
		"ABSTRACTS",
		"BRIDGES",
		"COMPOSES_WITH",
		"HAS_SYMBOL",
		"RELATES_TO",
	}

	var total int64
	for _, relType := range edgeTypes {
		sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		cypher := fmt.Sprintf(`
			MATCH (src1:MemoryNode {space_id: $from})-[r:%s]->(src2:MemoryNode {space_id: $from})
			MATCH (dup1:MemoryNode {space_id: $to, _source_node_id: src1.node_id})
			MATCH (dup2:MemoryNode {space_id: $to, _source_node_id: src2.node_id})
			CREATE (dup1)-[nr:%s]->(dup2)
			SET nr = properties(r)
			RETURN count(*) as copied`, relType, relType)

		result, err := sess.Run(ctx, cypher, map[string]any{"from": from, "to": to})
		if err != nil {
			sess.Close(ctx)
			return total, fmt.Errorf("copy %s edges: %w", relType, err)
		}
		if result.Next(ctx) {
			copied := result.Record().Values[0].(int64)
			total += copied
			if copied > 0 {
				fmt.Printf("[%s] Copied %d %s edges\n", time.Now().Format("15:04:05"), copied, relType)
			}
		}
		sess.Close(ctx)
	}

	return total, nil
}
