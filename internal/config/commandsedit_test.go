package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, ".no-mistakes.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestProposeCommandsInFile_CreatesFileWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".no-mistakes.yaml")
	changed, err := ProposeCommandsInFile(path, map[CommandField]string{CommandFieldTest: "go test -race ./..."})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true")
	}
	got, err := LoadRepo(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Commands.Test != "go test -race ./..." {
		t.Fatalf("commands.test = %q, want %q", got.Commands.Test, "go test -race ./...")
	}
}

func TestProposeCommandsInFile_CreatesCommandsBlockPreservingContent(t *testing.T) {
	path := writeTemp(t, `# top comment
agent: claude

# ignore patterns for the repo
ignore_patterns:
  - "*.md"
`)
	changed, err := ProposeCommandsInFile(path, map[CommandField]string{
		CommandFieldTest:   "go test ./...",
		CommandFieldFormat: "gofmt -w .",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true")
	}
	data, _ := os.ReadFile(path)
	text := string(data)
	if !strings.Contains(text, "# top comment") {
		t.Errorf("top comment not preserved:\n%s", text)
	}
	if !strings.Contains(text, "# ignore patterns for the repo") {
		t.Errorf("inline comment not preserved:\n%s", text)
	}
	if !strings.Contains(text, "agent: claude") {
		t.Errorf("agent key not preserved:\n%s", text)
	}
	cfg, err := parseRepoConfig(data)
	if err != nil {
		t.Fatalf("re-parse failed: %v\n%s", err, text)
	}
	if cfg.Commands.Test != "go test ./..." {
		t.Errorf("commands.test = %q", cfg.Commands.Test)
	}
	if cfg.Commands.Format != "gofmt -w ." {
		t.Errorf("commands.format = %q", cfg.Commands.Format)
	}
	if len(cfg.IgnorePatterns) != 1 || cfg.IgnorePatterns[0] != "*.md" {
		t.Errorf("ignore_patterns = %v", cfg.IgnorePatterns)
	}
}

func TestProposeCommandsInFile_DoesNotOverwriteExistingValue(t *testing.T) {
	path := writeTemp(t, `commands:
  test: existing-test-cmd
`)
	changed, err := ProposeCommandsInFile(path, map[CommandField]string{
		CommandFieldTest: "new-test-cmd",
		CommandFieldLint: "new-lint-cmd",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true (lint added)")
	}
	cfg, err := LoadRepo(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Commands.Test != "existing-test-cmd" {
		t.Errorf("commands.test overwritten: %q", cfg.Commands.Test)
	}
	if cfg.Commands.Lint != "new-lint-cmd" {
		t.Errorf("commands.lint = %q, want new-lint-cmd", cfg.Commands.Lint)
	}
}

func TestProposeCommandsInFile_NoChangeWhenAllPresent(t *testing.T) {
	path := writeTemp(t, `commands:
  test: existing
`)
	before, _ := os.ReadFile(path)
	changed, err := ProposeCommandsInFile(path, map[CommandField]string{CommandFieldTest: "different"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changed {
		t.Fatal("expected changed=false")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Errorf("file mutated when no change expected:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestProposeCommandsInFile_IgnoresEmptyUpdates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".no-mistakes.yaml")
	changed, err := ProposeCommandsInFile(path, map[CommandField]string{CommandFieldTest: "   "})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changed {
		t.Fatal("expected changed=false for empty command")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("expected no file to be created")
	}
}
