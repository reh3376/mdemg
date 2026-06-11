package alert

import "time"

// Severity levels for service alerts, aligned with ape.InsightSeverity values.
type Severity string

const (
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

// Alert represents a service health event that should be surfaced to the user.
type Alert struct {
	ID       string    `json:"id"`
	Time     time.Time `json:"time"`
	Service  string    `json:"service"`
	Severity Severity  `json:"severity"`
	Title    string    `json:"title"`
	Message  string    `json:"message"`
	SpaceID  string    `json:"space_id,omitempty"`
	Cleared  bool      `json:"cleared"`
}

// AlertFile is the JSON structure persisted to disk.
type AlertFile struct {
	UpdatedAt time.Time `json:"updated_at"`
	Alerts    []Alert   `json:"alerts"`
}
