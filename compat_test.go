package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestBashSuiteFromTheDotfilesStillPasses(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the bash suite needs a POSIX shell and tmux")
	}
	for _, tool := range []string{"bash", "tmux", "rg"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is not installed", tool)
		}
	}
	bin := filepath.Join(t.TempDir(), "claude-sessions")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	cmd := exec.Command("bash", filepath.Join("test", "compat", "claude-sessions.test.sh"))
	cmd.Env = append(os.Environ(), "CLAUDE_SESSIONS_BIN="+bin)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	t.Logf("\n%s", out)
}
