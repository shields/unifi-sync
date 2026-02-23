package main

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAndReadConfigFile(t *testing.T) {
	dir := t.TempDir()
	obj := map[string]any{
		"_id":  "abc123",
		"name": testNameHomeNet,
		"vlan": "100",
	}

	err := writeConfigFile(dir, "networkconf", "homenet", obj)
	if err != nil {
		t.Fatalf("writeConfigFile() error = %v", err)
	}

	path := filepath.Join(dir, "networkconf", "homenet.json")
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("file not created at %s: %v", path, statErr)
	}

	got, err := readConfigFile(path)
	if err != nil {
		t.Fatalf("readConfigFile() error = %v", err)
	}
	if got["name"] != testNameHomeNet {
		t.Errorf("name = %v, want %s", got["name"], testNameHomeNet)
	}
}

func TestReadConfigFiles(t *testing.T) {
	dir := t.TempDir()
	obj1 := map[string]any{"name": testNameHomeNet, "_id": "1"}
	obj2 := map[string]any{"name": "Guest Network", "_id": "2"}

	if err := writeConfigFile(dir, "networkconf", "homenet", obj1); err != nil {
		t.Fatalf("writeConfigFile: %v", err)
	}
	if err := writeConfigFile(dir, "networkconf", "guest-network", obj2); err != nil {
		t.Fatalf("writeConfigFile: %v", err)
	}

	files, err := readConfigFiles(dir, "networkconf")
	if err != nil {
		t.Fatalf("readConfigFiles() error = %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("len = %d, want 2", len(files))
	}
	if files["homenet"]["name"] != testNameHomeNet {
		t.Errorf("homenet name = %v", files["homenet"]["name"])
	}
	if files["guest-network"]["name"] != "Guest Network" {
		t.Errorf("guest-network name = %v", files["guest-network"]["name"])
	}
}

func TestReadConfigFilesEmptyDir(t *testing.T) {
	dir := t.TempDir()
	files, err := readConfigFiles(dir, "networkconf")
	if err != nil {
		t.Fatalf("readConfigFiles() error = %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected empty map, got %d entries", len(files))
	}
}

func TestReadConfigFileInvalid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("{invalid"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := readConfigFile(path)
	if err == nil {
		t.Error("readConfigFile(invalid JSON) should return error")
	}
}

func TestWriteConfigFileUnwritable(t *testing.T) {
	dir := t.TempDir()
	os.Chmod(dir, 0o555) //nolint:errcheck,gosec,revive // test setup
	t.Cleanup(func() {
		os.Chmod(dir, 0o750) //nolint:errcheck,gosec,revive // test cleanup
	})

	err := writeConfigFile(dir, "networkconf", "test", map[string]any{"name": "test"})
	if err == nil {
		t.Error("writeConfigFile(unwritable dir) should return error")
	}
}

func TestReadConfigFilesWithBadFile(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "networkconf")
	if err := os.MkdirAll(subdir, 0o750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "bad.json"), []byte("{invalid"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := readConfigFiles(dir, "networkconf")
	if err == nil {
		t.Error("readConfigFiles with bad JSON should return error")
	}
}

func TestWriteConfigFileMarshalError(t *testing.T) {
	dir := t.TempDir()
	obj := map[string]any{"bad": math.Inf(1)}
	err := writeConfigFile(dir, "networkconf", "test", obj)
	if err == nil {
		t.Error("writeConfigFile(unmarshalable) should return error")
	}
}

func TestReadConfigFileNotFound(t *testing.T) {
	_, err := readConfigFile("/nonexistent/file.json")
	if err == nil {
		t.Error("readConfigFile(missing) should return error")
	}
}

func TestReadConfigFilesUnreadableDir(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "networkconf")
	if err := os.MkdirAll(subdir, 0o750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	os.Chmod(subdir, 0o000) //nolint:errcheck,revive // test setup
	t.Cleanup(func() {
		os.Chmod(subdir, 0o750) //nolint:errcheck,gosec,revive // test cleanup
	})

	_, err := readConfigFiles(dir, "networkconf")
	if err == nil {
		t.Error("readConfigFiles(unreadable dir) should return error")
	}
}

func TestReadConfigFilesSkipsSubdirs(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "networkconf")
	if err := os.MkdirAll(filepath.Join(subdir, "nested"), 0o750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "test.json"), []byte(`{"name":"test"}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	files, err := readConfigFiles(dir, "networkconf")
	if err != nil {
		t.Fatalf("readConfigFiles() error = %v", err)
	}
	if len(files) != 1 {
		t.Errorf("expected 1 file (skipping subdir), got %d", len(files))
	}
}

func TestReadConfigFilesSkipsNonJSON(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "networkconf")
	if err := os.MkdirAll(subdir, 0o750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "readme.txt"), []byte("not json"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "test.json"), []byte(`{"name":"test"}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	files, err := readConfigFiles(dir, "networkconf")
	if err != nil {
		t.Fatalf("readConfigFiles() error = %v", err)
	}
	if len(files) != 1 {
		t.Errorf("expected 1 file, got %d", len(files))
	}
}
