package api

import (
	"encoding/json"
	"net/http"
	"time"
)

// featureEntry describes a service/feature with its runtime status.
type featureEntry struct {
	Name         string `json:"name"`
	Status       string `json:"status"`               // healthy, stopped, unavailable
	State        string `json:"state"`                // running, stopped, n/a
	Controllable bool   `json:"controllable"`         // true if Start/Stop available
	ConfigKey    string `json:"config_key,omitempty"` // config key that enables it
}

// handleFeatures serves GET /v1/admin/features.
// Returns all known services with their current status.
func (s *Server) handleFeatures(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	features := []featureEntry{
		s.featureAPEScheduler(),
		s.featureRSICWatchdog(),
		s.featureRSICStore(),
		s.featureBackupScheduler(),
		s.featureTSDBBackupScheduler(),
		s.featureMetricsRecorder(),
		s.featureFileWatcher(),
		s.featurePluginManager(),
		// Config-only services (no Start/Stop)
		s.featureConfigOnly("jiminy", s.jiminySvc != nil, "jiminy.enabled"),
		s.featureConfigOnly("learning", s.learner != nil, ""),
		s.featureConfigOnly("retrieval", s.retriever != nil, ""),
		s.featureConfigOnly("embeddings", s.embedder != nil, ""),
		s.featureConfigOnly("anomaly_detection", s.anomalyDetector != nil, ""),
		s.featureConfigOnly("conversation", s.conversationSvc != nil, ""),
		s.featureConfigOnly("hidden_layer", s.hiddenLayer != nil, ""),
		s.featureConfigOnly("scraper", s.scraperSvc != nil, ""),
	}

	writeJSON(w, http.StatusOK, map[string]any{"features": features})
}

func (s *Server) featureAPEScheduler() featureEntry {
	e := featureEntry{Name: "ape_scheduler", Controllable: true}
	if s.apeScheduler == nil {
		e.Status = "unavailable"
		e.State = "n/a"
	} else {
		e.Status = "healthy"
		e.State = "running"
	}
	return e
}

func (s *Server) featureRSICWatchdog() featureEntry {
	e := featureEntry{Name: "rsic_watchdog", Controllable: true}
	if s.rsicWatchdog == nil {
		e.Status = "unavailable"
		e.State = "n/a"
	} else if s.rsicWatchdog.Running() {
		e.Status = "healthy"
		e.State = "running"
	} else {
		e.Status = "stopped"
		e.State = "stopped"
	}
	return e
}

func (s *Server) featureRSICStore() featureEntry {
	e := featureEntry{Name: "rsic_store", Controllable: false}
	if s.rsicStore == nil {
		e.Status = "unavailable"
		e.State = "n/a"
	} else {
		e.Status = "healthy"
		e.State = "running"
	}
	return e
}

func (s *Server) featureBackupScheduler() featureEntry {
	e := featureEntry{Name: "backup_scheduler", Controllable: true, ConfigKey: "backup.enabled"}
	if s.backupScheduler == nil {
		e.Status = "unavailable"
		e.State = "n/a"
	} else {
		e.Status = "healthy"
		e.State = "running"
	}
	return e
}

func (s *Server) featureTSDBBackupScheduler() featureEntry {
	e := featureEntry{Name: "tsdb_backup_scheduler", Controllable: true}
	if s.tsdbBackupScheduler == nil {
		e.Status = "unavailable"
		e.State = "n/a"
	} else {
		e.Status = "healthy"
		e.State = "running"
	}
	return e
}

func (s *Server) featureMetricsRecorder() featureEntry {
	e := featureEntry{Name: "metrics_recorder", Controllable: true}
	if s.metricsRecorder == nil {
		e.Status = "unavailable"
		e.State = "n/a"
	} else {
		e.Status = "healthy"
		e.State = "running"
	}
	return e
}

func (s *Server) featureFileWatcher() featureEntry {
	e := featureEntry{Name: "file_watcher", Controllable: false}
	if s.fileWatcherMgr == nil {
		e.Status = "unavailable"
		e.State = "n/a"
	} else {
		e.Status = "healthy"
		e.State = "running"
	}
	return e
}

func (s *Server) featurePluginManager() featureEntry {
	e := featureEntry{Name: "plugin_manager", Controllable: true, ConfigKey: "plugins.enabled"}
	if s.pluginMgr == nil {
		e.Status = "unavailable"
		e.State = "n/a"
	} else {
		e.Status = "healthy"
		e.State = "running"
	}
	return e
}

func (s *Server) featureConfigOnly(name string, initialized bool, configKey string) featureEntry {
	e := featureEntry{Name: name, Controllable: false, ConfigKey: configKey}
	if initialized {
		e.Status = "healthy"
		e.State = "running"
	} else {
		e.Status = "unavailable"
		e.State = "n/a"
	}
	return e
}

// handleFeatureLifecycle handles POST /v1/admin/features/start|stop|restart.
func (s *Server) handleFeatureLifecycle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	if body.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	action := ""
	path := r.URL.Path
	if len(path) > 0 {
		parts := []byte(path)
		for i := len(parts) - 1; i >= 0; i-- {
			if parts[i] == '/' {
				action = string(parts[i+1:])
				break
			}
		}
	}

	var err error
	switch body.Name {
	case "ape_scheduler":
		if s.apeScheduler == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "ape_scheduler not initialized"})
			return
		}
		switch action {
		case "start":
			err = s.apeScheduler.Start()
		case "stop":
			s.apeScheduler.Stop()
		case "restart":
			s.apeScheduler.Stop()
			time.Sleep(200 * time.Millisecond)
			err = s.apeScheduler.Start()
		}

	case "rsic_watchdog":
		if s.rsicWatchdog == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "rsic_watchdog not initialized"})
			return
		}
		switch action {
		case "start":
			s.rsicWatchdog.Start()
		case "stop":
			s.rsicWatchdog.Stop()
		case "restart":
			s.rsicWatchdog.Restart()
		}

	case "backup_scheduler":
		if s.backupScheduler == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "backup_scheduler not initialized"})
			return
		}
		switch action {
		case "start":
			s.backupScheduler.Start()
		case "stop":
			s.backupScheduler.Stop()
		case "restart":
			s.backupScheduler.Stop()
			time.Sleep(200 * time.Millisecond)
			s.backupScheduler.Start()
		}

	case "tsdb_backup_scheduler":
		if s.tsdbBackupScheduler == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "tsdb_backup_scheduler not initialized"})
			return
		}
		switch action {
		case "start":
			s.tsdbBackupScheduler.Start()
		case "stop":
			s.tsdbBackupScheduler.Stop()
		case "restart":
			s.tsdbBackupScheduler.Stop()
			time.Sleep(200 * time.Millisecond)
			s.tsdbBackupScheduler.Start()
		}

	case "metrics_recorder":
		if s.metricsRecorder == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "metrics_recorder not initialized"})
			return
		}
		switch action {
		case "start":
			s.metricsRecorder.Start(30 * time.Second)
		case "stop":
			s.metricsRecorder.Stop()
		case "restart":
			s.metricsRecorder.Stop()
			time.Sleep(200 * time.Millisecond)
			s.metricsRecorder.Start(30 * time.Second)
		}

	case "plugin_manager":
		if s.pluginMgr == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "plugin_manager not initialized"})
			return
		}
		switch action {
		case "start":
			err = s.pluginMgr.Start()
		case "stop":
			err = s.pluginMgr.Stop()
		case "restart":
			_ = s.pluginMgr.Stop()
			time.Sleep(200 * time.Millisecond)
			err = s.pluginMgr.Start()
		}

	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown or non-controllable feature: " + body.Name})
		return
	}

	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": action + "ed", "name": body.Name})
}
