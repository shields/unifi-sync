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
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setRequiredEnv(t *testing.T, url string) {
	t.Helper()
	t.Setenv("UNIFI_SYNC_URL", url)
	t.Setenv("UNIFI_SYNC_USERNAME", "admin")
	t.Setenv("UNIFI_SYNC_PASSWORD", "pass")
}

func loginMux(responses map[string][]map[string]any) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == testAPILogin {
			w.WriteHeader(http.StatusOK)
			return
		}
		for path, data := range responses {
			if strings.HasSuffix(r.URL.Path, path) {
				w.Write(unifiResponse(data)) //nolint:errcheck,revive // test handler
				return
			}
		}
		if r.Method == http.MethodPut || r.Method == http.MethodPost {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			var obj map[string]any
			_ = json.Unmarshal(body, &obj)                //nolint:errcheck // test handler
			w.Write(unifiResponse([]map[string]any{obj})) //nolint:errcheck,revive // test handler
			return
		}
		w.Write(unifiResponse([]map[string]any{})) //nolint:errcheck,revive // test handler
	})
}

// setTerminal overrides the terminal check so color-gate tests are independent
// of the buffer they write to. The real isTerminal is covered by TestIsTerminal.
func setTerminal(t *testing.T, isTTY bool) {
	t.Helper()
	orig := isTerminalFn
	isTerminalFn = func(io.Writer) bool { return isTTY }
	t.Cleanup(func() { isTerminalFn = orig })
}

func TestRunNoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(nil, &stdout, &stderr)
	if code != 2 {
		t.Errorf("run(nil) = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "usage") {
		t.Errorf("stderr = %q, want usage message", stderr.String())
	}
}

func TestRunHelp(t *testing.T) {
	for _, arg := range []string{"-h", "-help", "--help", "help"} {
		t.Run(arg, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run([]string{arg}, &stdout, &stderr)
			if code != 0 {
				t.Errorf("run(%s) = %d, want 0", arg, code)
			}
			if !strings.Contains(stdout.String(), "usage:") {
				t.Errorf("stdout = %q, want usage message", stdout.String())
			}
		})
	}
}

func TestRunSubcommandHelp(t *testing.T) {
	for _, cmd := range []string{"pull", "push", "diff"} {
		t.Run(cmd, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run([]string{cmd, "-help"}, &stdout, &stderr)
			if code != 0 {
				t.Errorf("run(%s -help) = %d, want 0", cmd, code)
			}
			if !strings.Contains(stdout.String(), "-config") {
				t.Errorf("run(%s -help) stdout = %q, want flag usage", cmd, stdout.String())
			}
		})
	}
}

func TestRunUnknownCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"badcmd"}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("run(badcmd) = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "unknown command") {
		t.Errorf("stderr = %q, want unknown command", stderr.String())
	}
}

func TestRunMissingEnvVars(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"pull"}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("run(missing env) = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "UNIFI_SYNC_URL") {
		t.Errorf("stderr = %q, want env var mention", stderr.String())
	}
}

func TestRunMissingPartialEnvVars(t *testing.T) {
	t.Setenv("UNIFI_SYNC_URL", "http://example.com")
	var stdout, stderr bytes.Buffer
	code := run([]string{"pull"}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("run(partial env) = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "UNIFI_SYNC_USERNAME") {
		t.Errorf("stderr = %q, want missing var mention", stderr.String())
	}
}

func TestRunBadDotenv(t *testing.T) {
	dir := t.TempDir()
	badFile := filepath.Join(dir, ".env")
	if err := os.WriteFile(badFile, []byte("KEY=value"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	os.Chmod(badFile, 0o000) //nolint:errcheck,revive // test setup
	t.Cleanup(func() {
		os.Chmod(badFile, 0o600) //nolint:errcheck,revive // test cleanup
	})

	orig := dotenvPath
	dotenvPath = badFile
	t.Cleanup(func() { dotenvPath = orig })

	var stdout, stderr bytes.Buffer
	code := run([]string{"pull"}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("run(bad .env) = %d, want 2", code)
	}
}

func TestRunBadFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"pull", "-unknown"}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("run(bad flag) = %d, want 2", code)
	}
}

func TestRunLoginError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("unauthorized")) //nolint:errcheck,revive // test handler
	}))
	defer srv.Close()

	setRequiredEnv(t, srv.URL)
	var stdout, stderr bytes.Buffer
	code := run([]string{"pull"}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("run(login error) = %d, want 2", code)
	}
}

func TestRunPull(t *testing.T) {
	srv := httptest.NewServer(loginMux(map[string][]map[string]any{
		"/rest/networkconf": {{"_id": "n1", "name": testNameHomeNet}},
		"/rest/wlanconf":    {},
		"/rest/usergroup":   {},
	}))
	defer srv.Close()

	setRequiredEnv(t, srv.URL)
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := run([]string{"pull", "-config", dir}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run(pull) = %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "pulled networkconf/homenet") {
		t.Errorf("stdout = %q", stdout.String())
	}
}

func TestRunPullError(t *testing.T) {
	srv := httptest.NewServer(loginMux(map[string][]map[string]any{
		"/rest/networkconf": {{"_id": "n1", "name": testNameHomeNet}},
	}))
	defer srv.Close()

	setRequiredEnv(t, srv.URL)
	dir := t.TempDir()
	os.Chmod(dir, 0o555) //nolint:errcheck,gosec,revive // test setup
	t.Cleanup(func() {
		os.Chmod(dir, 0o750) //nolint:errcheck,gosec,revive // test cleanup
	})

	var stdout, stderr bytes.Buffer
	code := run([]string{"pull", "-config", dir, "-type", "networkconf"}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("run(pull error) = %d, want 2", code)
	}
}

func TestRunPush(t *testing.T) {
	srv := httptest.NewServer(loginMux(map[string][]map[string]any{
		"/rest/networkconf": {{"_id": "abc123", "name": testNameHomeNet}},
	}))
	defer srv.Close()

	setRequiredEnv(t, srv.URL)
	dir := t.TempDir()
	if err := writeConfigFile(dir, "networkconf", "homenet", map[string]any{
		"_id": "abc123", "name": testNameHomeNet,
	}); err != nil {
		t.Fatalf("writeConfigFile: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"push", "-config", dir, "-type", "networkconf"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run(push) = %d, stderr: %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "updated networkconf/homenet") {
		t.Errorf("stdout = %q", out)
	}
	if !strings.Contains(out, "verified") {
		t.Errorf("stdout missing verified: %q", out)
	}
}

func TestRunPushDryRun(t *testing.T) {
	srv := httptest.NewServer(loginMux(map[string][]map[string]any{}))
	defer srv.Close()

	setRequiredEnv(t, srv.URL)
	dir := t.TempDir()
	if err := writeConfigFile(dir, "networkconf", "homenet", map[string]any{
		"_id": "abc123", "name": testNameHomeNet,
	}); err != nil {
		t.Fatalf("writeConfigFile: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"push", "-config", dir, "-type", "networkconf", "-dry-run"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run(push -dry-run) = %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "would update") {
		t.Errorf("stdout = %q", stdout.String())
	}
}

func TestRunPushError(t *testing.T) {
	srv := httptest.NewServer(loginMux(map[string][]map[string]any{}))
	defer srv.Close()

	setRequiredEnv(t, srv.URL)
	var stdout, stderr bytes.Buffer
	_ = run([]string{"push", "-config", "/nonexistent/path", "-type", "networkconf"}, &stdout, &stderr)
	code := run([]string{"push", "-type", "badtype"}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("run(push error) = %d, want 2", code)
	}
}

func TestRunPushVerificationDiffs(t *testing.T) {
	srv := httptest.NewServer(loginMux(map[string][]map[string]any{
		"/rest/networkconf": {{"_id": "abc123", "name": testNameHomeNet, "extra": "field"}},
	}))
	defer srv.Close()

	setRequiredEnv(t, srv.URL)
	dir := t.TempDir()
	if err := writeConfigFile(dir, "networkconf", "homenet", map[string]any{
		"_id": "abc123", "name": testNameHomeNet,
	}); err != nil {
		t.Fatalf("writeConfigFile: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"push", "-config", dir, "-type", "networkconf"}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("run(push verification diffs) = %d, want 1, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "push succeeded but verification found differences") {
		t.Errorf("stderr = %q, want verification message", stderr.String())
	}
}

func TestRunDiffNoDifferences(t *testing.T) {
	obj := map[string]any{"_id": "n1", "name": testNameHomeNet}
	srv := httptest.NewServer(loginMux(map[string][]map[string]any{
		"/rest/networkconf": {obj},
	}))
	defer srv.Close()

	setRequiredEnv(t, srv.URL)
	dir := t.TempDir()
	if err := writeConfigFile(dir, "networkconf", "homenet", obj); err != nil {
		t.Fatalf("writeConfigFile: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"diff", "-config", dir, "-type", "networkconf"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run(diff, no diffs) = %d, stderr: %s", code, stderr.String())
	}
}

func TestRunDiffWithDifferences(t *testing.T) {
	srv := httptest.NewServer(loginMux(map[string][]map[string]any{
		"/rest/networkconf": {{"_id": "n1", "name": testNameHomeNet, "vlan": json.Number("10")}},
	}))
	defer srv.Close()

	setRequiredEnv(t, srv.URL)
	dir := t.TempDir()
	if err := writeConfigFile(dir, "networkconf", "homenet", map[string]any{
		"_id": "n1", "name": testNameHomeNet, "vlan": json.Number("20"),
	}); err != nil {
		t.Fatalf("writeConfigFile: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"diff", "-config", dir, "-type", "networkconf"}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("run(diff, with diffs) = %d, want 1", code)
	}
}

func TestRunDiffError(t *testing.T) {
	srv := httptest.NewServer(loginMux(map[string][]map[string]any{}))
	defer srv.Close()

	setRequiredEnv(t, srv.URL)
	var stdout, stderr bytes.Buffer
	code := run([]string{"diff", "-type", "badtype"}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("run(diff error) = %d, want 2", code)
	}
}

// diffOutputHasANSI runs `diff` against a server that differs from local and
// reports whether stdout contained ANSI color codes. Callers set TERM/NO_COLOR
// and the terminal state before calling.
func diffOutputHasANSI(t *testing.T) bool {
	t.Helper()
	srv := httptest.NewServer(loginMux(map[string][]map[string]any{
		"/rest/networkconf": {{"_id": "n1", "name": testNameHomeNet, "vlan": json.Number("10")}},
	}))
	defer srv.Close()

	setRequiredEnv(t, srv.URL)
	dir := t.TempDir()
	if err := writeConfigFile(dir, "networkconf", "homenet", map[string]any{
		"_id": "n1", "name": testNameHomeNet, "vlan": json.Number("20"),
	}); err != nil {
		t.Fatalf("writeConfigFile: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"diff", "-config", dir, "-type", "networkconf"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run(diff) = %d, want 1, stderr: %s", code, stderr.String())
	}
	return strings.Contains(stdout.String(), "\033[")
}

func TestRunDiffColor(t *testing.T) {
	setTerminal(t, true)
	t.Setenv("TERM", "xterm")
	t.Setenv("NO_COLOR", "")
	if !diffOutputHasANSI(t) {
		t.Error("expected ANSI color on a terminal with TERM set")
	}
}

func TestRunDiffNonTerminalSuppressesColor(t *testing.T) {
	// TERM set and NO_COLOR unset, but stdout is not a terminal -> no color.
	setTerminal(t, false)
	t.Setenv("TERM", "xterm")
	t.Setenv("NO_COLOR", "")
	if diffOutputHasANSI(t) {
		t.Error("expected no ANSI color when stdout is not a terminal")
	}
}

func TestRunDiffNoColor(t *testing.T) {
	// On a terminal, NO_COLOR alone must still disable color.
	setTerminal(t, true)
	t.Setenv("NO_COLOR", "1")
	t.Setenv("TERM", "xterm")
	if diffOutputHasANSI(t) {
		t.Error("expected no ANSI color when NO_COLOR is set")
	}
}

func TestIsTerminal(t *testing.T) {
	// A bytes.Buffer is not an *os.File.
	if isTerminal(&bytes.Buffer{}) {
		t.Error("isTerminal(buffer) = true, want false")
	}
	// A typed-nil *os.File must report false without panicking.
	if isTerminal((*os.File)(nil)) {
		t.Error("isTerminal(nil *os.File) = true, want false")
	}
	// A regular file is not a character device.
	f, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer f.Close() //nolint:errcheck // test cleanup
	if isTerminal(f) {
		t.Error("isTerminal(regular file) = true, want false")
	}
	// A closed file cannot be Stat'd.
	closed, err := os.CreateTemp(t.TempDir(), "closed")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	_ = closed.Close() //nolint:errcheck // intentionally closed to force a Stat error below
	if isTerminal(closed) {
		t.Error("isTerminal(closed file) = true, want false")
	}
	// A character device (os.DevNull) reports as a terminal under this stdlib heuristic.
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	defer devNull.Close() //nolint:errcheck // test cleanup
	if !isTerminal(devNull) {
		t.Errorf("isTerminal(%s) = false, want true", os.DevNull)
	}
}

func TestRunSiteEnv(t *testing.T) {
	var requestedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == testAPILogin {
			w.WriteHeader(http.StatusOK)
			return
		}
		requestedPath = r.URL.Path
		w.Write(unifiResponse([]map[string]any{})) //nolint:errcheck,revive // test handler
	}))
	defer srv.Close()

	setRequiredEnv(t, srv.URL)
	t.Setenv("UNIFI_SYNC_SITE", "mysite")

	var stdout, stderr bytes.Buffer
	code := run([]string{"pull", "-type", "networkconf"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run(pull, custom site) = %d, stderr: %s", code, stderr.String())
	}
	if requestedPath != "/api/s/mysite/rest/networkconf" {
		t.Errorf("path = %q, want /api/s/mysite/rest/networkconf", requestedPath)
	}
}
