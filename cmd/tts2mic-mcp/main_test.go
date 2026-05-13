package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestDetachedSpeakArgsWithoutDelay(t *testing.T) {
	args := detachedSpeakArgs("hello world", 0)

	wanted := []string{"speak", "--target", macOSBackendTarget, "--text", "hello world"}
	if len(args) != len(wanted) {
		t.Fatalf("detachedSpeakArgs() len = %d, want %d (%v)", len(args), len(wanted), args)
	}

	for i := range wanted {
		if args[i] != wanted[i] {
			t.Fatalf("detachedSpeakArgs()[%d] = %q, want %q", i, args[i], wanted[i])
		}
	}
}

func TestDetachedSpeakArgsWithDelay(t *testing.T) {
	args := detachedSpeakArgs("hello world", 1500*time.Millisecond)

	wanted := []string{"speak", "--target", macOSBackendTarget, "--text", "hello world", "--delay", "1.5s"}
	if len(args) != len(wanted) {
		t.Fatalf("detachedSpeakArgs() len = %d, want %d (%v)", len(args), len(wanted), args)
	}

	for i := range wanted {
		if args[i] != wanted[i] {
			t.Fatalf("detachedSpeakArgs()[%d] = %q, want %q", i, args[i], wanted[i])
		}
	}
}

func TestCurrentWorkingDirectory(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}

	got := currentWorkingDirectory()
	if got != wd {
		t.Fatalf("currentWorkingDirectory() = %q, want %q", got, wd)
	}
}

func TestNewDetachedSpeakCommandSetsDetachedProcessState(t *testing.T) {
	cmd, err := newDetachedSpeakCommand("/bin/echo", "hello world", 2*time.Second)
	if err != nil {
		t.Fatalf("newDetachedSpeakCommand() error = %v", err)
	}

	t.Cleanup(func() {
		if closer, ok := cmd.Stdin.(*os.File); ok {
			_ = closer.Close()
		}
	})

	wantedArgs := []string{"/bin/echo", "speak", "--target", macOSBackendTarget, "--text", "hello world", "--delay", "2s"}
	if len(cmd.Args) != len(wantedArgs) {
		t.Fatalf("cmd.Args len = %d, want %d (%v)", len(cmd.Args), len(wantedArgs), cmd.Args)
	}
	for i := range wantedArgs {
		if cmd.Args[i] != wantedArgs[i] {
			t.Fatalf("cmd.Args[%d] = %q, want %q", i, cmd.Args[i], wantedArgs[i])
		}
	}

	if cmd.Dir != currentWorkingDirectory() {
		t.Fatalf("cmd.Dir = %q, want %q", cmd.Dir, currentWorkingDirectory())
	}
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setpgid {
		t.Fatal("cmd.SysProcAttr.Setpgid = false, want true")
	}
	if cmd.Stdin == nil || cmd.Stdout == nil || cmd.Stderr == nil {
		t.Fatal("detached command stdio should all be redirected")
	}
	if len(cmd.Env) == 0 {
		t.Fatal("detached command environment should inherit current environment")
	}
}

func TestLoadDotEnvFileParsesSupportedLines(t *testing.T) {
	tempDir := t.TempDir()
	dotenvPath := filepath.Join(tempDir, ".env")
	content := "# comment\nFOO=bar\nexport HELLO=world\nQUOTED=\"hello world\"\nSINGLE='value'\nEMPTY=\n"
	if err := os.WriteFile(dotenvPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(.env) error = %v", err)
	}

	values, err := loadDotEnvFile(dotenvPath)
	if err != nil {
		t.Fatalf("loadDotEnvFile() error = %v", err)
	}

	wanted := map[string]string{
		"FOO":    "bar",
		"HELLO":  "world",
		"QUOTED": "hello world",
		"SINGLE": "value",
		"EMPTY":  "",
	}

	if len(values) != len(wanted) {
		t.Fatalf("loadDotEnvFile() len = %d, want %d (%v)", len(values), len(wanted), values)
	}

	for key, want := range wanted {
		if got := values[key]; got != want {
			t.Fatalf("loadDotEnvFile()[%q] = %q, want %q", key, got, want)
		}
	}
}

func TestLoadDotEnvFileMissingReturnsEmptyMap(t *testing.T) {
	values, err := loadDotEnvFile(filepath.Join(t.TempDir(), ".env"))
	if err != nil {
		t.Fatalf("loadDotEnvFile() error = %v", err)
	}
	if len(values) != 0 {
		t.Fatalf("loadDotEnvFile() len = %d, want 0", len(values))
	}
}

func TestMergeEnvValuesKeepsInheritedValues(t *testing.T) {
	base := []string{"FOO=from-env", "BAR=keep"}
	dotenvValues := map[string]string{
		"FOO": "from-dotenv",
		"BAZ": "from-dotenv",
	}

	merged := mergeEnvValues(base, dotenvValues)
	if !slices.Contains(merged, "FOO=from-env") {
		t.Fatalf("mergeEnvValues() missing inherited FOO: %v", merged)
	}
	if slices.Contains(merged, "FOO=from-dotenv") {
		t.Fatalf("mergeEnvValues() should not override inherited FOO: %v", merged)
	}
	if !slices.Contains(merged, "BAZ=from-dotenv") {
		t.Fatalf("mergeEnvValues() missing dotenv-only BAZ: %v", merged)
	}
}

func TestDetachedSpeakEnvLoadsSiblingDotEnv(t *testing.T) {
	tempDir := t.TempDir()
	execPath := filepath.Join(tempDir, "tts2mic-mcp")
	if err := os.WriteFile(execPath, []byte(""), 0o755); err != nil {
		t.Fatalf("WriteFile(exec) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, ".env"), []byte("DOTENV_ONLY=loaded\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(.env) error = %v", err)
	}

	env, err := detachedSpeakEnv(execPath)
	if err != nil {
		t.Fatalf("detachedSpeakEnv() error = %v", err)
	}
	if !slices.Contains(env, "DOTENV_ONLY=loaded") {
		t.Fatalf("detachedSpeakEnv() missing dotenv value: %v", env)
	}
}
