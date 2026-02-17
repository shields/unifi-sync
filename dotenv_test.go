package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseDotenv(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    map[string]string
		wantErr bool
	}{
		{
			name:  "simple key=value",
			input: "FOO=bar\nBAZ=qux\n",
			want:  map[string]string{"FOO": "bar", "BAZ": "qux"},
		},
		{
			name:  "comments and blank lines",
			input: "# comment\n\nFOO=bar\n  # indented comment\n",
			want:  map[string]string{"FOO": "bar"},
		},
		{
			name:  "double quoted value",
			input: `FOO="hello world"` + "\n",
			want:  map[string]string{"FOO": "hello world"},
		},
		{
			name:  "single quoted value",
			input: "FOO='hello world'\n",
			want:  map[string]string{"FOO": "hello world"},
		},
		{
			name:  "whitespace trimmed",
			input: "  FOO  =  bar  \n",
			want:  map[string]string{"FOO": "bar"},
		},
		{
			name:  "value with equals sign",
			input: "URL=https://host:8443/api?foo=bar\n",
			want:  map[string]string{"URL": "https://host:8443/api?foo=bar"},
		},
		{
			name:  "empty value",
			input: "FOO=\n",
			want:  map[string]string{"FOO": ""},
		},
		{
			name:  "empty input",
			input: "",
			want:  map[string]string{},
		},
		{
			name:  "line without equals is skipped",
			input: "NOEQUALSSIGN\nFOO=bar\n",
			want:  map[string]string{"FOO": "bar"},
		},
		{
			name:  "empty key is skipped",
			input: "=value\nFOO=bar\n",
			want:  map[string]string{"FOO": "bar"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDotenv(strings.NewReader(tt.input))
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseDotenv() error = %v, wantErr %v", err, tt.wantErr)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("parseDotenv() got %d entries, want %d", len(got), len(tt.want))
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("parseDotenv()[%q] = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestLoadDotenv(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	os.WriteFile(envFile, []byte("NEW_VAR=from_file\nEXISTING_VAR=from_file\n"), 0o644)

	t.Setenv("EXISTING_VAR", "from_env")
	t.Cleanup(func() { os.Unsetenv("NEW_VAR") })

	if err := loadDotenv(envFile); err != nil {
		t.Fatalf("loadDotenv() error = %v", err)
	}

	if got := os.Getenv("NEW_VAR"); got != "from_file" {
		t.Errorf("NEW_VAR = %q, want %q", got, "from_file")
	}
	if got := os.Getenv("EXISTING_VAR"); got != "from_env" {
		t.Errorf("EXISTING_VAR = %q, want %q (env takes precedence)", got, "from_env")
	}
}

func TestLoadDotenvMissing(t *testing.T) {
	err := loadDotenv("/nonexistent/.env")
	if err != nil {
		t.Errorf("loadDotenv(missing) should return nil, got %v", err)
	}
}

func TestLoadDotenvScanError(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	// Line longer than bufio.MaxScanTokenSize triggers scanner error
	longLine := strings.Repeat("x", 1024*1024)
	os.WriteFile(envFile, []byte("FOO="+longLine+"\n"), 0o644)

	err := loadDotenv(envFile)
	if err == nil {
		t.Error("loadDotenv(scan error) should return error")
	}
}

func TestLoadDotenvSetenvError(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	os.WriteFile(envFile, []byte("SETENV_ERR_VAR=val\n"), 0o644)

	errSetenv := errors.New("setenv failed")
	orig := setenvFunc
	setenvFunc = func(k, v string) error { return errSetenv }
	defer func() { setenvFunc = orig }()

	err := loadDotenv(envFile)
	if !errors.Is(err, errSetenv) {
		t.Errorf("loadDotenv() error = %v, want %v", err, errSetenv)
	}
}

func TestLoadDotenvUnreadable(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, ".env")
	os.WriteFile(envFile, []byte("FOO=bar\n"), 0o644)
	os.Chmod(envFile, 0o000)
	t.Cleanup(func() { os.Chmod(envFile, 0o644) })

	err := loadDotenv(envFile)
	if err == nil {
		t.Error("loadDotenv(unreadable) should return error")
	}
}
