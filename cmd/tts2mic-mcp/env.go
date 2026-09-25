package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Preserve the previous executable-sibling .env behavior; inherited values win.
// Diagnostics stay on stderr, never on MCP stdout. No credentials are logged.
func loadSiblingEnv() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	values, err := loadDotEnvFile(filepath.Join(filepath.Dir(executable), ".env"))
	if err != nil {
		return err
	}
	for key, value := range values {
		if _, exists := os.LookupEnv(key); !exists {
			if err := os.Setenv(key, value); err != nil {
				return err
			}
		}
	}
	return nil
}

func loadDotEnvFile(path string) (map[string]string, error) {
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	values := map[string]string{}
	for lineNumber, raw := range strings.Split(string(content), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if !ok || key == "" {
			return nil, fmt.Errorf("invalid .env line %d", lineNumber+1)
		}
		if len(value) >= 2 {
			if value[0] == '\'' && value[len(value)-1] == '\'' {
				value = value[1 : len(value)-1]
			} else if value[0] == '"' && value[len(value)-1] == '"' {
				value, err = strconv.Unquote(value)
				if err != nil {
					return nil, fmt.Errorf("invalid quoted .env value on line %d", lineNumber+1)
				}
			}
		}
		values[key] = value
	}
	return values, nil
}
