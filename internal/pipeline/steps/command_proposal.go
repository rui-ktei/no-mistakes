package steps

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/kunchenguid/no-mistakes/internal/config"
	"github.com/kunchenguid/no-mistakes/internal/git"
	"github.com/kunchenguid/no-mistakes/internal/pipeline"
)

// commandProposalFields is the stable order in which command fields are
// considered and rendered.
var commandProposalFields = []config.CommandField{
	config.CommandFieldTest,
	config.CommandFieldLint,
	config.CommandFieldFormat,
}

// proposeDiscoveredCommands writes the canonical test/lint/format commands the
// discovery agents reported this run into the branch's .no-mistakes.yaml and
// commits them as a dedicated commit, so a human merge to the default branch is
// what promotes them to trusted, executed config.
//
// It is a no-op when the feature is disabled, when nothing was discovered, and
// for any field that is already set in the effective (trusted) config or
// already present in the branch working-tree file. It NEVER touches the trusted
// default branch, never enables allow_repo_commands, and stages only
// .no-mistakes.yaml so it cannot bundle unrelated working-tree changes. The
// resulting commit rides the branch through PushStep's normal (lease-guarded)
// push - this function never pushes.
func proposeDiscoveredCommands(sctx *pipeline.StepContext) error {
	if sctx.Config == nil || !sctx.Config.ProposeCommands {
		return nil
	}
	discovered := sctx.Shared.DiscoveredCommands()
	if len(discovered) == 0 {
		return nil
	}

	updates, err := proposableCommands(sctx, discovered)
	if err != nil {
		return err
	}
	if len(updates) == 0 {
		return nil
	}

	path := filepath.Join(sctx.WorkDir, ".no-mistakes.yaml")
	changed, err := config.ProposeCommandsInFile(path, updates)
	if err != nil {
		return fmt.Errorf("write command proposal: %w", err)
	}
	if !changed {
		return nil
	}
	return commitProposedCommands(sctx, updates)
}

// proposableCommands returns the discovered commands eligible to be proposed:
// unset in the effective config AND not already present in the branch
// working-tree .no-mistakes.yaml (the latter keeps the proposer idempotent
// across reruns of the same branch, since commands are read from the default
// branch, not the branch file).
func proposableCommands(sctx *pipeline.StepContext, discovered map[config.CommandField]string) (map[config.CommandField]string, error) {
	branchCfg, err := config.LoadRepo(sctx.WorkDir)
	if err != nil {
		return nil, fmt.Errorf("read branch config for command proposal: %w", err)
	}
	effective := sctx.Config.Commands
	updates := map[config.CommandField]string{}
	for _, field := range commandProposalFields {
		cmd := strings.TrimSpace(discovered[field])
		if cmd == "" {
			continue
		}
		if strings.TrimSpace(effective.Get(field)) != "" {
			continue
		}
		if strings.TrimSpace(branchCfg.Commands.Get(field)) != "" {
			continue
		}
		updates[field] = cmd
	}
	return updates, nil
}

func commitProposedCommands(sctx *pipeline.StepContext, updates map[config.CommandField]string) error {
	ctx := sctx.Ctx
	if _, err := git.Run(ctx, sctx.WorkDir, "add", "--", ".no-mistakes.yaml"); err != nil {
		return fmt.Errorf("stage command proposal: %w", err)
	}
	staged, _ := git.Run(ctx, sctx.WorkDir, "diff", "--cached", "--name-only")
	if strings.TrimSpace(staged) == "" {
		return nil
	}

	var fields []string
	for _, field := range commandProposalFields {
		if _, ok := updates[field]; ok {
			fields = append(fields, string(field))
		}
	}
	noun := "command"
	if len(fields) != 1 {
		noun = "commands"
	}
	message := fixedFixCommitMessage(sctx, fmt.Sprintf("propose discovered %s %s", strings.Join(fields, ", "), noun))
	if _, err := git.Run(ctx, sctx.WorkDir, "commit", "-m", message); err != nil {
		return fmt.Errorf("commit command proposal: %w", err)
	}
	headSHA, err := git.HeadSHA(ctx, sctx.WorkDir)
	if err != nil {
		return fmt.Errorf("resolve head after command proposal commit: %w", err)
	}
	ref := normalizedBranchRef(sctx.Run.Branch)
	if _, err := git.Run(ctx, sctx.WorkDir, "update-ref", ref, headSHA); err != nil {
		return fmt.Errorf("update local branch ref: %w", err)
	}
	sctx.Run.HeadSHA = headSHA
	if err := sctx.DB.UpdateRunHeadSHA(sctx.Run.ID, headSHA); err != nil {
		return err
	}
	sctx.Log(fmt.Sprintf("proposed discovered %s in .no-mistakes.yaml: %s", noun, strings.Join(fields, ", ")))
	return nil
}

// pendingProposedCommands returns the commands present in the branch
// working-tree .no-mistakes.yaml that are absent from the effective (trusted)
// config - i.e. proposals on this branch not yet merged to the default branch.
// The PR step uses this to note pending proposals; it is derived from the
// branch file (not this run's scratch) so it stays accurate across reruns.
func pendingProposedCommands(sctx *pipeline.StepContext) map[config.CommandField]string {
	if sctx == nil || sctx.Config == nil || strings.TrimSpace(sctx.WorkDir) == "" {
		return nil
	}
	branchCfg, err := config.LoadRepo(sctx.WorkDir)
	if err != nil {
		return nil
	}
	effective := sctx.Config.Commands
	pending := map[config.CommandField]string{}
	for _, field := range commandProposalFields {
		branchValue := strings.TrimSpace(branchCfg.Commands.Get(field))
		if branchValue == "" {
			continue
		}
		if strings.TrimSpace(effective.Get(field)) != "" {
			continue
		}
		pending[field] = branchValue
	}
	if len(pending) == 0 {
		return nil
	}
	return pending
}
