package audio

import (
	"encoding/binary"
	"testing"
)

// Build fixtures independently of EncodeWAV, including optional RIFF chunks.
func fixture(rate, channels int, junk bool) []byte {
	data := make([]byte, 44+rate*channels*2/10)
	copy(data, "RIFF")
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], uint16(channels))
	binary.LittleEndian.PutUint32(data[24:], uint32(rate))
	binary.LittleEndian.PutUint32(data[28:], uint32(rate*channels*2))
	binary.LittleEndian.PutUint16(data[32:], uint16(channels*2))
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], uint32(len(data)-44))
	if junk {
		extra := []byte{'J', 'U', 'N', 'K', 1, 0, 0, 0, 7, 0}
		data = append(append(append([]byte{}, data[:12]...), extra...), data[12:]...)
	}
	binary.LittleEndian.PutUint32(data[4:], uint32(len(data)-8))
	return data
}

func TestDecodePCM16FormatsAndChunks(t *testing.T) {
	for _, rate := range []int{16000, 24000, 48000} {
		for _, channels := range []int{1, 2} {
			got, err := DecodePCM16(fixture(rate, channels, true))
			if err != nil {
				t.Fatal(err)
			}
			if got.SampleRate != rate || got.Channels != channels || len(got.Samples) != rate*channels*2/10 {
				t.Fatalf("wrong format: %+v", got)
			}
		}
	}
}

func TestDecodePCM16RejectsMalformed(t *testing.T) {
	valid := fixture(16000, 1, false)
	badRate := append([]byte{}, valid...)
	binary.LittleEndian.PutUint32(badRate[24:], 0)
	badFormat := append([]byte{}, valid...)
	binary.LittleEndian.PutUint16(badFormat[20:], 3)
	badChunk := append([]byte{}, valid...)
	binary.LittleEndian.PutUint32(badChunk[40:], 0xffffffff)
	badAlign := append([]byte{}, valid...)
	binary.LittleEndian.PutUint16(badAlign[32:], 4)
	for _, input := range [][]byte{nil, []byte("not wav"), valid[:len(valid)-1], badRate, badFormat, badChunk, badAlign} {
		if _, err := DecodePCM16(input); err == nil {
			t.Fatal("malformed WAV accepted")
		}
	}
}
