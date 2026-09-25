package inject

import "fmt"

// ResolveTarget is independent of process state so platform defaults are testable.
// CI should explicitly set TTS2MIC_BACKEND rather than rely on auto-detection.
func ResolveTarget(explicit, configured, goos string) (string, error) {
	target := explicit
	if target == "" {
		target = configured
	}
	if target == "" {
		switch goos {
		case "darwin":
			target = "macos-blackhole"
		case "linux":
			target = "pipewire"
		default:
			return "", fmt.Errorf("no default audio backend for %s; select --target explicitly", goos)
		}
	}
	switch target {
	case "macos-blackhole", "pipewire", "chrome-file":
		return target, nil
	default:
		return "", fmt.Errorf("unknown audio backend %q", target)
	}
}
