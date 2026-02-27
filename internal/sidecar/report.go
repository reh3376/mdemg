package sidecar

import (
	"encoding/json"
	"fmt"
	"os"
)

// NewReportEnvelope creates a ReportEnvelope with common fields populated.
// Slices are initialized to empty (not nil) to produce [] in JSON.
func NewReportEnvelope(command string, stateBefore, stateAfter State, exitCode int) ReportEnvelope {
	result := "success"
	if exitCode != ExitSuccess {
		result = "error"
	}
	return ReportEnvelope{
		SchemaVersion: ReportSchemaVersion,
		Command:       command,
		Timestamp:     NowUTC(),
		Result:        result,
		ExitCode:      exitCode,
		StateBefore:   stateBefore,
		StateAfter:    stateAfter,
		Changes:       []ReportChange{},
		Issues:        []ReportIssue{},
		NextActions:   []string{},
	}
}

// PrintJSON marshals v as indented JSON and writes it to stdout.
func PrintJSON(v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}
	_, err = fmt.Fprintln(os.Stdout, string(data))
	return err
}
