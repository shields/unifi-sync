package main

import (
	"os"
	"path/filepath"
	"strings"
)

// writeConfigFile writes a JSON config file. Slug is safe (from slugify: [a-z0-9-]).
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
