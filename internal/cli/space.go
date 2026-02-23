// Package cli provides CLI commands for the unified MDEMG CLI.
package cli

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"

	devspacepb "mdemg/api/devspacepb"
	pb "mdemg/api/transferpb"
	"mdemg/internal/devspace"
	"mdemg/internal/transfer"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// newSpaceCmd returns the parent "space" command with all subcommands.
func newSpaceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "space",
		Short: "Manage MDEMG spaces (export, import, list, info, serve, pull)",
		Long: `Export/import MDEMG space graphs as .mdemg files or via gRPC.
Requires NEO4J_URI, NEO4J_USER, NEO4J_PASS for Neo4j operations.`,
	}

	cmd.AddCommand(newSpaceExportCmd())
	cmd.AddCommand(newSpaceImportCmd())
	cmd.AddCommand(newSpaceListCmd())
	cmd.AddCommand(newSpaceInfoCmd())
	cmd.AddCommand(newSpaceServeCmd())
	cmd.AddCommand(newSpacePullCmd())

	return cmd
}

// exportConfig holds all flags for the export subcommand.
type exportConfig struct {
	spaceID        string
	output         string
	profile        string
	repoDir        string
	skipGitCheck   bool
	chunkSize      int
	noEmbeddings   bool
	noObservations bool
	noSymbols      bool
	noLearnedEdges bool
	minLayer       int
	maxLayer       int
	sinceTimestamp string
	sinceCursor    string
}

func newSpaceExportCmd() *cobra.Command {
	cfg := &exportConfig{}

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export a space to a .mdemg file",
		Long: `Export a MDEMG space to a .mdemg file.
Supports selective export via profiles and filters.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSpaceExport(cmd.Context(), cfg)
		},
	}

	cmd.Flags().StringVar(&cfg.spaceID, "space-id", "", "Space ID to export (required)")
	cmd.Flags().StringVar(&cfg.output, "output", "", "Output .mdemg file path (default: <space-id>.mdemg)")
	cmd.Flags().StringVar(&cfg.profile, "profile", "full", "Export profile: full | codebase | cms | learned | metadata")
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

	cmd.MarkFlagRequired("space-id")

	return cmd
}

func runSpaceExport(ctx context.Context, cfg *exportConfig) error {
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
	input    string
	conflict string
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

	cmd.MarkFlagRequired("input")

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
			return runSpaceInfo(cmd.Context(), cfg)
		},
	}

	cmd.Flags().StringVar(&cfg.spaceID, "space-id", "", "Space ID (required)")
	cmd.MarkFlagRequired("space-id")

	return cmd
}

func runSpaceInfo(ctx context.Context, cfg *infoConfig) error {
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
	defer lis.Close()

	grpcServer := grpc.NewServer()
	pb.RegisterSpaceTransferServer(grpcServer, transfer.NewGRPCServer(driver))
	if cfg.enableDevSpace {
		catalog, err := devspace.NewCatalog(cfg.devSpaceDataDir)
		if err != nil {
			return fmt.Errorf("devspace catalog: %w", err)
		}
		devspacepb.RegisterDevSpaceServer(grpcServer, devspace.NewServer(catalog, devspace.NewBroker()))
		log.Printf("DevSpace hub enabled (data dir: %s)", cfg.devSpaceDataDir)
	}
	log.Printf("SpaceTransfer gRPC listening on :%d", cfg.port)
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
			return runSpacePull(cmd.Context(), cfg)
		},
	}

	cmd.Flags().StringVar(&cfg.remote, "remote", "", "Remote gRPC address (host:port, required)")
	cmd.Flags().StringVar(&cfg.spaceID, "space-id", "", "Space ID to pull (required)")
	cmd.Flags().StringVar(&cfg.output, "output", "", "Output .mdemg file path (default: <space-id>.mdemg)")

	cmd.MarkFlagRequired("remote")
	cmd.MarkFlagRequired("space-id")

	return cmd
}

func runSpacePull(ctx context.Context, cfg *pullConfig) error {
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
