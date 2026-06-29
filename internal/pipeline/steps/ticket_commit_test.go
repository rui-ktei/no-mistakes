package steps

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kunchenguid/no-mistakes/internal/config"
	"github.com/kunchenguid/no-mistakes/internal/db"
	"github.com/kunchenguid/no-mistakes/internal/pipeline"
	"github.com/kunchenguid/no-mistakes/internal/types"
)

func stepContextForBranch(branch, pattern string) *pipeline.StepContext {
	return &pipeline.StepContext{
		Run:    &db.Run{Branch: branch},
		Config: &config.Config{TicketPrefixPattern: pattern},
	}
}

// stepContextWithCommits builds a real git repo whose feature branch carries
// the given commit subjects (oldest first) off main, and returns a
// StepContext wired to it so the author-commit scan can run for real.
func stepContextWithCommits(t *testing.T, branch, pattern string, subjects []string) *pipeline.StepContext {
	t.Helper()
	dir := t.TempDir()
	gitCmd(t, dir, "init")
	gitCmd(t, dir, "config", "user.name", "test")
	gitCmd(t, dir, "config", "user.email", "test@test.com")
	gitCmd(t, dir, "config", "commit.gpgsign", "false")
	gitCmd(t, dir, "checkout", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "init.txt"), []byte("init"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCmd(t, dir, "add", "-A")
	gitCmd(t, dir, "commit", "-m", "initial")
	baseSHA := gitCmd(t, dir, "rev-parse", "HEAD")

	gitCmd(t, dir, "checkout", "-b", "feature")
	for _, subject := range subjects {
		if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte(subject), 0o644); err != nil {
			t.Fatal(err)
		}
		gitCmd(t, dir, "add", "-A")
		gitCmd(t, dir, "commit", "-m", subject)
	}
	headSHA := gitCmd(t, dir, "rev-parse", "HEAD")

	return &pipeline.StepContext{
		Ctx:     context.Background(),
		Run:     &db.Run{Branch: branch, BaseSHA: baseSHA, HeadSHA: headSHA},
		Repo:    &db.Repo{WorkingPath: dir, DefaultBranch: "main"},
		WorkDir: dir,
		Config:  &config.Config{TicketPrefixPattern: pattern},
	}
}

func TestResolveTicket(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		branch   string
		pattern  string
		prTitle  string
		subjects []string
		want     string
	}{
		{"branch only", "WEB-12345-fix-thing", `WEB-\d+`, "", []string{"add thing"}, "WEB-12345"},
		{"commit only", "fix/jira-empty-release", `WEB-\d+`, "", []string{"WEB-12345: fix empty releases"}, "WEB-12345"},
		{"pr title only", "fix/jira-empty-release", `WEB-\d+`, "WEB-12345: fix empty releases", []string{"fix empty releases"}, "WEB-12345"},
		{"branch wins over pr title and commit", "WEB-100-x", `WEB-\d+`, "WEB-200: t", []string{"WEB-300: c"}, "WEB-100"},
		{"pr title wins over commit", "fix/x", `WEB-\d+`, "WEB-200: t", []string{"WEB-300: c"}, "WEB-200"},
		{"no source carries ticket", "fix/x", `WEB-\d+`, "", []string{"chore: y"}, ""},
		{"pattern empty disables", "WEB-100-x", "", "WEB-200: t", []string{"WEB-300: c"}, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sctx := stepContextWithCommits(t, tc.branch, tc.pattern, tc.subjects)
			if got := resolveTicket(sctx, tc.prTitle); got != tc.want {
				t.Fatalf("resolveTicket(branch=%q, prTitle=%q, subjects=%v) = %q, want %q", tc.branch, tc.prTitle, tc.subjects, got, tc.want)
			}
		})
	}
}

func TestFirstAuthorCommitTicket(t *testing.T) {
	t.Parallel()

	t.Run("oldest-first first match", func(t *testing.T) {
		t.Parallel()
		sctx := stepContextWithCommits(t, "fix/x", `WEB-\d+`, []string{
			"chore: no ticket here",
			"WEB-100: first ticketed",
			"WEB-200: second ticketed",
		})
		if got := firstAuthorCommitTicket(sctx, `WEB-\d+`); got != "WEB-100" {
			t.Fatalf("got %q, want WEB-100", got)
		}
	})

	t.Run("skips gate-authored subjects", func(t *testing.T) {
		t.Parallel()
		sctx := stepContextWithCommits(t, "fix/x", `WEB-\d+`, []string{
			"no-mistakes(lint): WEB-999 cleanup",
			"WEB-50: no-mistakes: apply CI fixes",
			"WEB-100: real author commit",
		})
		if got := firstAuthorCommitTicket(sctx, `WEB-\d+`); got != "WEB-100" {
			t.Fatalf("got %q, want WEB-100 (gate-authored subjects must be skipped)", got)
		}
	})
}

func TestDeterministicFixCommitMessage(t *testing.T) {
	t.Parallel()

	t.Run("ticket leads subject with step trace", func(t *testing.T) {
		t.Parallel()
		got := deterministicFixCommitMessage(stepContextForBranch("WEB-12345-readme", `WEB-\d+`), types.StepDocument, "drop stale key")
		want := "WEB-12345: no-mistakes(document): drop stale key"
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("no ticket keeps conventional subject", func(t *testing.T) {
		t.Parallel()
		got := deterministicFixCommitMessage(stepContextForBranch("docs/readme-refresh", `WEB-\d+`), types.StepDocument, "drop stale key")
		want := "no-mistakes(document): drop stale key"
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})
}

func TestFixedFixCommitMessage(t *testing.T) {
	t.Parallel()

	t.Run("ticket leads subject", func(t *testing.T) {
		t.Parallel()
		got := fixedFixCommitMessage(stepContextForBranch("WEB-7-x", `WEB-\d+`), "apply CI fixes")
		want := "WEB-7: no-mistakes: apply CI fixes"
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("no ticket keeps default subject", func(t *testing.T) {
		t.Parallel()
		got := fixedFixCommitMessage(stepContextForBranch("feature/x", `WEB-\d+`), "apply CI fixes")
		want := "no-mistakes: apply CI fixes"
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})
}
