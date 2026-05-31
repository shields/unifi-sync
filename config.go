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
	"os"
	"path/filepath"
	"strings"
)

// writeConfigFile writes a JSON config file. The slug comes from slugify, which
// yields a filesystem-safe basename (letters and digits joined by single
// hyphens; no path separators or metacharacters).
func writeConfigFile(dir, resourceType, slug string, data map[string]any) error {
	subdir := filepath.Join(dir, resourceType)
	//nolint:gosec // config dirs need world-readable perms
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		return err
	}
	content, err := marshalJSON(data)
	if err != nil {
		return err
	}
	path := filepath.Join(subdir, slug+".json")
	return os.WriteFile(path, content, 0o644) //nolint:gosec // config files need world-readable perms
}

func readConfigFile(path string) (map[string]any, error) {
	f, err := os.Open(path) //nolint:gosec // path is constructed from trusted config directory
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck // read-only file, close error is harmless
	return decodeJSON(f)
}

func readConfigFiles(dir, resourceType string) (map[string]map[string]any, error) {
	subdir := filepath.Join(dir, resourceType)
	entries, err := os.ReadDir(subdir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]map[string]any{}, nil
		}
		return nil, err
	}
	result := make(map[string]map[string]any)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		slug := strings.TrimSuffix(name, ".json")
		obj, err := readConfigFile(filepath.Join(subdir, name))
		if err != nil {
			return nil, err
		}
		result[slug] = obj
	}
	return result, nil
}
