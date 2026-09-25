package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/Jacarte/tts2mic-mcp/internal/inject"
	"github.com/Jacarte/tts2mic-mcp/internal/jobs"
)

const jobTimeout = 2 * time.Minute

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if err := loadSiblingEnv(); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if len(os.Args) > 1 && os.Args[1] != "serve" {
		return runCLI(ctx, os.Args[1:])
	}
	manager := jobs.New(ctx)
	defer manager.Close()
	return server.NewStdioServer(newMCPServer(manager)).Listen(ctx, os.Stdin, os.Stdout)
}

func resolveTarget(explicit string) (string, error) {
	return inject.ResolveTarget(explicit, os.Getenv("TTS2MIC_BACKEND"), runtime.GOOS)
}

func parseDelay(value string) (time.Duration, error) {
	if value == "" {
		return 0, nil
	}
	delay, err := time.ParseDuration(value)
	if err != nil || delay < 0 || delay >= jobTimeout {
		return 0, fmt.Errorf("delay must be a nonnegative Go duration below %s (for example 500ms)", jobTimeout)
	}
	return delay, nil
}

func waitDelay(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return ctx.Err()
	case <-ctx.Done():
		return ctx.Err()
	}
}

func runCLI(parent context.Context, args []string) error {
	command := args[0]
	if command != "speak" && command != "prepare" && command != "play" {
		return fmt.Errorf("unknown command %q; use serve, speak, prepare or play", command)
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	target := flags.String("target", "", "backend; defaults to TTS2MIC_BACKEND or OS default")
	text := flags.String("text", "", "text to synthesize")
	voice := flags.String("voice", "", "provider voice ID")
	file := flags.String("file", "", "PCM16 WAV fixture for play")
	out := flags.String("out", "/tmp/tts2mic.wav", "WAV output for prepare or chrome-file")
	delayString := flags.String("delay", "", "delay before injection, e.g. 500ms")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	delay, err := parseDelay(*delayString)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(parent, jobTimeout)
	defer cancel()
	var wav []byte
	var backend string
	if command != "prepare" {
		backend, err = resolveTarget(*target)
		if err != nil {
			return err
		}
	}
	if command == "play" {
		if *file == "" {
			return errors.New("play requires --file")
		}
		wav, err = readWAV(*file)
	} else {
		wav, err = synthesize(ctx, *text, *voice)
	}
	if err != nil {
		return err
	}
	if command == "prepare" {
		if err := os.WriteFile(*out, wav, 0o600); err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"status": "prepared", "file": *out})
	}
	if err := waitDelay(ctx, delay); err != nil {
		return err
	}
	if backend == "chrome-file" {
		inject.SetOutputPath(*out)
	}
	if err := inject.NewBackend(backend).Inject(ctx, wav); err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"status": "completed", "target": backend})
}

type toolInput struct {
	Text    string `json:"text"`
	Voice   string `json:"voice"`
	Target  string `json:"target"`
	Delay   string `json:"delay"`
	DelayMS string `json:"delay_ms"` // Compatibility: still a Go duration string.
	ClipID  string `json:"clip_id"`
	ID      string `json:"id"`
}

func result(value any, err error) (*mcp.CallToolResult, error) {
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	output := mcp.NewToolResultText(string(data))
	output.StructuredContent = value
	return output, nil
}

func newMCPServer(manager *jobs.Manager) *server.MCPServer {
	srv := server.NewMCPServer("tts2mic-mcp", "0.2.0", server.WithToolCapabilities(true))
	for _, name := range []string{"speak", "speak_delay"} {
		options := []mcp.ToolOption{
			mcp.WithDescription("Queue supervised speech on the configured virtual microphone. Returns a job ID; poll job_status. Completion is backend completion, not browser capture proof. Jobs are serial; delay starts after synthesis. Cancel with cancel_job."),
			mcp.WithString("text", mcp.Required(), mcp.Description("Text to synthesize (1–4000 bytes).")),
			mcp.WithString("voice", mcp.Description("Optional provider voice ID.")),
			mcp.WithString("target", mcp.Description("Optional backend: pipewire, macos-blackhole or chrome-file.")),
			mcp.WithString("delay", mcp.Description("Optional duration before injection, after synthesis (e.g. 500ms).")),
		}
		if name == "speak_delay" {
			options = append(options, mcp.WithString("delay_ms", mcp.Required(), mcp.Description("Legacy duration string, e.g. 500ms, not an integer.")))
		}
		legacy := name == "speak_delay"
		srv.AddTool(mcp.NewTool(name, options...), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var in toolInput
			if err := req.BindArguments(&in); err != nil {
				return result(nil, err)
			}
			if err := validateText(in.Text); err != nil {
				return result(nil, err)
			}
			if legacy {
				if in.DelayMS == "" {
					return result(nil, errors.New("speak_delay requires delay_ms"))
				}
				if in.Delay != "" {
					return result(nil, errors.New("use either delay or delay_ms"))
				}
				in.Delay = in.DelayMS
			}
			delay, err := parseDelay(in.Delay)
			if err != nil {
				return result(nil, err)
			}
			target, err := resolveTarget(in.Target)
			if err != nil {
				return result(nil, err)
			}
			if err := ctx.Err(); err != nil {
				return result(nil, err)
			}
			snapshot, err := manager.Submit(0, jobTimeout, func(jobCtx context.Context) error {
				wav, err := synthesize(jobCtx, in.Text, in.Voice)
				if err != nil {
					return err
				}
				if err := waitDelay(jobCtx, delay); err != nil {
					return err
				}
				return inject.NewBackend(target).Inject(jobCtx, wav)
			})
			return result(snapshot, err)
		})
	}
	srv.AddTool(mcp.NewTool("prepare",
		mcp.WithDescription("Synthesize and persist a content-addressed WAV without playing it. Use play with the returned clip_id for timing-sensitive tests."),
		mcp.WithString("text", mcp.Required()), mcp.WithString("voice"),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var in toolInput
		if err := req.BindArguments(&in); err != nil {
			return result(nil, err)
		}
		ctx, cancel := context.WithTimeout(ctx, jobTimeout)
		defer cancel()
		clip, err := prepareClip(ctx, in.Text, in.Voice)
		return result(clip, err)
	})
	srv.AddTool(mcp.NewTool("play",
		mcp.WithDescription("Queue a previously prepared clip. No synthesis occurs. Returns a supervised job ID. Delay starts at the head of the serial queue."),
		mcp.WithString("clip_id", mcp.Required()), mcp.WithString("target"), mcp.WithString("delay"),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var in toolInput
		if err := req.BindArguments(&in); err != nil {
			return result(nil, err)
		}
		delay, err := parseDelay(in.Delay)
		if err != nil {
			return result(nil, err)
		}
		target, err := resolveTarget(in.Target)
		if err != nil {
			return result(nil, err)
		}
		// Validate before enqueue; read at execution to avoid retaining many WAVs.
		if err := validateClipID(in.ClipID); err != nil {
			return result(nil, err)
		}
		if err := ctx.Err(); err != nil {
			return result(nil, err)
		}
		snapshot, err := manager.Submit(delay, jobTimeout, func(jobCtx context.Context) error {
			wav, err := loadClip(in.ClipID)
			if err != nil {
				return err
			}
			return inject.NewBackend(target).Inject(jobCtx, wav)
		})
		return result(snapshot, err)
	})
	for _, name := range []string{"job_status", "cancel_job"} {
		cancel := name == "cancel_job"
		srv.AddTool(mcp.NewTool(name,
			mcp.WithDescription("Read or cancel an audio job. States: queued, running, cancelling, completed, failed, cancelled. The last 128 terminal jobs are retained."),
			mcp.WithString("id", mcp.Required()),
		), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			id, err := req.RequireString("id")
			if err != nil {
				return result(nil, err)
			}
			if cancel {
				snapshot, err := manager.Cancel(id)
				return result(snapshot, err)
			}
			snapshot, err := manager.Get(id)
			return result(snapshot, err)
		})
	}
	return srv
}

func validateText(text string) error {
	if strings.TrimSpace(text) == "" || len(text) > 4000 {
		return errors.New("text must contain 1–4000 bytes of nonblank text")
	}
	return nil
}
