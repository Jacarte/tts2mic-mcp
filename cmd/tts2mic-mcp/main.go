package main

import (
    "context"
    "encoding/json"
    "flag"
    "fmt"
    "os"

    "github.com/Jacarte/tts2mic-mcp/internal/audio"
    "github.com/Jacarte/tts2mic-mcp/internal/inject"
    "github.com/Jacarte/tts2mic-mcp/internal/tts"
)

// Simple CLI + JSON tool loop (stdio) scaffold

func main() {
    if len(os.Args) > 1 && os.Args[1] == "speak" {
        speakCLI()
        return
    }

    // stdio JSON loop (very minimal MCP-like)
    dec := json.NewDecoder(os.Stdin)
    enc := json.NewEncoder(os.Stdout)

    for {
        var req map[string]any
        if err := dec.Decode(&req); err != nil {
            return
        }

        name, _ := req["name"].(string)
        args, _ := req["arguments"].(map[string]any)

        switch name {
        case "speak":
            var in SpeakInput
            b, _ := json.Marshal(args)
            _ = json.Unmarshal(b, &in)

            err := Speak(context.Background(), in)
            if err != nil {
                _ = enc.Encode(map[string]any{"error": err.Error()})
                continue
            }
            _ = enc.Encode(map[string]any{"ok": true})
        default:
            _ = enc.Encode(map[string]any{"error": "unknown tool"})
        }
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
        // force output path for file backend
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
