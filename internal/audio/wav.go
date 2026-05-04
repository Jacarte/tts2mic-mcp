package audio

import (
    "bytes"
    "encoding/binary"
)

// EncodeWAV encodes mono/stereo int16 PCM to WAV bytes
func EncodeWAV(pcm []int16, sampleRate int, channels int) ([]byte, error) {
    buf := &bytes.Buffer{}

    // RIFF header
    _ = binary.Write(buf, binary.LittleEndian, []byte{'R','I','F','F'})
    dataLen := uint32(len(pcm) * 2)
    riffLen := 36 + dataLen
    _ = binary.Write(buf, binary.LittleEndian, riffLen)
    _ = binary.Write(buf, binary.LittleEndian, []byte{'W','A','V','E'})

    // fmt chunk
    _ = binary.Write(buf, binary.LittleEndian, []byte{'f','m','t',' '})
    _ = binary.Write(buf, binary.LittleEndian, uint32(16))          // PCM
    _ = binary.Write(buf, binary.LittleEndian, uint16(1))           // PCM format
    _ = binary.Write(buf, binary.LittleEndian, uint16(channels))
    _ = binary.Write(buf, binary.LittleEndian, uint32(sampleRate))
    byteRate := sampleRate * channels * 2
    _ = binary.Write(buf, binary.LittleEndian, uint32(byteRate))
    blockAlign := channels * 2
    _ = binary.Write(buf, binary.LittleEndian, uint16(blockAlign))
    _ = binary.Write(buf, binary.LittleEndian, uint16(16)) // bits

    // data chunk
    _ = binary.Write(buf, binary.LittleEndian, []byte{'d','a','t','a'})
    _ = binary.Write(buf, binary.LittleEndian, dataLen)
    _ = binary.Write(buf, binary.LittleEndian, pcm)

    return buf.Bytes(), nil
}
