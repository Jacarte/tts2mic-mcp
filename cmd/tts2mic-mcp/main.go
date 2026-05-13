package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/sirupsen/logrus"

	"github.com/Jacarte/tts2mic-mcp/internal/audio"
	"github.com/Jacarte/tts2mic-mcp/internal/inject"
	"github.com/Jacarte/tts2mic-mcp/internal/tts"
)

const logFilePath = "/tmp/tts2mic"

const macOSBackendTarget = "macos-blackhole"

func main() {
	logger, closeLogger, err := newLogger()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer closeLogger()

	if len(os.Args) > 1 && os.Args[1] == "speak" {
		speakCLI(logger)
		return
	}

	logger.WithField("log_file", logFilePath).Info("starting MCP server")

	srv := server.NewMCPServer(
		"tts2mic-mcp",
		"0.1.0",
		server.WithToolCapabilities(true),
	)

	// the simpler the better
	speakTool := mcp.NewTool("speak",
		mcp.WithDescription("Speak text through the default simulated microphone backend (currently macos-blackhole). The tool starts a detached child process and returns as soon as that process is launched. A successful response means the playback job was started, not that audio playback already finished."),
		mcp.WithString("text", mcp.Required(), mcp.Description("Text to synthesize and play through the simulated microphone.")),
	)

	speakDelayTool := mcp.NewTool("speak_delay",
		mcp.WithDescription("Speak text through the default simulated microphone backend (currently macos-blackhole) after a delay. The tool starts a detached child process and returns as soon as that delayed playback job is launched. A successful response means the delayed job was scheduled, not that playback already finished."),
		mcp.WithString("text", mcp.Required(), mcp.Description("Text to synthesize and play through the simulated microphone.")),
		mcp.WithString("delay_ms", mcp.Required(), mcp.Description("Delay before playback as a Go duration string, for example '500ms', '2s', or '1.5s'. Plain integers like '500' are invalid.")),
	)

	srv.AddTool(speakTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		text, err := req.RequireString("text")
		if err != nil {
			logger.WithError(err).Error("invalid speak request")
			return mcp.NewToolResultError(err.Error()), nil
		}

		logger.WithField("text_length", len(text)).Info("accepted speak request")

		if err := launchDetachedSpeak(logger, text, 0); err != nil {
			logger.WithError(err).WithField("text_length", len(text)).Error("failed to launch detached speak process")
			return mcp.NewToolResultError(err.Error()), nil
		}

		return mcp.NewToolResultText("ok"), nil
	})

	srv.AddTool(speakDelayTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		text, err := req.RequireString("text")
		if err != nil {
			logger.WithError(err).Error("invalid speak_delay request text")
			return mcp.NewToolResultError(err.Error()), nil
		}

		delayMs, err := req.RequireString("delay_ms")
		if err != nil {
			logger.WithError(err).Error("invalid speak_delay request delay")
			return mcp.NewToolResultError(err.Error()), nil
		}
		delayMsParsed, err := time.ParseDuration(delayMs)
		if err != nil {
			logger.WithError(err).WithField("delay_ms", delayMs).Error("failed to parse speak_delay duration")
			return mcp.NewToolResultError(fmt.Sprintf("invalid delay_ms: %v", err)), nil
		}

		logger.WithFields(logrus.Fields{
			"delay":       delayMsParsed.String(),
			"text_length": len(text),
		}).Info("accepted speak_delay request")

		if err := launchDetachedSpeak(logger, text, delayMsParsed); err != nil {
			logger.WithError(err).WithFields(logrus.Fields{
				"delay":       delayMsParsed.String(),
				"text_length": len(text),
			}).Error("failed to launch detached delayed speak process")
			return mcp.NewToolResultError(err.Error()), nil
		}

		return mcp.NewToolResultText("ok"), nil
	})

	if err := server.ServeStdio(srv); err != nil {
		logger.WithError(err).Error("MCP server stopped with error")
		os.Exit(1)
	}
}

func speakCLI(logger *logrus.Logger) {
	fs := flag.NewFlagSet("speak", flag.ExitOnError)
	target := fs.String("target", "chrome-file", "injection target")
	text := fs.String("text", "hello world", "text to synthesize")
	voice := fs.String("voice", "default", "voice name")
	delay := fs.String("delay", "", "delay before speaking (duration, for example 500ms or 2s)")
	out := fs.String("out", "/tmp/tts2mic.wav", "output wav (for chrome-file)")
	_ = fs.Parse(os.Args[2:])

	if *voice == "default" {
		if v := os.Getenv("ELEVENLABS_VOICE_ID"); v != "" {
			*voice = v
		}
	}

	in := SpeakInput{Target: *target, Text: *text, Voice: *voice}
	if *target == "chrome-file" {
		inject.SetOutputPath(*out)
	}

	if *delay != "" {
		delayDuration, err := time.ParseDuration(*delay)
		if err != nil {
			logger.WithError(err).WithField("delay", *delay).Error("CLI speak delay is invalid")
			os.Exit(1)
		}

		logger.WithField("delay", delayDuration.String()).Info("delaying CLI speak request")
		time.Sleep(delayDuration)
	}

	logger.WithFields(logrus.Fields{
		"target":      in.Target,
		"text_length": len(in.Text),
		"voice":       in.Voice,
	}).Info("starting CLI speak request")

	if err := Speak(context.Background(), in); err != nil {
		logger.WithError(err).WithFields(logrus.Fields{
			"target":      in.Target,
			"text_length": len(in.Text),
			"voice":       in.Voice,
		}).Error("CLI speak request failed")
		os.Exit(1)
	}

	logger.WithFields(logrus.Fields{
		"target":      in.Target,
		"text_length": len(in.Text),
		"voice":       in.Voice,
	}).Info("completed CLI speak request")

	fmt.Printf("ok\n")
}

func newLogger() (*logrus.Logger, func(), error) {
	logFile, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file %q: %w", logFilePath, err)
	}

	logger := logrus.New()
	logger.SetOutput(logFile)
	logger.SetLevel(logrus.InfoLevel)
	logger.SetFormatter(&logrus.TextFormatter{
		DisableColors:   true,
		FullTimestamp:   true,
		TimestampFormat: time.RFC3339,
	})

	closeLogger := func() {
		if err := logFile.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to close log file: %v\n", err)
		}
	}

	return logger, closeLogger, nil
}

func launchDetachedSpeak(logger *logrus.Logger, text string, delay time.Duration) error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}

	cmd, err := newDetachedSpeakCommand(execPath, text, delay)
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start detached speak process: %w", err)
	}

	logger.WithFields(logrus.Fields{
		"delay":       delay.String(),
		"pid":         cmd.Process.Pid,
		"text_length": len(text),
	}).Info("launched detached speak process")

	if err := cmd.Process.Release(); err != nil {
		return fmt.Errorf("release detached speak process: %w", err)
	}

	return nil
}

func newDetachedSpeakCommand(execPath string, text string, delay time.Duration) (*exec.Cmd, error) {
	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", os.DevNull, err)
	}

	args := detachedSpeakArgs(text, delay)
	cmd := exec.Command(execPath, args...)
	env, err := detachedSpeakEnv(execPath)
	if err != nil {
		_ = devNull.Close()
		return nil, err
	}
	cmd.Env = env
	cmd.Stdin = devNull
	cmd.Stdout = devNull
	cmd.Stderr = devNull
	cmd.Dir = currentWorkingDirectory()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	return cmd, nil
}

func detachedSpeakArgs(text string, delay time.Duration) []string {
	args := []string{"speak", "--target", macOSBackendTarget, "--text", text}
	if delay > 0 {
		args = append(args, "--delay", delay.String())
	}

	return args
}

func detachedSpeakEnv(execPath string) ([]string, error) {
	base := os.Environ()
	dotenvPath := filepath.Join(filepath.Dir(execPath), ".env")
	dotenvValues, err := loadDotEnvFile(dotenvPath)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", dotenvPath, err)
	}

	return mergeEnvValues(base, dotenvValues), nil
}

func loadDotEnvFile(path string) (map[string]string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]string{}, nil
		}

		return nil, err
	}

	values := map[string]string{}
	for lineNumber, rawLine := range strings.Split(string(content), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("invalid line %d", lineNumber+1)
		}

		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("invalid empty key on line %d", lineNumber+1)
		}

		value = strings.TrimSpace(value)
		if len(value) >= 2 {
			if value[0] == '\'' && value[len(value)-1] == '\'' {
				value = value[1 : len(value)-1]
			} else if value[0] == '"' && value[len(value)-1] == '"' {
				unquoted, unquoteErr := strconv.Unquote(value)
				if unquoteErr != nil {
					return nil, fmt.Errorf("invalid quoted value for %s on line %d: %w", key, lineNumber+1, unquoteErr)
				}
				value = unquoted
			}
		}

		values[key] = value
	}

	return values, nil
}

func mergeEnvValues(base []string, dotenvValues map[string]string) []string {
	merged := make([]string, 0, len(base)+len(dotenvValues))
	seen := map[string]struct{}{}

	for _, entry := range base {
		merged = append(merged, entry)
		key, _, ok := strings.Cut(entry, "=")
		if ok {
			seen[key] = struct{}{}
		}
	}

	for key, value := range dotenvValues {
		if _, exists := seen[key]; exists {
			continue
		}
		merged = append(merged, key+"="+value)
	}

	return merged
}

func currentWorkingDirectory() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}

	return dir
}

type SpeakInput struct {
	Target string `json:"target"`
	Text   string `json:"text"`
	Voice  string `json:"voice,omitempty"`
}

func Speak(ctx context.Context, in SpeakInput) error {
	provider := tts.NewProviderFromEnv()
	pcm, sr, err := provider.Synthesize(ctx, in.Text, in.Voice)
	if err != nil {
		return err
	}

	wav, err := audio.EncodeWAV(pcm, sr, 1)
	if err != nil {
		return err
	}

	inj := inject.NewBackend(in.Target)
	return inj.Inject(ctx, wav)
}
