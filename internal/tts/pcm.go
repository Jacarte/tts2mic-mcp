package tts

import "encoding/binary"

func int16ToBytes(in []int16) []byte {
    b := make([]byte, len(in)*2)
    for i, v := range in {
        binary.LittleEndian.PutUint16(b[i*2:], uint16(v))
    }
    return b
}

func bytesToInt16(b []byte) []int16 {
    if len(b)%2 != 0 {
        return nil
    }
    out := make([]int16, len(b)/2)
    for i := 0; i < len(out); i++ {
        out[i] = int16(binary.LittleEndian.Uint16(b[i*2:]))
    }
    return out
}
