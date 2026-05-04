package cache

import (
    "crypto/sha256"
    "encoding/hex"
    "fmt"
    "os"
    "path/filepath"
    "strings"
)

const DefaultDir = ".tts2mic-cache"

// Key represents a stable TTS cache key.
// Logical form: lang:voice_id:provider:hash(text)
type Key struct {
    Lang     string
    VoiceID  string
    Provider string
    TextHash string
}

func NewKey(lang, voiceID, provider, text string) Key {
    sum := sha256.Sum256([]byte(text))
    return Key{
        Lang:     normalize(lang, "unknown-lang"),
        VoiceID:  normalize(voiceID, "default"),
        Provider: normalize(provider, "unknown-provider"),
        TextHash: hex.EncodeToString(sum[:]),
    }
}

func (k Key) String() string {
    return fmt.Sprintf("%s:%s:%s:%s", k.Lang, k.VoiceID, k.Provider, k.TextHash)
}

func (k Key) Filename() string {
    // Keep the logical key readable but filesystem-safe.
    return strings.ReplaceAll(k.String(), ":", "__") + ".pcm"
}

type Store struct {
    Dir string
}

func NewStore(dir string) Store {
    if dir == "" {
        dir = DefaultDir
    }
    return Store{Dir: dir}
}

func (s Store) Path(k Key) string {
    return filepath.Join(s.Dir, k.Provider, k.Lang, k.VoiceID, k.Filename())
}

func (s Store) Get(k Key) ([]byte, bool, error) {
    b, err := os.ReadFile(s.Path(k))
    if err == nil {
        return b, true, nil
    }
    if os.IsNotExist(err) {
        return nil, false, nil
    }
    return nil, false, err
}

func (s Store) Set(k Key, pcm []byte) error {
    path := s.Path(k)
    if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
        return err
    }

    tmp := path + ".tmp"
    if err := os.WriteFile(tmp, pcm, 0644); err != nil {
        return err
    }
    return os.Rename(tmp, path)
}

func normalize(v, fallback string) string {
    v = strings.TrimSpace(v)
    if v == "" {
        return fallback
    }

    var b strings.Builder
    for _, r := range v {
        switch {
        case r >= 'a' && r <= 'z':
            b.WriteRune(r)
        case r >= 'A' && r <= 'Z':
            b.WriteRune(r)
        case r >= '0' && r <= '9':
            b.WriteRune(r)
        case r == '-', r == '_', r == '.':
            b.WriteRune(r)
        default:
            b.WriteRune('_')
        }
    }
    return b.String()
}
