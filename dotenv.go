// Copyright © 2026 Michael Shields
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bufio"
	"errors"
	"io"
	"io/fs"
	"os"
	"strings"
)

// parseDotenv parses KEY=VALUE lines from r. Per spec: only full-line comments
// (lines starting with #) are supported. Inline comments, "export" prefixes,
// and BOM handling are intentionally omitted -- values containing # (e.g., URLs
// with fragments) must not be truncated.
func parseDotenv(r io.Reader) (map[string]string, error) {
	env := make(map[string]string)
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}
		env[key] = value
	}
	return env, scanner.Err()
}

// Injectable for testing; tests in this package are not parallel.
var setenvFunc = os.Setenv

func loadDotenv(path string) error {
	f, err := os.Open(path) //nolint:gosec // path is the well-known .env file location
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	defer f.Close() //nolint:errcheck // read-only file, close error is harmless

	env, err := parseDotenv(f)
	if err != nil {
		return err
	}
	for k, v := range env {
		if _, exists := os.LookupEnv(k); !exists {
			if err := setenvFunc(k, v); err != nil {
				return err
			}
		}
	}
	return nil
}
