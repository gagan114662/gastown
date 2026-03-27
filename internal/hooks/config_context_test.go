package hooks

import (
	"strings"
	"testing"
)

func TestDefaultBaseUserPromptSubmitIncludesEntropyGuard(t *testing.T) {
	base := DefaultBase()
	if len(base.UserPromptSubmit) == 0 || len(base.UserPromptSubmit[0].Hooks) == 0 {
		t.Fatal("expected default UserPromptSubmit hook")
	}
	command := base.UserPromptSubmit[0].Hooks[0].Command
	if !strings.Contains(command, "gt entropy --guard") {
		t.Fatalf("expected entropy guard in hook command, got %q", command)
	}
	if !strings.Contains(command, "gt mail check --inject") {
		t.Fatalf("expected mail injection to remain in hook command, got %q", command)
	}
}
