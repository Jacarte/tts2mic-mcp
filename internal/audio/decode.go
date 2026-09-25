package audio

import (
	"encoding/binary"
	"fmt"
)

// PCM16 is interleaved little-endian PCM. Samples aliases the validated input WAV.
type PCM16 struct {
	Samples    []byte
	SampleRate int
	Channels   int
}

// DecodePCM16 accepts RIFF/WAVE PCM16, including padded and unknown chunks.
// It rejects truncated chunks and incomplete frames instead of guessing a format.
func DecodePCM16(wav []byte) (PCM16, error) {
	fail := func(message string) (PCM16, error) { return PCM16{}, fmt.Errorf("invalid PCM16 WAV: %s", message) }
	if len(wav) < 12 || string(wav[:4]) != "RIFF" || string(wav[8:12]) != "WAVE" {
		return fail("missing RIFF/WAVE header")
	}
	end := uint64(binary.LittleEndian.Uint32(wav[4:8])) + 8
	if end < 12 || end > uint64(len(wav)) {
		return fail("truncated RIFF payload")
	}
	var result PCM16
	var haveFormat, haveData bool
	for offset := uint64(12); offset < end; {
		if end-offset < 8 {
			return fail("truncated chunk header")
		}
		id := string(wav[offset : offset+4])
		size := uint64(binary.LittleEndian.Uint32(wav[offset+4 : offset+8]))
		start := offset + 8
		if size > end-start {
			return fail("truncated " + id + " chunk")
		}
		chunk := wav[start : start+size]
		switch id {
		case "fmt ":
			if haveFormat || len(chunk) < 16 {
				return fail("missing or duplicate format")
			}
			if binary.LittleEndian.Uint16(chunk[:2]) != 1 || binary.LittleEndian.Uint16(chunk[14:16]) != 16 {
				return fail("only uncompressed 16-bit PCM is supported")
			}
			result.Channels = int(binary.LittleEndian.Uint16(chunk[2:4]))
			result.SampleRate = int(binary.LittleEndian.Uint32(chunk[4:8]))
			if result.Channels < 1 || result.Channels > 2 || result.SampleRate < 8000 || result.SampleRate > 192000 {
				return fail("expected mono/stereo audio at 8–192 kHz")
			}
			if int(binary.LittleEndian.Uint16(chunk[12:14])) != result.Channels*2 ||
				int(binary.LittleEndian.Uint32(chunk[8:12])) != result.SampleRate*result.Channels*2 {
				return fail("inconsistent block alignment or byte rate")
			}
			haveFormat = true
		case "data":
			if haveData {
				return fail("duplicate data chunk")
			}
			result.Samples = chunk
			haveData = true
		}
		offset = start + size + size%2
		if offset > end {
			return fail("missing chunk padding")
		}
	}
	if !haveFormat || !haveData {
		return fail("format or data chunk missing")
	}
	if len(result.Samples) == 0 || len(result.Samples)%(result.Channels*2) != 0 {
		return fail("empty or incomplete PCM frames")
	}
	return result, nil
}
