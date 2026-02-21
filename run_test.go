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
	t.Setenv("UNIFI_URL", url)
	t.Setenv("UNIFI_USERNAME", "admin")
	t.Setenv("UNIFI_PASSWORD", "pass")
}

func loginMux(responses map[string][]map[string]any) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/login" {
			w.WriteHeader(http.StatusOK)
			return
		}
		for path, data := range responses {
			if strings.HasSuffix(r.URL.Path, path) {
				w.Write(unifiResponse(data))
				return
			}
		}
		if r.Method == http.MethodPut || r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			var obj map[string]any
			json.Unmarshal(body, &obj)
			w.Write(unifiResponse([]map[string]any{obj}))
			return
		}
		w.Write(unifiResponse([]map[string]any{}))
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
	if !strings.Contains(stderr.String(), "UNIFI_URL") {
		t.Errorf("stderr = %q, want env var mention", stderr.String())
	}
}

func TestRunMissingPartialEnvVars(t *testing.T) {
	t.Setenv("UNIFI_URL", "http://example.com")
	var stdout, stderr bytes.Buffer
	code := run([]string{"pull"}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("run(partial env) = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "UNIFI_USERNAME") {
		t.Errorf("stderr = %q, want missing var mention", stderr.String())
	}
}

func TestRunBadDotenv(t *testing.T) {
	dir := t.TempDir()
	badFile := filepath.Join(dir, ".env")
	os.WriteFile(badFile, []byte("KEY=value"), 0o644)
	os.Chmod(badFile, 0o000)
	t.Cleanup(func() { os.Chmod(badFile, 0o644) })

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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("unauthorized"))
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
		"/rest/networkconf": {{"_id": "n1", "name": "HomeNet"}},
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
		"/rest/networkconf": {{"_id": "n1", "name": "HomeNet"}},
	}))
	defer srv.Close()

	setRequiredEnv(t, srv.URL)
	dir := t.TempDir()
	os.Chmod(dir, 0o555)
	t.Cleanup(func() { os.Chmod(dir, 0o755) })

	var stdout, stderr bytes.Buffer
	code := run([]string{"pull", "-config", dir, "-type", "networkconf"}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("run(pull error) = %d, want 2", code)
	}
}

func TestRunPush(t *testing.T) {
	srv := httptest.NewServer(loginMux(map[string][]map[string]any{
		"/rest/networkconf": {{"_id": "abc123", "name": "HomeNet"}},
	}))
	defer srv.Close()

	setRequiredEnv(t, srv.URL)
	dir := t.TempDir()
	writeConfigFile(dir, "networkconf", "homenet", map[string]any{
		"_id": "abc123", "name": "HomeNet",
	})

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
	writeConfigFile(dir, "networkconf", "homenet", map[string]any{
		"_id": "abc123", "name": "HomeNet",
	})

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
	code := run([]string{"push", "-config", "/nonexistent/path", "-type", "networkconf"}, &stdout, &stderr)
	// push with nonexistent config dir returns 0 (empty dir = nothing to push)
	// instead test with bad type
	code = run([]string{"push", "-type", "badtype"}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("run(push error) = %d, want 2", code)
	}
}

func TestRunPushVerificationDiffs(t *testing.T) {
	srv := httptest.NewServer(loginMux(map[string][]map[string]any{
		"/rest/networkconf": {{"_id": "abc123", "name": "HomeNet", "extra": "field"}},
	}))
	defer srv.Close()

	setRequiredEnv(t, srv.URL)
	dir := t.TempDir()
	writeConfigFile(dir, "networkconf", "homenet", map[string]any{
		"_id": "abc123", "name": "HomeNet",
	})

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
	obj := map[string]any{"_id": "n1", "name": "HomeNet"}
	srv := httptest.NewServer(loginMux(map[string][]map[string]any{
		"/rest/networkconf": {obj},
	}))
	defer srv.Close()

	setRequiredEnv(t, srv.URL)
	dir := t.TempDir()
	writeConfigFile(dir, "networkconf", "homenet", obj)

	var stdout, stderr bytes.Buffer
	code := run([]string{"diff", "-config", dir, "-type", "networkconf"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run(diff, no diffs) = %d, stderr: %s", code, stderr.String())
	}
}

func TestRunDiffWithDifferences(t *testing.T) {
	srv := httptest.NewServer(loginMux(map[string][]map[string]any{
		"/rest/networkconf": {{"_id": "n1", "name": "HomeNet", "vlan": json.Number("10")}},
	}))
	defer srv.Close()

	setRequiredEnv(t, srv.URL)
	dir := t.TempDir()
	writeConfigFile(dir, "networkconf", "homenet", map[string]any{
		"_id": "n1", "name": "HomeNet", "vlan": json.Number("20"),
	})

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
		"/rest/networkconf": {{"_id": "n1", "name": "HomeNet", "vlan": json.Number("10")}},
	}))
	defer srv.Close()

	setRequiredEnv(t, srv.URL)
	t.Setenv("TERM", "xterm")
	dir := t.TempDir()
	writeConfigFile(dir, "networkconf", "homenet", map[string]any{
		"_id": "n1", "name": "HomeNet", "vlan": json.Number("20"),
	})

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
		"/rest/networkconf": {{"_id": "n1", "name": "HomeNet", "vlan": json.Number("10")}},
	}))
	defer srv.Close()

	setRequiredEnv(t, srv.URL)
	t.Setenv("NO_COLOR", "1")
	t.Setenv("TERM", "xterm")
	dir := t.TempDir()
	writeConfigFile(dir, "networkconf", "homenet", map[string]any{
		"_id": "n1", "name": "HomeNet", "vlan": json.Number("20"),
	})

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
		if r.URL.Path == "/api/login" {
			w.WriteHeader(http.StatusOK)
			return
		}
		requestedPath = r.URL.Path
		w.Write(unifiResponse([]map[string]any{}))
	}))
	defer srv.Close()

	setRequiredEnv(t, srv.URL)
	t.Setenv("UNIFI_SITE", "mysite")

	var stdout, stderr bytes.Buffer
	code := run([]string{"pull", "-type", "networkconf"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run(pull, custom site) = %d, stderr: %s", code, stderr.String())
	}
	if requestedPath != "/api/s/mysite/rest/networkconf" {
		t.Errorf("path = %q, want /api/s/mysite/rest/networkconf", requestedPath)
	}
}
