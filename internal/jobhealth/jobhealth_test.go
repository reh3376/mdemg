package jobhealth

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mdemg/internal/alert"
	"mdemg/internal/tsdb"
)

// Report uses a real alert.Dispatcher with a file backend (temp dir) so we can
// assert the alert-on-failure policy end-to-end without a TSDB pool (nil pool
// → record skipped, alert path still exercised). The file backend delivers
// asynchronously, so reads poll briefly.

func newFileDispatcher(t *testing.T) (*alert.Dispatcher, string) {
	t.Helper()
	alertFile := filepath.Join(t.TempDir(), "alerts.json")
	disp := alert.NewDispatcher(alert.Config{
		Enabled: true, CooldownSec: 0, AlertFilePath: alertFile, MaxAlerts: 50,
	})
	return disp, alertFile
}

func readAlerts(path string) []map[string]any {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var wrapper struct {
		Alerts []map[string]any `json:"alerts"`
	}
	if json.Unmarshal(b, &wrapper) == nil && wrapper.Alerts != nil {
		return wrapper.Alerts
	}
	var bare []map[string]any
	_ = json.Unmarshal(b, &bare)
	return bare
}

func waitForAlerts(path string) []map[string]any {
	for range 100 { // up to ~1s
		if a := readAlerts(path); len(a) > 0 {
			return a
		}
		time.Sleep(10 * time.Millisecond)
	}
	return readAlerts(path)
}

func TestReport_FailureFiresAlert(t *testing.T) {
	disp, alertFile := newFileDispatcher(t)
	Report(context.Background(), nil, disp, tsdb.JobEventRow{
		JobName: "tsdb-backup", SpaceID: "mdemg-dev", Success: false,
		ErrorMessage: "docker not found",
	})
	alerts := waitForAlerts(alertFile)
	if len(alerts) == 0 {
		t.Fatal("failure should have produced an alert")
	}
	found := false
	for _, a := range alerts {
		if strings.Contains(toStr(a["title"]), "tsdb-backup") || strings.Contains(toStr(a["message"]), "docker not found") {
			found = true
		}
	}
	if !found {
		t.Errorf("alert should reference the failed job; got %+v", alerts)
	}
}

func TestReport_SuccessNoAlert(t *testing.T) {
	disp, alertFile := newFileDispatcher(t)
	Report(context.Background(), nil, disp, tsdb.JobEventRow{
		JobName: "tsdb-backup", SpaceID: "mdemg-dev", Success: true,
	})
	time.Sleep(150 * time.Millisecond) // give any (erroneous) async write a chance
	if alerts := readAlerts(alertFile); len(alerts) != 0 {
		t.Errorf("success must not produce an alert, got %+v", alerts)
	}
}

func TestReport_NilDispatcherNoPanic(t *testing.T) {
	Report(context.Background(), nil, nil, tsdb.JobEventRow{JobName: "x", Success: false})
}

func toStr(v any) string {
	s, _ := v.(string)
	return s
}
