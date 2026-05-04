package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/Jacarte/tts2mic-mcp/internal/audio"
	"github.com/Jacarte/tts2mic-mcp/internal/inject"
	"github.com/Jacarte/tts2mic-mcp/internal/tts"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "speak" {
		speakCLI()
		return
	}

	srv := server.NewMCPServer(
		"tts2mic-mcp",
		"0.1.0",
		server.WithToolCapabilities(true),
	)

	speakTool := mcp.NewTool("speak",
		mcp.WithDescription("Synthesize text to speech and inject into a simulated microphone"),
		mcp.WithString("target", mcp.Required()),
		mcp.WithString("text", mcp.Required()),
		mcp.WithString("voice"),
	)

	srv.AddTool(speakTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		target, err := req.RequireString("target")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		text, err := req.RequireString("text")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		voice := req.GetString("voice", "")

		err = Speak(ctx, SpeakInput{Target: target, Text: text, Voice: voice})
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText("ok"), nil
	})

	if err := server.ServeStdio(srv); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

func speakCLI() {
	fs := flag.NewFlagSet("speak", flag.ExitOnError)
	target := fs.String("target", "chrome-file", "injection target")
	text := fs.String("text", "hello world", "text to synthesize")
	voice := fs.String("voice", "default", "voice name")
	out := fs.String("out", "/tmp/tts2mic.wav", "output wav (for chrome-file)")
	_ = fs.Parse(os.Args[2:])

	in := SpeakInput{Target: *target, Text: *text, Voice: *voice}
	if *target == "chrome-file" {
		inject.SetOutputPath(*out)
	}

	if err := Speak(context.Background(), in); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("ok\n")
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
