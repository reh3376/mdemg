package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	pb "mdemg/api/modulepb"
)

// WorkflowEngine evaluates config-driven workflows after CRUD operations.
type WorkflowEngine struct {
	Workflows []Workflow `yaml:"workflows"`
}

// Workflow defines a single automation rule.
type Workflow struct {
	Name    string  `yaml:"name"`
	Trigger Trigger `yaml:"trigger"`
	Actions []Action `yaml:"actions"`
}

// Trigger defines when a workflow fires.
type Trigger struct {
	Event      string      `yaml:"event"`       // on-create, on-update, on-delete
	EntityType string      `yaml:"entity_type"` // issue, project, comment
	Conditions []Condition `yaml:"conditions"`
}

// Condition defines a match predicate.
type Condition struct {
	Field    string `yaml:"field"`
	Operator string `yaml:"operator"` // eq, neq, contains, changed_to, exists
	Value    string `yaml:"value"`
}

// Action defines what happens when a workflow fires.
type Action struct {
	Type   string            `yaml:"type"` // auto-assign, auto-label, add-comment, auto-transition, set-field
	Params map[string]string `yaml:"params"`
}

// NewWorkflowEngine creates a new empty workflow engine.
func NewWorkflowEngine() *WorkflowEngine {
	return &WorkflowEngine{}
}

// LoadFromFile loads workflow definitions from a YAML file.
func (w *WorkflowEngine) LoadFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read workflow file: %w", err)
	}
	return w.LoadFromBytes(data)
}

// LoadFromBytes loads workflow definitions from YAML bytes.
func (w *WorkflowEngine) LoadFromBytes(data []byte) error {
	var config struct {
		Workflows []Workflow `yaml:"workflows"`
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("parse workflow YAML: %w", err)
	}
	w.Workflows = config.Workflows
	slog.Info("workflow engine loaded", "workflow_count", len(w.Workflows))
	return nil
}

// HasChangedToConditions returns true if any workflow for the given entity type
// uses the changed_to operator (requires reading previous state before update).
func (w *WorkflowEngine) HasChangedToConditions(entityType string) bool {
	for _, wf := range w.Workflows {
		if wf.Trigger.EntityType != entityType {
			continue
		}
		for _, c := range wf.Trigger.Conditions {
			if c.Operator == "changed_to" {
				return true
			}
		}
	}
	return false
}

// EvaluateEvent checks all workflows against an event and executes matching actions.
// previousFields is only provided for on-update events (to support changed_to).
func (w *WorkflowEngine) EvaluateEvent(event, entityType string, fields, previousFields map[string]string, module *LinearModule) {
	for _, wf := range w.Workflows {
		if wf.Trigger.Event != event {
			continue
		}
		if wf.Trigger.EntityType != entityType {
			continue
		}

		if !w.evaluateConditions(wf.Trigger.Conditions, fields, previousFields) {
			continue
		}

		slog.Info("workflow engine triggered", "workflow", wf.Name, "event", event, "entity_type", entityType)

		for _, action := range wf.Actions {
			w.executeAction(action, fields, module)
			// Rate limiting between actions
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// evaluateConditions checks if all conditions match.
func (w *WorkflowEngine) evaluateConditions(conditions []Condition, fields, previousFields map[string]string) bool {
	for _, c := range conditions {
		if !MatchCondition(c, fields, previousFields) {
			return false
		}
	}
	return true
}

// MatchCondition evaluates a single condition against fields.
func MatchCondition(c Condition, fields, previousFields map[string]string) bool {
	fieldValue := fields[c.Field]

	switch c.Operator {
	case "eq":
		return fieldValue == c.Value
	case "neq":
		return fieldValue != c.Value
	case "contains":
		return strings.Contains(fieldValue, c.Value)
	case "changed_to":
		if previousFields == nil {
			return false
		}
		prevValue := previousFields[c.Field]
		return fieldValue == c.Value && prevValue != c.Value
	case "exists":
		_, exists := fields[c.Field]
		return exists
	default:
		slog.Warn("workflow engine unknown operator", "operator", c.Operator)
		return false
	}
}

// executeAction runs a single workflow action.
func (w *WorkflowEngine) executeAction(action Action, fields map[string]string, module *LinearModule) {
	entityID := fields["id"]
	entityType := "issue" // default — actions typically target issues

	switch action.Type {
	case "add-comment":
		body := interpolateTemplate(action.Params["body"], fields)
		issueID := action.Params["issue_id"]
		if issueID == "" {
			issueID = entityID
		}
		commentFields := map[string]string{
			"issue_id": issueID,
			"body":     body,
		}
		query, err := buildCommentCreateMutation(commentFields)
		if err != nil {
			slog.Error("workflow engine add-comment build error", "error", err)
			return
		}
		if _, err := module.executeGraphQL(context.Background(), query); err != nil {
			slog.Error("workflow engine add-comment error", "error", err)
		}

	case "auto-assign":
		assigneeID := action.Params["assignee_id"]
		if assigneeID == "" {
			slog.Warn("workflow engine auto-assign requires assignee_id param")
			return
		}
		updateFields := map[string]string{"assignee_id": assigneeID}
		query, err := buildIssueUpdateMutation(entityID, updateFields)
		if err != nil {
			slog.Error("workflow engine auto-assign build error", "error", err)
			return
		}
		if _, err := module.executeGraphQL(context.Background(), query); err != nil {
			slog.Error("workflow engine auto-assign error", "error", err)
		}

	case "auto-label":
		labelIDs := action.Params["label_ids"]
		if labelIDs == "" {
			slog.Warn("workflow engine auto-label requires label_ids param")
			return
		}
		updateFields := map[string]string{"label_ids": labelIDs}
		query, err := buildIssueUpdateMutation(entityID, updateFields)
		if err != nil {
			slog.Error("workflow engine auto-label build error", "error", err)
			return
		}
		if _, err := module.executeGraphQL(context.Background(), query); err != nil {
			slog.Error("workflow engine auto-label error", "error", err)
		}

	case "auto-transition":
		stateID := action.Params["state_id"]
		if stateID == "" {
			slog.Warn("workflow engine auto-transition requires state_id param")
			return
		}
		updateFields := map[string]string{"state_id": stateID}
		query, err := buildIssueUpdateMutation(entityID, updateFields)
		if err != nil {
			slog.Error("workflow engine auto-transition build error", "error", err)
			return
		}
		if _, err := module.executeGraphQL(context.Background(), query); err != nil {
			slog.Error("workflow engine auto-transition error", "error", err)
		}

	case "set-field":
		fieldName := action.Params["field"]
		fieldValue := interpolateTemplate(action.Params["value"], fields)
		if fieldName == "" {
			slog.Warn("workflow engine set-field requires field param")
			return
		}
		updateFields := map[string]string{fieldName: fieldValue}

		var query map[string]interface{}
		var err error
		if entityType == "project" {
			query, err = buildProjectUpdateMutation(entityID, updateFields)
		} else {
			query, err = buildIssueUpdateMutation(entityID, updateFields)
		}
		if err != nil {
			slog.Error("workflow engine set-field build error", "error", err)
			return
		}
		if _, err := module.executeGraphQL(context.Background(), query); err != nil {
			slog.Error("workflow engine set-field error", "error", err)
		}

	default:
		slog.Warn("workflow engine unknown action type", "action_type", action.Type)
	}
}

// interpolateTemplate replaces {{field}} placeholders with values from fields.
func interpolateTemplate(template string, fields map[string]string) string {
	result := template
	for key, value := range fields {
		result = strings.ReplaceAll(result, "{{"+key+"}}", value)
	}
	return result
}

// Ensure LinearModule implements CRUDModuleServer at compile time.
var _ pb.CRUDModuleServer = (*LinearModule)(nil)
