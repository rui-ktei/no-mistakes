package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadGlobal_ProposeCommandsDefaultsOn(t *testing.T) {
	cfg, err := LoadGlobal("/nonexistent/config.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.ProposeCommands {
		t.Fatalf("propose_commands default = %v, want true", cfg.ProposeCommands)
	}
}

func TestLoadGlobal_ProposeCommandsExplicitFalse(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("propose_commands: false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadGlobal(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ProposeCommands {
		t.Fatalf("propose_commands = %v, want false", cfg.ProposeCommands)
	}
}

func TestMerge_ProposeCommandsRepoOverridesGlobal(t *testing.T) {
	on := true
	off := false
	tests := []struct {
		name   string
		global bool
		repo   *bool
		want   bool
	}{
		{"global on, repo unset", true, nil, true},
		{"global on, repo off", true, &off, false},
		{"global off, repo on", false, &on, true},
		{"global off, repo unset", false, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &GlobalConfig{ProposeCommands: tt.global}
			r := &RepoConfig{ProposeCommands: tt.repo}
			cfg := Merge(g, r)
			if cfg.ProposeCommands != tt.want {
				t.Fatalf("ProposeCommands = %v, want %v", cfg.ProposeCommands, tt.want)
			}
		})
	}
}

func TestEffectiveRepoConfig_ProposeCommandsTrustedOnly(t *testing.T) {
	off := false
	on := true

	// A contributor's pushed branch setting propose_commands must be ignored.
	pushed := &RepoConfig{ProposeCommands: &on}
	effective := EffectiveRepoConfig(pushed, nil, false)
	if effective.ProposeCommands != nil {
		t.Fatalf("pushed propose_commands leaked into effective config: %v", *effective.ProposeCommands)
	}

	// The trusted default-branch value is the one honored.
	trusted := &RepoConfig{ProposeCommands: &off}
	effective = EffectiveRepoConfig(pushed, trusted, false)
	if effective.ProposeCommands == nil || *effective.ProposeCommands != false {
		t.Fatalf("trusted propose_commands not honored: %v", effective.ProposeCommands)
	}

	// Even under allow_repo_commands, propose_commands stays trusted-scoped.
	effective = EffectiveRepoConfig(pushed, trusted, true)
	if effective.ProposeCommands == nil || *effective.ProposeCommands != false {
		t.Fatalf("propose_commands not trusted-scoped under allow_repo_commands: %v", effective.ProposeCommands)
	}
}

func TestCommands_UnsetCommandFields(t *testing.T) {
	tests := []struct {
		name string
		cmds Commands
		want []CommandField
	}{
		{"all unset", Commands{}, []CommandField{CommandFieldTest, CommandFieldLint, CommandFieldFormat}},
		{"all set", Commands{Test: "t", Lint: "l", Format: "f"}, nil},
		{"lint set", Commands{Lint: "golangci-lint run"}, []CommandField{CommandFieldTest, CommandFieldFormat}},
		{"whitespace counts as unset", Commands{Test: "   "}, []CommandField{CommandFieldTest, CommandFieldLint, CommandFieldFormat}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cmds.UnsetCommandFields()
			if len(got) != len(tt.want) {
				t.Fatalf("UnsetCommandFields() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("UnsetCommandFields()[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}
