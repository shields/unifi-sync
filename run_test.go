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

func TestRunDiffColor(t *testing.T) {
	srv := httptest.NewServer(loginMux(map[string][]map[string]any{
		"/rest/networkconf": {{"_id": "n1", "name": testNameHomeNet, "vlan": json.Number("10")}},
	}))
	defer srv.Close()

	setRequiredEnv(t, srv.URL)
	t.Setenv("TERM", "xterm")
	dir := t.TempDir()
	if err := writeConfigFile(dir, "networkconf", "homenet", map[string]any{
		"_id": "n1", "name": testNameHomeNet, "vlan": json.Number("20"),
	}); err != nil {
		t.Fatalf("writeConfigFile: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"diff", "-config", dir, "-type", "networkconf"}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("run(diff color) = %d, want 1", code)
	}
	if !strings.Contains(stdout.String(), "\033[") {
		t.Error("output should contain ANSI escape codes when TERM is set")
	}
}

func TestRunDiffNoColor(t *testing.T) {
	srv := httptest.NewServer(loginMux(map[string][]map[string]any{
		"/rest/networkconf": {{"_id": "n1", "name": testNameHomeNet, "vlan": json.Number("10")}},
	}))
	defer srv.Close()

	setRequiredEnv(t, srv.URL)
	t.Setenv("NO_COLOR", "1")
	t.Setenv("TERM", "xterm")
	dir := t.TempDir()
	if err := writeConfigFile(dir, "networkconf", "homenet", map[string]any{
		"_id": "n1", "name": testNameHomeNet, "vlan": json.Number("20"),
	}); err != nil {
		t.Fatalf("writeConfigFile: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := run([]string{"diff", "-config", dir, "-type", "networkconf"}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("run(diff no-color) = %d, want 1", code)
	}
	if strings.Contains(stdout.String(), "\033[") {
		t.Error("output should NOT contain ANSI when NO_COLOR is set")
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
