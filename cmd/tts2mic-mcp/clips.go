package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Jacarte/tts2mic-mcp/internal/audio"
	"github.com/Jacarte/tts2mic-mcp/internal/tts"
)

const maxWAVBytes = 64 << 20

type clipInfo struct {
	ID         string `json:"clip_id"`
	SampleRate int    `json:"sample_rate"`
	Channels   int    `json:"channels"`
	DurationMS int64  `json:"duration_ms"`
}

func synthesize(ctx context.Context, text, voice string) ([]byte, error) {
	if err := validateText(text); err != nil {
		return nil, err
	}
	switch os.Getenv("TTS_PROVIDER") {
	case "", "stub", "elevenlabs":
	default:
		return nil, fmt.Errorf("unknown TTS_PROVIDER %q; use stub or elevenlabs", os.Getenv("TTS_PROVIDER"))
	}
	if voice == "default" {
		voice = ""
	}
	pcm, rate, err := tts.NewProviderFromEnv().Synthesize(ctx, text, voice)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	wav, err := audio.EncodeWAV(pcm, rate, 1)
	if err != nil {
		return nil, err
	}
	if _, err := audio.DecodePCM16(wav); err != nil {
		return nil, err
	}
	return wav, nil
}

func clipDirectory() string {
	if dir := os.Getenv("TTS2MIC_CLIP_DIR"); dir != "" {
		return dir
	}
	return ".tts2mic-clips"
}

func validateClipID(id string) error {
	decoded, err := hex.DecodeString(id)
	if err != nil || len(decoded) != sha256.Size || hex.EncodeToString(decoded) != id {
		return errors.New("clip_id must be a lowercase SHA-256 identifier returned by prepare")
	}
	return nil
}

func prepareClip(ctx context.Context, text, voice string) (clipInfo, error) {
	wav, err := synthesize(ctx, text, voice)
	if err != nil {
		return clipInfo{}, err
	}
	decoded, err := audio.DecodePCM16(wav)
	if err != nil {
		return clipInfo{}, err
	}
	sum := sha256.Sum256(wav)
	id := hex.EncodeToString(sum[:])
	dir := clipDirectory()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return clipInfo{}, err
	}
	f, err := os.CreateTemp(dir, ".prepare-*.wav")
	if err != nil {
		return clipInfo{}, err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(wav); err != nil {
		f.Close()
		return clipInfo{}, err
	}
	if err := f.Close(); err != nil {
		return clipInfo{}, err
	}
	if err := os.Rename(f.Name(), filepath.Join(dir, id+".wav")); err != nil {
		return clipInfo{}, err
	}
	return clipInfo{ID: id, SampleRate: decoded.SampleRate, Channels: decoded.Channels, DurationMS: int64(len(decoded.Samples)) * 1000 / int64(decoded.SampleRate*decoded.Channels*2)}, nil
}

func loadClip(id string) ([]byte, error) {
	if err := validateClipID(id); err != nil {
		return nil, err
	}
	wav, err := readWAV(filepath.Join(clipDirectory(), id+".wav"))
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(wav)
	if hex.EncodeToString(sum[:]) != id {
		return nil, errors.New("prepared clip content hash mismatch")
	}
	return wav, nil
}

func readWAV(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxWAVBytes {
		return nil, errors.New("audio input must be a regular WAV file no larger than 64 MiB")
	}
	wav, err := io.ReadAll(io.LimitReader(f, maxWAVBytes+1))
	if err != nil {
		return nil, err
	}
	if len(wav) > maxWAVBytes {
		return nil, errors.New("WAV exceeds 64 MiB limit")
	}
	if _, err := audio.DecodePCM16(wav); err != nil {
		return nil, err
	}
	return wav, nil
}
