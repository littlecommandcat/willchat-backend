package data

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
)

const (
	path string = ".env"
)

type AppLatestVersion struct {
	Python *string `json:"python,omitempty"`
	Rust   *string `json:"rust,omitempty"`
	Cpp    *string `json:"cpp,omitempty"`
}

var (
	LatestServerVersion string
	LatestAppVersion    AppLatestVersion
)

func LoadEnv() error {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return fmt.Errorf("invalid %s entry at line %d", path, lineNumber)
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), "\"'")
		if os.Getenv(key) == "" {
			if err := os.Setenv(key, value); err != nil {
				return fmt.Errorf("set environment variable %s: %w", key, err)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	return nil
}

func GetEnvPointer(key string) *string {
	value, exists := os.LookupEnv(key)
	if !exists || value == "" {
		return nil
	}
	return &value
}
