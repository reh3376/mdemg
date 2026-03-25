package plugins

import (
	"context"
	"log/slog"
	"time"

	pb "mdemg/api/modulepb"
)

// EventDispatcher routes events to non-APE modules that declared
// EventSubscriptions in their manifest. APE modules receive events
// through the APE scheduler; this handles INGESTION and CRUD modules.
type EventDispatcher struct {
	pluginMgr *Manager
}

// NewEventDispatcher creates a new EventDispatcher backed by the plugin manager.
func NewEventDispatcher(mgr *Manager) *EventDispatcher {
	if mgr == nil {
		return &EventDispatcher{}
	}
	return &EventDispatcher{pluginMgr: mgr}
}

// DispatchEvent sends an event to all non-APE modules whose manifest
// declares a matching EventSubscription. For INGESTION modules it calls
// Parse with the event context; for other module types it logs the dispatch.
func (d *EventDispatcher) DispatchEvent(event string, ctx map[string]string) {
	if d.pluginMgr == nil {
		return
	}

	// Check INGESTION modules
	for _, mod := range d.pluginMgr.GetIngestionModules() {
		if !hasSubscription(mod.Manifest.Capabilities.EventSubscriptions, event) {
			continue
		}
		go d.dispatchToIngestion(mod, event, ctx)
	}

	// Check CRUD modules
	for _, mod := range d.pluginMgr.GetCRUDModules() {
		if !hasSubscription(mod.Manifest.Capabilities.EventSubscriptions, event) {
			continue
		}
		slog.Info("event-dispatch: CRUD module subscribed but no OnEvent RPC yet, skipping", "module", mod.Manifest.ID, "event", event)
	}
}

// dispatchToIngestion calls Parse on an INGESTION module with event metadata.
func (d *EventDispatcher) dispatchToIngestion(mod *ModuleInfo, event string, eventCtx map[string]string) {
	if mod.IngestionClient == nil {
		slog.Warn("event-dispatch: ingestion module has no client, skipping", "module", mod.Manifest.ID)
		return
	}

	callCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	metadata := make(map[string]string, len(eventCtx)+1)
	for k, v := range eventCtx {
		metadata[k] = v
	}
	metadata["event"] = event

	req := &pb.ParseRequest{
		SourceUri:   "event://" + event,
		ContentType: "application/x-mdemg-event",
		Metadata:    metadata,
	}

	resp, err := mod.IngestionClient.Parse(callCtx, req)
	if err != nil {
		slog.Error("event-dispatch: Parse failed", "module", mod.Manifest.ID, "event", event, "error", err)
		return
	}

	slog.Info("event-dispatch: module handled event", "module", mod.Manifest.ID, "event", event, "observations", len(resp.GetObservations()))
}

// hasSubscription checks if the subscriptions list contains the given event.
func hasSubscription(subscriptions []string, event string) bool {
	for _, s := range subscriptions {
		if s == event || s == "*" {
			return true
		}
	}
	return false
}
