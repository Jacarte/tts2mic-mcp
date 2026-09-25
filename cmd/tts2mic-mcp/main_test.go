package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseDelay(t *testing.T) {
	for input, want := range map[string]time.Duration{"": 0, "0s": 0, "500ms": 500 * time.Millisecond, "1.5s": 1500 * time.Millisecond} {
		got, err := parseDelay(input)
		if err != nil || got != want {
			t.Fatalf("%s: %s %v", input, got, err)
		}
	}
	for _, input := range []string{"-1s", "500", "banana", "2m", "3h"} {
		if _, err := parseDelay(input); err == nil {
			t.Fatalf("accepted %q", input)
		}
	}
}

func TestWaitDelayIsCancellable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := waitDelay(ctx, time.Minute); err != context.Canceled {
		t.Fatal(err)
	}
}

func TestTextAndProviderValidation(t *testing.T) {
	for _, text := range []string{"", "   ", strings.Repeat("a", 4001)} {
		if validateText(text) == nil {
			t.Fatal("invalid text accepted")
		}
	}
	t.Setenv("TTS_PROVIDER", "typo")
	if _, err := synthesize(context.Background(), "hello", ""); err == nil {
		t.Fatal("unknown provider accepted")
	}
}

func TestPrepareAndLoadClip(t *testing.T) {
	t.Setenv("TTS_PROVIDER", "stub")
	t.Setenv("TTS_PROVIDER_NAME", "stub")
	t.Setenv("TTS_CACHE_DIR", t.TempDir())
	t.Setenv("TTS2MIC_CLIP_DIR", t.TempDir())
	clip, err := prepareClip(context.Background(), "hello", "")
	if err != nil {
		t.Fatal(err)
	}
	if clip.SampleRate != 16000 || clip.Channels != 1 || clip.DurationMS != 1000 {
		t.Fatalf("%+v", clip)
	}
	if _, err := loadClip(clip.ID); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(clipDirectory(), clip.ID+".wav"), []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadClip(clip.ID); err == nil {
		t.Fatal("corrupt clip accepted")
	}
	for _, id := range []string{"../secret", "", strings.Repeat("A", 64)} {
		if validateClipID(id) == nil {
			t.Fatal("invalid clip ID accepted")
		}
	}
}

func TestLoadDotEnv(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if values, err := loadDotEnvFile(path); err != nil || len(values) != 0 {
		t.Fatalf("missing .env: %v %v", values, err)
	}
	if err := os.WriteFile(path, []byte("# note\nexport FOO=bar\nDOUBLE=\"hello world\"\nSINGLE='abc'\nEMPTY=\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	values, err := loadDotEnvFile(path)
	if err != nil || values["FOO"] != "bar" || values["DOUBLE"] != "hello world" || values["SINGLE"] != "abc" {
		t.Fatalf("%v %v", values, err)
	}
}
