package steps

import (
	"encoding/json"

	"github.com/kunchenguid/no-mistakes/internal/pipeline"
	"github.com/kunchenguid/no-mistakes/internal/types"
)

// Finding represents a single code review or lint finding.
type Finding = types.Finding

// Findings is the structured output from a pipeline step agent call.
type Findings = types.Findings

// findingsSchema is the JSON schema for structured findings output.
var findingsSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"findings": {
			"type": "array",
			"items": {
				"type": "object",
				"properties": {
					"id": {"type": "string"},
					"severity": {"type": "string", "enum": ["error", "warning", "info"]},
					"file": {"type": "string"},
					"line": {"type": "integer"},
					"description": {"type": "string"},
					"action": {"type": "string", "enum": ["no-op", "auto-fix", "ask-user"]}
				},
				"required": ["severity", "description", "action"]
			}
		},
		"summary": {"type": "string"},
		"tested": {
			"type": "array",
			"items": {"type": "string"}
		},
		"testing_summary": {
			"type": "string"
		},
		"canonical_command": {
			"type": "string",
			"description": "The single reproducible lint command you ran (e.g. golangci-lint run), or empty if none is confidently canonical"
		},
		"canonical_format_command": {
			"type": "string",
			"description": "The single reproducible formatter command you ran (e.g. gofmt -w .), distinct from the lint command, or empty if no distinct formatter ran"
		}
	},
	"required": ["findings", "summary"]
}`)

var testFindingsSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"findings": {
			"type": "array",
			"items": {
				"type": "object",
				"properties": {
					"id": {"type": "string"},
					"severity": {"type": "string", "enum": ["error", "warning", "info"]},
					"file": {"type": "string"},
					"line": {"type": "integer"},
					"description": {"type": "string"},
					"action": {"type": "string", "enum": ["no-op", "auto-fix", "ask-user"]}
				},
				"required": ["severity", "description", "action"]
			}
		},
		"summary": {"type": "string"},
		"tested": {
			"type": "array",
			"items": {"type": "string"}
		},
		"testing_summary": {
			"type": "string"
		},
		"canonical_command": {
			"type": "string",
			"description": "The single reproducible command that runs the project's tests (e.g. go test -race ./...), or empty if you used no single canonical command"
		},
		"artifacts": {
			"type": "array",
			"items": {
				"type": "object",
				"properties": {
					"kind": {"type": "string", "description": "artifact type such as screenshot, gif, image, video, log, command-output, or other"},
					"label": {"type": "string"},
					"path": {"type": "string", "description": "artifact file path, including absolute paths for temporary local evidence files when available"},
					"url": {"type": "string", "description": "artifact URL when available"},
					"content": {"type": "string", "description": "short log, command output, or textual artifact content to show inline"}
				},
				"required": ["label"]
			}
		}
	},
	"required": ["findings", "summary", "tested", "testing_summary", "artifacts"]
}`)

// reviewFindingsSchema is the JSON schema for structured review output with risk assessment.
// Field order matters for chain-of-thought: findings first, then risk level, then rationale.
var reviewFindingsSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"findings": {
			"type": "array",
			"items": {
				"type": "object",
				"properties": {
					"id": {"type": "string"},
					"severity": {"type": "string", "enum": ["error", "warning", "info"]},
					"file": {"type": "string"},
					"line": {"type": "integer"},
					"description": {"type": "string"},
					"action": {"type": "string", "enum": ["no-op", "auto-fix", "ask-user"]}
				},
				"required": ["severity", "description", "action"]
			}
		},
		"tested": {
			"type": "array",
			"items": {"type": "string"}
		},
		"testing_summary": {
			"type": "string"
		},
		"risk_level": {"type": "string", "enum": ["low", "medium", "high"]},
		"risk_rationale": {"type": "string"}
	},
	"required": ["findings", "risk_level", "risk_rationale"]
}`)

// AllSteps returns the fixed pipeline step sequence.
// When NM_DEMO=1, it returns mock steps for demo recordings.
func AllSteps() []pipeline.Step {
	if IsDemoMode() {
		return DemoSteps()
	}
	return []pipeline.Step{
		&IntentStep{},
		&RebaseStep{},
		&ReviewStep{},
		&TestStep{},
		&DocumentStep{},
		&LintStep{},
		&PushStep{},
		&PRStep{},
		&CIStep{},
	}
}
