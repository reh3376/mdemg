package api

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"mdemg/internal/pathsafe"
	"mdemg/internal/plugins"
	"mdemg/internal/plugins/scaffold"
)

// pluginIDSafeRe restricts pluginID (parsed from the URL path) to
// characters that cannot escape a filepath.Join boundary. No dots
// (blocks ".." traversal), no slashes (blocks nested-path injection),
// no path separators of any flavor. Alphanumerics + `-` + `_` cover
// every realistic plugin naming convention.
//
// PLUGIN-PATH-INJECTION-FIX (2026-08-10, CodeQL alert #22 critical):
// prior code used `pluginID` directly in filepath.Join(PluginsDir, pluginID)
// with zero validation, allowing a request like
// `POST /v1/plugins/..%2F..%2F..%2Fetc/validate` to resolve pluginDir
// to arbitrary filesystem locations and trigger exec of arbitrary
// binaries via handlePluginValidate → ValidateProtoCompliance.
var pluginIDSafeRe = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// validatePluginID returns nil iff pluginID matches pluginIDSafeRe.
// Handlers MUST call this before using pluginID in any path
// construction. Returns a 400-ready error message.
func validatePluginID(pluginID string) error {
	if pluginID == "" {
		return errPluginIDEmpty
	}
	if !pluginIDSafeRe.MatchString(pluginID) {
		return errPluginIDUnsafe
	}
	return nil
}

// verifyPluginBinaryInside is the belt-and-suspenders containment
// check after building binaryPath via filepath.Join. Even with a
// validated pluginID, a symlink inside the plugin dir could escape;
// this check catches that class. Returns nil if binaryPath resolves
// to a location strictly inside pluginsDir; otherwise the path-
// escape error.
func verifyPluginBinaryInside(pluginsDir, binaryPath string) error {
	absDir, err := filepath.Abs(pluginsDir)
	if err != nil {
		return err
	}
	absBin, err := filepath.Abs(binaryPath)
	if err != nil {
		return err
	}
	// Canonicalize BOTH sides via EvalSymlinks so the containment
	// comparison is apples-to-apples. Required because macOS resolves
	// /var/folders/* → /private/var/folders/*: without this, an
	// operator whose PluginsDir is symlinked (or lives under /var)
	// would see every legit binary rejected as "outside", and the fix
	// would silently break every plugin ops path. Live-caught by the
	// pin test TestVerifyPluginBinaryInside/accepts_inside_binary
	// during the PLUGIN-PATH-INJECTION-FIX build.
	//
	// Ignore not-exist errors from EvalSymlinks — the caller
	// separately verifies binary existence via os.Stat; here we care
	// only about the path SHAPE. Fall back to the abs form when the
	// path doesn't exist yet.
	if resolved, err := filepath.EvalSymlinks(absDir); err == nil {
		absDir = resolved
	}
	if resolved, err := filepath.EvalSymlinks(absBin); err == nil {
		absBin = resolved
	}
	if !strings.HasPrefix(absBin+string(os.PathSeparator), absDir+string(os.PathSeparator)) && absBin != absDir {
		return errPluginBinaryOutsideDir
	}
	return nil
}

// Errors returned by the plugin-path validators. Named so callers
// can `errors.Is` against them for testable error handling.
var (
	errPluginIDEmpty         = &pluginPathError{"plugin_id is required"}
	errPluginIDUnsafe        = &pluginPathError{"plugin_id must match ^[a-zA-Z0-9_-]{1,64}$ (no dots, no slashes, no path separators)"}
	errPluginBinaryOutsideDir = &pluginPathError{"plugin binary path resolves outside the plugins directory (rejected)"}
)

// pluginPathError is a lightweight sentinel type — small enough to
// inline; supports errors.Is via pointer identity.
type pluginPathError struct{ msg string }

func (e *pluginPathError) Error() string { return e.msg }

// PluginCreateRequest is the request body for POST /v1/plugins/create
type PluginCreateRequest struct {
	Name         string   `json:"name"`
	Type         string   `json:"type"` // INGESTION, REASONING, or APE
	Version      string   `json:"version,omitempty"`
	Description  string   `json:"description,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	Author       string   `json:"author,omitempty"`
}

// PluginCreateResponse is the response for POST /v1/plugins/create
type PluginCreateResponse struct {
	PluginID     string                  `json:"plugin_id"`
	PluginPath   string                  `json:"plugin_path"`
	FilesCreated []string                `json:"files_created"`
	Validation   PluginValidationSummary `json:"validation"`
	NextSteps    []string                `json:"next_steps"`
}

// PluginValidationSummary is a summary of validation results
type PluginValidationSummary struct {
	ManifestValid bool     `json:"manifest_valid"`
	Errors        []string `json:"errors,omitempty"`
	Warnings      []string `json:"warnings,omitempty"`
}

// PluginListResponse is the response for GET /v1/plugins
type PluginListResponse struct {
	Plugins []PluginInfo `json:"plugins"`
}

// PluginInfo represents a plugin in the list response
type PluginInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Version string `json:"version"`
	Status  string `json:"status"` // running, stopped, error
	Path    string `json:"path"`
}

// PluginDetailResponse is the response for GET /v1/plugins/{id}
type PluginDetailResponse struct {
	ID           string              `json:"id"`
	Name         string              `json:"name"`
	Type         string              `json:"type"`
	Version      string              `json:"version"`
	Status       string              `json:"status"`
	Path         string              `json:"path"`
	SocketPath   string              `json:"socket_path,omitempty"`
	PID          int                 `json:"pid,omitempty"`
	StartedAt    string              `json:"started_at,omitempty"`
	LastHealthy  string              `json:"last_healthy,omitempty"`
	LastError    string              `json:"last_error,omitempty"`
	Capabilities []string            `json:"capabilities,omitempty"`
	Metrics      map[string]string   `json:"metrics,omitempty"`
	Manifest     *plugins.Manifest   `json:"manifest,omitempty"`
	Health       *PluginHealthStatus `json:"health,omitempty"`
}

// PluginHealthStatus contains health information
type PluginHealthStatus struct {
	Healthy        bool              `json:"healthy"`
	Status         string            `json:"status,omitempty"`
	ResponseTimeMs int64             `json:"response_time_ms,omitempty"`
	Metrics        map[string]string `json:"metrics,omitempty"`
}

// PluginValidateResponse is the response for POST /v1/plugins/{id}/validate
type PluginValidateResponse struct {
	Valid     bool                         `json:"valid"`
	Manifest  *plugins.ManifestValidation  `json:"manifest,omitempty"`
	Proto     *plugins.ProtoValidation     `json:"proto,omitempty"`
	Health    *plugins.HealthValidation    `json:"health,omitempty"`
	Lifecycle *plugins.LifecycleValidation `json:"lifecycle,omitempty"`
}

// handlePluginCreate handles POST /v1/plugins/create
func (s *Server) handlePluginCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	var req PluginCreateRequest
	if !readJSON(w, r, &req) {
		return
	}

	// Validate required fields
	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "name is required"})
		return
	}

	if req.Type == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "type is required"})
		return
	}

	// Validate module type
	moduleType := strings.ToUpper(req.Type)
	if !scaffold.ValidModuleTypes[moduleType] {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "type must be one of: INGESTION, REASONING, APE",
		})
		return
	}

	// Set defaults
	version := req.Version
	if version == "" {
		version = "1.0.0"
	}

	// Generate plugin scaffold
	cfg := scaffold.Config{
		Name:         req.Name,
		Type:         scaffold.ModuleType(moduleType),
		OutputDir:    s.cfg.PluginsDir,
		Version:      version,
		Description:  req.Description,
		Capabilities: req.Capabilities,
		Author:       req.Author,
	}

	result, err := scaffold.Generate(cfg)
	if err != nil {
		writeInternalError(w, err, "generate plugin scaffold")
		return
	}

	// Validate the generated manifest
	validation := PluginValidationSummary{
		ManifestValid: true,
		Errors:        []string{},
		Warnings:      []string{},
	}

	manifestValidation, err := plugins.ValidateManifest(result.PluginPath)
	if err != nil {
		validation.ManifestValid = false
		validation.Errors = append(validation.Errors, "validation error: "+err.Error())
	} else if manifestValidation != nil {
		validation.ManifestValid = manifestValidation.Valid
		validation.Errors = manifestValidation.Errors
		validation.Warnings = manifestValidation.Warnings
	}

	// Build response
	resp := PluginCreateResponse{
		PluginID:     result.PluginID,
		PluginPath:   result.PluginPath,
		FilesCreated: result.FilesCreated,
		Validation:   validation,
		NextSteps: []string{
			"1. Edit handler.go to implement your logic",
			"2. Run 'make build' to compile",
			"3. Run 'go run ./cmd/plugin-validate --plugin=" + result.PluginPath + "' to validate",
		},
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": resp})
}

// handlePluginList handles GET /v1/plugins
func (s *Server) handlePluginList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	pluginList := []PluginInfo{}

	// Get running plugins from plugin manager
	runningPlugins := make(map[string]plugins.ModuleStatus)
	if s.pluginMgr != nil {
		for _, mod := range s.pluginMgr.ListModules() {
			runningPlugins[mod.ID] = mod
		}
	}

	// Scan plugins directory
	pluginsDir := s.cfg.PluginsDir
	entries, err := os.ReadDir(pluginsDir)
	if err != nil && !os.IsNotExist(err) {
		writeInternalError(w, err, "read plugins directory")
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == ".disabled" {
			continue
		}

		pluginDir := filepath.Join(pluginsDir, entry.Name())
		manifestPath := filepath.Join(pluginDir, "manifest.json")

		// Check if manifest exists
		if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
			continue
		}

		// Validate manifest to get info
		manifestValidation, err := plugins.ValidateManifest(pluginDir)
		if err != nil || manifestValidation == nil || manifestValidation.Manifest == nil {
			continue
		}

		manifest := manifestValidation.Manifest
		info := PluginInfo{
			ID:      manifest.ID,
			Name:    manifest.Name,
			Type:    manifest.Type,
			Version: manifest.Version,
			Path:    pluginDir,
			Status:  "stopped",
		}

		// Check if running
		if running, ok := runningPlugins[manifest.ID]; ok {
			switch running.State {
			case "ready":
				info.Status = "running"
			case "unhealthy", "crashed":
				info.Status = "error"
			default:
				info.Status = running.State
			}
		}

		pluginList = append(pluginList, info)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": PluginListResponse{
			Plugins: pluginList,
		},
	})
}

// handlePluginDetail handles GET /v1/plugins/{id}
func (s *Server) handlePluginDetail(w http.ResponseWriter, r *http.Request, pluginID string) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	// PLUGIN-PATH-INJECTION-FIX (2026-08-10): validate pluginID before
	// using in filepath.Join — same class as handlePluginValidate. This
	// handler doesn't exec but does os.Stat + os.ReadFile which could
	// probe/leak arbitrary filesystem locations without validation.
	if err := validatePluginID(pluginID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	// SEC-TRANCHE-2 (2026-08-11): route the join through
	// pathsafe.SafeJoinUnderDir so CodeQL's path-injection detector
	// sees a recognized sanitizer. validatePluginID above catches
	// malicious IDs early with a specific error; SafeJoinUnderDir is
	// the CodeQL-visible containment check.
	pluginDir, err := pathsafe.SafeJoinUnderDir(s.cfg.PluginsDir, pluginID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid plugin id: " + err.Error()})
		return
	}
	manifestPath := filepath.Join(pluginDir, "manifest.json")

	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "plugin not found"})
		return
	}

	// Validate manifest to get full info
	manifestValidation, err := plugins.ValidateManifest(pluginDir)
	if err != nil || manifestValidation == nil || manifestValidation.Manifest == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": "failed to read plugin manifest",
		})
		return
	}

	manifest := manifestValidation.Manifest

	resp := PluginDetailResponse{
		ID:       manifest.ID,
		Name:     manifest.Name,
		Type:     manifest.Type,
		Version:  manifest.Version,
		Path:     pluginDir,
		Status:   "stopped",
		Manifest: manifest,
	}

	// Collect capabilities
	resp.Capabilities = append(resp.Capabilities, manifest.Capabilities.IngestionSources...)
	resp.Capabilities = append(resp.Capabilities, manifest.Capabilities.ContentTypes...)
	resp.Capabilities = append(resp.Capabilities, manifest.Capabilities.PatternDetectors...)
	resp.Capabilities = append(resp.Capabilities, manifest.Capabilities.EventTriggers...)

	// Check if running via plugin manager
	if s.pluginMgr != nil {
		if modInfo, ok := s.pluginMgr.GetModule(manifest.ID); ok {
			switch modInfo.State {
			case plugins.StateReady:
				resp.Status = "running"
			case plugins.StateUnhealthy, plugins.StateCrashed:
				resp.Status = "error"
			default:
				resp.Status = string(modInfo.State)
			}

			resp.SocketPath = modInfo.SocketPath
			resp.PID = modInfo.PID
			if !modInfo.StartedAt.IsZero() {
				resp.StartedAt = modInfo.StartedAt.Format("2006-01-02T15:04:05Z07:00")
			}
			if !modInfo.LastHealthy.IsZero() {
				resp.LastHealthy = modInfo.LastHealthy.Format("2006-01-02T15:04:05Z07:00")
			}
			resp.LastError = modInfo.LastError
			resp.Metrics = modInfo.Metrics

			// Include health status
			resp.Health = &PluginHealthStatus{
				Healthy: modInfo.State == plugins.StateReady,
				Status:  string(modInfo.State),
				Metrics: modInfo.Metrics,
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": resp})
}

// handlePluginValidate handles POST /v1/plugins/{id}/validate
func (s *Server) handlePluginValidate(w http.ResponseWriter, r *http.Request, pluginID string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	// PLUGIN-PATH-INJECTION-FIX (2026-08-10, CodeQL alert #22 critical):
	// pluginID from URL path MUST be validated before any filepath op.
	// Without this, `POST /v1/plugins/..%2F..%2F..%2Fetc/validate` would
	// escape s.cfg.PluginsDir and trigger exec of arbitrary binaries.
	if err := validatePluginID(pluginID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	// SEC-TRANCHE-2 (2026-08-11): CodeQL-recognized sanitizer via
	// filepath.Clean + strings.HasPrefix containment check. The regex
	// validation above is redundant defense-in-depth (validatePluginID
	// gives a clearer error message for the common empty/bad-charset
	// cases); SafeJoinUnderDir is what CodeQL's data-flow closes on.
	pluginDir, err := pathsafe.SafeJoinUnderDir(s.cfg.PluginsDir, pluginID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid plugin id: " + err.Error()})
		return
	}
	manifestPath := filepath.Join(pluginDir, "manifest.json")

	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "plugin not found"})
		return
	}

	resp := PluginValidateResponse{
		Valid: true,
	}

	// Validate manifest
	manifestValidation, err := plugins.ValidateManifest(pluginDir)
	if err != nil {
		writeInternalError(w, err, "manifest validation")
		return
	}
	resp.Manifest = manifestValidation

	if !manifestValidation.Valid {
		resp.Valid = false
		// Still need to provide proto validation info if manifest failed due to missing binary
		if manifestValidation.Manifest != nil && manifestValidation.Manifest.Binary != "" {
			// SEC-TRANCHE-2 (2026-08-11): even in the error-message
			// branch, sanitize through pathsafe. Rendering an unsanitized
			// attacker-controlled path in an error string is a minor
			// info-leak (not exec), but the pattern must be uniform so a
			// future refactor doesn't accidentally exec it.
			binaryPath, _ := pathsafe.SafeJoinUnderDir(pluginDir, manifestValidation.Manifest.Binary)
			if binaryPath == "" {
				binaryPath = "<rejected>"
			}
			resp.Proto = &plugins.ProtoValidation{
				ValidationResult: plugins.ValidationResult{
					Valid:    false,
					Errors:   []string{"binary not found: " + binaryPath + " (run 'make build' first)"},
					Warnings: []string{},
				},
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": resp})
		return
	}

	// SEC-TRANCHE-2 (2026-08-11): manifest.Binary is plugin-author-
	// provided and could be "../../../usr/bin/rm". Route through
	// pathsafe.SafeJoinUnderDir with the plugin's OWN dir as base so
	// the binary MUST resolve inside its containing plugin — same
	// safety envelope as verifyPluginBinaryInside (kept above for
	// callers that still reference it) but as one CodeQL-recognized
	// sanitizer call rather than a join-then-verify pair. Reject
	// BEFORE the os.Stat so a malicious Binary can't even probe
	// arbitrary filesystem locations for existence.
	binaryPath, err := pathsafe.SafeJoinUnderDir(pluginDir, manifestValidation.Manifest.Binary)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "plugin manifest.binary rejected: " + err.Error(),
		})
		return
	}

	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		// Binary doesn't exist yet - skip proto validation
		resp.Proto = &plugins.ProtoValidation{
			ValidationResult: plugins.ValidationResult{
				Valid:    false,
				Errors:   []string{"binary not found: " + binaryPath + " (run 'make build' first)"},
				Warnings: []string{},
			},
		}
		resp.Valid = false
		writeJSON(w, http.StatusOK, map[string]any{"data": resp})
		return
	}

	// Validate proto compliance (starts the binary temporarily)
	protoValidation, err := plugins.ValidateProtoCompliance(binaryPath, manifestValidation.Manifest.Type)
	if err != nil {
		resp.Proto = &plugins.ProtoValidation{
			ValidationResult: plugins.ValidationResult{
				Valid:  false,
				Errors: []string{"proto validation error: " + err.Error()},
			},
		}
		resp.Valid = false
	} else {
		resp.Proto = protoValidation
		if !protoValidation.Valid {
			resp.Valid = false
		}
	}

	// If the plugin is running, validate health
	if s.pluginMgr != nil {
		if modInfo, ok := s.pluginMgr.GetModule(pluginID); ok && modInfo.SocketPath != "" {
			healthValidation, err := plugins.ValidateHealthCheck(modInfo.SocketPath)
			if err != nil {
				resp.Health = &plugins.HealthValidation{
					ValidationResult: plugins.ValidationResult{
						Valid:  false,
						Errors: []string{"health check error: " + err.Error()},
					},
				}
			} else {
				resp.Health = healthValidation
				if !healthValidation.Valid {
					resp.Valid = false
				}
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": resp})
}

// handlePluginOperation routes requests under /v1/plugins/... to the appropriate handler
func (s *Server) handlePluginOperation(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v1/plugins")
	path = strings.TrimPrefix(path, "/")

	// Handle /v1/plugins or /v1/plugins/
	if path == "" {
		s.handlePluginList(w, r)
		return
	}

	// Handle /v1/plugins/create
	if path == "create" {
		s.handlePluginCreate(w, r)
		return
	}

	// Parse plugin ID and optional action
	parts := strings.SplitN(path, "/", 2)
	pluginID := parts[0]

	if len(parts) == 1 {
		// GET /v1/plugins/{id}
		s.handlePluginDetail(w, r, pluginID)
		return
	}

	action := parts[1]
	switch action {
	case "validate":
		// POST /v1/plugins/{id}/validate
		s.handlePluginValidate(w, r, pluginID)
	case "start":
		s.handlePluginLifecycle(w, r, pluginID, "start")
	case "stop":
		s.handlePluginLifecycle(w, r, pluginID, "stop")
	case "restart":
		s.handlePluginLifecycle(w, r, pluginID, "restart")
	default:
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "unknown action: " + action})
	}
}

// handlePluginLifecycle handles POST /v1/plugins/{id}/start|stop|restart.
func (s *Server) handlePluginLifecycle(w http.ResponseWriter, r *http.Request, pluginID, action string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	// PLUGIN-PATH-INJECTION-FIX (2026-08-10): validate pluginID.
	// Downstream: s.pluginMgr.{Start,Stop,Restart}(pluginID) likely uses
	// pluginID to construct plugin paths; validate at the handler surface
	// so untrusted URL segments never reach path-building code.
	if err := validatePluginID(pluginID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if s.pluginMgr == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "plugin manager not initialized"})
		return
	}

	var err error
	switch action {
	case "start":
		err = s.pluginMgr.StartModuleByID(pluginID)
	case "stop":
		err = s.pluginMgr.StopModuleByID(pluginID)
	case "restart":
		err = s.pluginMgr.RestartModuleByID(pluginID)
	}

	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": action + "ed", "plugin_id": pluginID})
}
