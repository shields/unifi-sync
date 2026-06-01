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
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSelectedTypesAll(t *testing.T) {
	types, err := selectedTypes("")
	if err != nil {
		t.Fatalf("selectedTypes() error = %v", err)
	}
	if len(types) != 3 {
		t.Fatalf("len = %d, want 3", len(types))
	}
}

func TestSelectedTypesFiltered(t *testing.T) {
	types, err := selectedTypes("wlanconf")
	if err != nil {
		t.Fatalf("selectedTypes() error = %v", err)
	}
	if len(types) != 1 || types[0] != "wlanconf" {
		t.Errorf("types = %v, want [wlanconf]", types)
	}
}

func TestSelectedTypesInvalid(t *testing.T) {
	_, err := selectedTypes("badtype")
	if err == nil {
		t.Error("selectedTypes(badtype) should return error")
	}
}

func testMux(responses map[string][]map[string]any) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func TestCmdPull(t *testing.T) {
	srv := httptest.NewServer(testMux(map[string][]map[string]any{
		"/rest/networkconf": {{"_id": "n1", "name": testNameHomeNet, "vlan": json.Number("10")}},
		"/rest/wlanconf":    {{"_id": "w1", "name": "Guest WiFi", "x_passphrase": "secret"}},
		"/rest/usergroup":   {{"_id": "u1", "name": "Default"}},
	}))
	defer srv.Close()

	dir := t.TempDir()
	c := newClient(srv.URL, false)
	var buf bytes.Buffer
	err := cmdPull(context.Background(), c, "default", dir, "", &buf)
	if err != nil {
		t.Fatalf("cmdPull() error = %v", err)
	}

	homenet, err := readConfigFile(filepath.Join(dir, "networkconf", "homenet.json"))
	if err != nil {
		t.Fatalf("readConfigFile(homenet) error = %v", err)
	}
	if homenet["name"] != testNameHomeNet {
		t.Errorf("name = %v", homenet["name"])
	}

	wifi, err := readConfigFile(filepath.Join(dir, "wlanconf", "guest-wifi.json"))
	if err != nil {
		t.Fatalf("readConfigFile(guest-wifi) error = %v", err)
	}
	if wifi["x_passphrase"] != redactedValue {
		t.Errorf("x_passphrase = %v, want %q", wifi["x_passphrase"], redactedValue)
	}

	out := buf.String()
	if !strings.Contains(out, "pulled networkconf/homenet") {
		t.Errorf("output missing homenet: %q", out)
	}
	if !strings.Contains(out, "pulled wlanconf/guest-wifi") {
		t.Errorf("output missing guest-wifi: %q", out)
	}
}

func TestCmdPullTypeFilter(t *testing.T) {
	called := map[string]bool{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called[r.URL.Path] = true
		w.Write(unifiResponse([]map[string]any{})) //nolint:errcheck,revive // test handler
	}))
	defer srv.Close()

	dir := t.TempDir()
	c := newClient(srv.URL, false)
	var buf bytes.Buffer
	err := cmdPull(context.Background(), c, "default", dir, "networkconf", &buf)
	if err != nil {
		t.Fatalf("cmdPull() error = %v", err)
	}
	if !called[testAPINetworkconf] {
		t.Error("networkconf not fetched")
	}
	if called["/api/s/default/rest/wlanconf"] {
		t.Error("wlanconf should not be fetched with filter")
	}
}

func TestCmdPullSkipsNoName(t *testing.T) {
	srv := httptest.NewServer(testMux(map[string][]map[string]any{
		"/rest/networkconf": {{"_id": "n1"}},
		"/rest/wlanconf":    {},
		"/rest/usergroup":   {},
	}))
	defer srv.Close()

	dir := t.TempDir()
	c := newClient(srv.URL, false)
	var buf bytes.Buffer
	err := cmdPull(context.Background(), c, "default", dir, "", &buf)
	if err != nil {
		t.Fatalf("cmdPull() error = %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("output should be empty for nameless items, got %q", buf.String())
	}
}

func TestCmdPullSkipsEmptySlug(t *testing.T) {
	srv := httptest.NewServer(testMux(map[string][]map[string]any{
		"/rest/networkconf": {{"_id": "n1", "name": "!!!"}},
	}))
	defer srv.Close()

	dir := t.TempDir()
	c := newClient(srv.URL, false)
	var buf bytes.Buffer
	if err := cmdPull(context.Background(), c, "default", dir, "networkconf", &buf); err != nil {
		t.Fatalf("cmdPull() error = %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("output should be empty for unsluggable name, got %q", buf.String())
	}
	// The name slugifies to "", which must not be written as a hidden ".json".
	if _, err := os.Stat(filepath.Join(dir, "networkconf", ".json")); !os.IsNotExist(err) {
		t.Errorf(".json file should not be created, stat err = %v", err)
	}
}

func TestCmdPullSlugCollision(t *testing.T) {
	srv := httptest.NewServer(testMux(map[string][]map[string]any{
		"/rest/networkconf": {
			{"_id": "n1", "name": "Guest WiFi"},
			{"_id": "n2", "name": "Guest-WiFi"},
		},
	}))
	defer srv.Close()

	c := newClient(srv.URL, false)
	err := cmdPull(context.Background(), c, "default", t.TempDir(), "networkconf", io.Discard)
	if err == nil {
		t.Error("cmdPull() should return error on slug collision")
	}
	if !strings.Contains(err.Error(), "slug collision") {
		t.Errorf("error = %v, want slug collision", err)
	}
}

func TestCmdPullListError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error")) //nolint:errcheck,revive // test handler
	}))
	defer srv.Close()

	c := newClient(srv.URL, false)
	err := cmdPull(context.Background(), c, "default", t.TempDir(), "networkconf", io.Discard)
	if err == nil {
		t.Error("cmdPull() should return error on list failure")
	}
}

func TestCmdPullWriteError(t *testing.T) {
	srv := httptest.NewServer(testMux(map[string][]map[string]any{
		"/rest/networkconf": {{"_id": "n1", "name": testNameHomeNet}},
	}))
	defer srv.Close()

	dir := t.TempDir()
	os.Chmod(dir, 0o555) //nolint:errcheck,gosec,revive // test setup
	t.Cleanup(func() {
		os.Chmod(dir, 0o750) //nolint:errcheck,gosec,revive // test cleanup
	})

	c := newClient(srv.URL, false)
	err := cmdPull(context.Background(), c, "default", dir, "networkconf", io.Discard)
	if err == nil {
		t.Error("cmdPull() should return error on write failure")
	}
}

func TestCmdPullInvalidType(t *testing.T) {
	c := newClient("http://example.com", false)
	err := cmdPull(context.Background(), c, "default", t.TempDir(), "badtype", io.Discard)
	if err == nil {
		t.Error("cmdPull(badtype) should return error")
	}
}

func TestCmdPushUpdate(t *testing.T) {
	var putPath string
	var putBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			putPath = r.URL.Path
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("ReadAll: %v", err)
			}
			_ = json.Unmarshal(body, &putBody)                //nolint:errcheck // test handler
			w.Write(unifiResponse([]map[string]any{putBody})) //nolint:errcheck,revive // test handler
			return
		}
		resp := []map[string]any{{"_id": "abc123", "name": testNameHomeNet}}
		w.Write(unifiResponse(resp)) //nolint:errcheck,revive // test handler
	}))
	defer srv.Close()

	dir := t.TempDir()
	if err := writeConfigFile(dir, "networkconf", "homenet", map[string]any{
		"_id": "abc123", "name": testNameHomeNet,
	}); err != nil {
		t.Fatalf("writeConfigFile: %v", err)
	}

	c := newClient(srv.URL, false)
	var buf bytes.Buffer
	hasDiffs, err := cmdPush(context.Background(), c, "default", dir, "networkconf", false, false, &buf)
	if err != nil {
		t.Fatalf("cmdPush() error = %v", err)
	}
	if hasDiffs {
		t.Error("hasDiffs = true, want false")
	}
	if putPath != testAPINetworkconfID {
		t.Errorf("PUT path = %q", putPath)
	}
	out := buf.String()
	if !strings.Contains(out, "updated networkconf/homenet") {
		t.Errorf("output = %q", out)
	}
	if !strings.Contains(out, "verifying...") || !strings.Contains(out, "verified") {
		t.Errorf("output missing verification messages: %q", out)
	}
}

func TestCmdPushCreate(t *testing.T) {
	var postPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			postPath = r.URL.Path
		}
		resp := []map[string]any{{"_id": "new1", "name": testNameHomeNet}}
		w.Write(unifiResponse(resp)) //nolint:errcheck,revive // test handler
	}))
	defer srv.Close()

	dir := t.TempDir()
	if err := writeConfigFile(dir, "networkconf", "homenet", map[string]any{
		"name": testNameHomeNet,
	}); err != nil {
		t.Fatalf("writeConfigFile: %v", err)
	}

	c := newClient(srv.URL, false)
	var buf bytes.Buffer
	hasDiffs, err := cmdPush(context.Background(), c, "default", dir, "networkconf", false, false, &buf)
	if err != nil {
		t.Fatalf("cmdPush() error = %v", err)
	}
	if hasDiffs {
		t.Error("hasDiffs = true, want false")
	}
	if postPath != testAPINetworkconf {
		t.Errorf("POST path = %q", postPath)
	}
	if !strings.Contains(buf.String(), "created networkconf/homenet") {
		t.Errorf("output = %q", buf.String())
	}

	updated, err := readConfigFile(filepath.Join(dir, "networkconf", "homenet.json"))
	if err != nil {
		t.Fatalf("readConfigFile() error = %v", err)
	}
	if updated["_id"] != "new1" {
		t.Errorf("_id = %v, want new1 (written back from server)", updated["_id"])
	}
}

func TestCmdPushCreatePreservesRedacted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := []map[string]any{{"_id": "new1", "name": "Guest WiFi"}}
		w.Write(unifiResponse(resp)) //nolint:errcheck,revive // test handler
	}))
	defer srv.Close()

	dir := t.TempDir()
	if err := writeConfigFile(dir, "wlanconf", "guest-wifi", map[string]any{
		"name": "Guest WiFi", "x_passphrase": redactedValue,
	}); err != nil {
		t.Fatalf("writeConfigFile: %v", err)
	}
	t.Setenv("UNIFI_SYNC_SECRET_GUEST_WIFI_X_PASSPHRASE", "mypass")

	c := newClient(srv.URL, false)
	_, err := cmdPush(context.Background(), c, "default", dir, "wlanconf", false, false, io.Discard)
	if err != nil {
		t.Fatalf("cmdPush() error = %v", err)
	}

	written, err := readConfigFile(filepath.Join(dir, "wlanconf", "guest-wifi.json"))
	if err != nil {
		t.Fatalf("readConfigFile() error = %v", err)
	}
	if written["_id"] != "new1" {
		t.Errorf("_id = %v, want new1", written["_id"])
	}
	if written["x_passphrase"] != redactedValue {
		t.Errorf("x_passphrase = %v, want %q (preserved)", written["x_passphrase"], redactedValue)
	}
}

func TestCmdPushCreatePreservesRedactedNested(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := []map[string]any{{"_id": "new1", "name": "Guest WiFi"}}
		w.Write(unifiResponse(resp)) //nolint:errcheck,revive // test handler
	}))
	defer srv.Close()

	dir := t.TempDir()
	if err := writeConfigFile(dir, "wlanconf", "guest-wifi", map[string]any{
		"name": "Guest WiFi",
		"private_preshared_keys": []any{
			map[string]any{"networkconf_id": "net0", "password": redactedValue},
		},
	}); err != nil {
		t.Fatalf("writeConfigFile: %v", err)
	}
	t.Setenv("UNIFI_SYNC_SECRET_GUEST_WIFI_PRIVATE_PRESHARED_KEYS_0_PASSWORD", "realpass")

	c := newClient(srv.URL, false)
	if _, err := cmdPush(context.Background(), c, "default", dir, "wlanconf", false, false, io.Discard); err != nil {
		t.Fatalf("cmdPush() error = %v", err)
	}

	written, err := readConfigFile(filepath.Join(dir, "wlanconf", "guest-wifi.json"))
	if err != nil {
		t.Fatalf("readConfigFile() error = %v", err)
	}
	// The injected cleartext must not be written back to disk: the deep copy
	// keeps the original (and therefore the file) redacted.
	if got := nestedElem(t, written, "private_preshared_keys", 0)["password"]; got != redactedValue {
		t.Errorf("written nested password = %v, want %q (injected secret leaked to disk)", got, redactedValue)
	}
}

func TestCmdPushCreateWriteBackError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := []map[string]any{{"_id": "new1", "name": testNameHomeNet}}
		w.Write(unifiResponse(resp)) //nolint:errcheck,revive // test handler
	}))
	defer srv.Close()

	dir := t.TempDir()
	if err := writeConfigFile(dir, "networkconf", "homenet", map[string]any{"name": testNameHomeNet}); err != nil {
		t.Fatalf("writeConfigFile: %v", err)
	}

	filePath := filepath.Join(dir, "networkconf", "homenet.json")
	os.Chmod(filePath, 0o444) //nolint:errcheck,gosec,revive // test setup
	t.Cleanup(func() {
		os.Chmod(filePath, 0o600) //nolint:errcheck,revive // test cleanup
	})

	c := newClient(srv.URL, false)
	_, err := cmdPush(context.Background(), c, "default", dir, "networkconf", false, false, io.Discard)
	if err == nil {
		t.Error("cmdPush() should return error on write-back failure")
	}
}

func TestCmdPushDryRunUpdate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("no HTTP calls expected during dry-run")
	}))
	defer srv.Close()

	dir := t.TempDir()
	if err := writeConfigFile(dir, "networkconf", "homenet", map[string]any{
		"_id": "abc123", "name": testNameHomeNet,
	}); err != nil {
		t.Fatalf("writeConfigFile: %v", err)
	}

	c := newClient(srv.URL, false)
	var buf bytes.Buffer
	hasDiffs, err := cmdPush(context.Background(), c, "default", dir, "networkconf", true, false, &buf)
	if err != nil {
		t.Fatalf("cmdPush() error = %v", err)
	}
	if hasDiffs {
		t.Error("hasDiffs = true, want false for dry-run")
	}
	out := buf.String()
	if !strings.Contains(out, "would update networkconf/homenet") {
		t.Errorf("output = %q", out)
	}
	if strings.Contains(out, "verifying") {
		t.Errorf("dry-run should skip verification, got %q", out)
	}
}

func TestCmdPushDryRunCreate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("no HTTP calls expected during dry-run")
	}))
	defer srv.Close()

	dir := t.TempDir()
	if err := writeConfigFile(dir, "networkconf", "homenet", map[string]any{
		"name": testNameHomeNet,
	}); err != nil {
		t.Fatalf("writeConfigFile: %v", err)
	}

	c := newClient(srv.URL, false)
	var buf bytes.Buffer
	hasDiffs, err := cmdPush(context.Background(), c, "default", dir, "networkconf", true, false, &buf)
	if err != nil {
		t.Fatalf("cmdPush() error = %v", err)
	}
	if hasDiffs {
		t.Error("hasDiffs = true, want false for dry-run")
	}
	if !strings.Contains(buf.String(), "would create networkconf/homenet") {
		t.Errorf("output = %q", buf.String())
	}
}

func TestCmdPushInjectSecrets(t *testing.T) {
	var putBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("ReadAll: %v", err)
			}
			_ = json.Unmarshal(body, &putBody)                //nolint:errcheck // test handler
			w.Write(unifiResponse([]map[string]any{putBody})) //nolint:errcheck,revive // test handler
			return
		}
		w.Write(unifiResponse([]map[string]any{ //nolint:errcheck,revive // test handler
			{"_id": "w1", "name": "Guest WiFi", "x_passphrase": "mysecretpass"},
		}))
	}))
	defer srv.Close()

	dir := t.TempDir()
	if err := writeConfigFile(dir, "wlanconf", "guest-wifi", map[string]any{
		"_id": "w1", "name": "Guest WiFi", "x_passphrase": redactedValue,
	}); err != nil {
		t.Fatalf("writeConfigFile: %v", err)
	}

	t.Setenv("UNIFI_SYNC_SECRET_GUEST_WIFI_X_PASSPHRASE", "mysecretpass")

	c := newClient(srv.URL, false)
	_, err := cmdPush(context.Background(), c, "default", dir, "wlanconf", false, false, io.Discard)
	if err != nil {
		t.Fatalf("cmdPush() error = %v", err)
	}
	if putBody["x_passphrase"] != "mysecretpass" {
		t.Errorf("x_passphrase = %v, want mysecretpass", putBody["x_passphrase"])
	}
}

func TestCmdPushMissingSecret(t *testing.T) {
	dir := t.TempDir()
	if err := writeConfigFile(dir, "wlanconf", "guest-wifi", map[string]any{
		"_id": "w1", "name": "Guest WiFi", "x_passphrase": redactedValue,
	}); err != nil {
		t.Fatalf("writeConfigFile: %v", err)
	}

	c := newClient("http://example.com", false)
	_, err := cmdPush(context.Background(), c, "default", dir, "wlanconf", false, false, io.Discard)
	if err == nil {
		t.Error("cmdPush() should return error for missing secret env var")
	}
}

func TestCmdPushPutError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("forbidden")) //nolint:errcheck,revive // test handler
	}))
	defer srv.Close()

	dir := t.TempDir()
	if err := writeConfigFile(dir, "networkconf", "homenet", map[string]any{
		"_id": "abc123", "name": testNameHomeNet,
	}); err != nil {
		t.Fatalf("writeConfigFile: %v", err)
	}

	c := newClient(srv.URL, false)
	_, err := cmdPush(context.Background(), c, "default", dir, "networkconf", false, false, io.Discard)
	if err == nil {
		t.Error("cmdPush() should return error on PUT failure")
	}
}

func TestCmdPushPostError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad request")) //nolint:errcheck,revive // test handler
	}))
	defer srv.Close()

	dir := t.TempDir()
	if err := writeConfigFile(dir, "networkconf", "homenet", map[string]any{
		"name": testNameHomeNet,
	}); err != nil {
		t.Fatalf("writeConfigFile: %v", err)
	}

	c := newClient(srv.URL, false)
	_, err := cmdPush(context.Background(), c, "default", dir, "networkconf", false, false, io.Discard)
	if err == nil {
		t.Error("cmdPush() should return error on POST failure")
	}
}

func TestCmdPushInvalidType(t *testing.T) {
	c := newClient("http://example.com", false)
	_, err := cmdPush(context.Background(), c, "default", t.TempDir(), "badtype", false, false, io.Discard)
	if err == nil {
		t.Error("cmdPush(badtype) should return error")
	}
}

func TestCmdPushReadError(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "networkconf")
	if err := os.MkdirAll(subdir, 0o750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	os.Chmod(subdir, 0o000) //nolint:errcheck,revive // test setup
	t.Cleanup(func() {
		os.Chmod(subdir, 0o750) //nolint:errcheck,gosec,revive // test cleanup
	})

	c := newClient("http://example.com", false)
	_, err := cmdPush(context.Background(), c, "default", dir, "networkconf", false, false, io.Discard)
	if err == nil {
		t.Error("cmdPush() should return error on read failure")
	}
}

func TestCmdPushEmptyDir(t *testing.T) {
	srv := httptest.NewServer(testMux(map[string][]map[string]any{
		"/rest/networkconf": {},
	}))
	defer srv.Close()

	dir := t.TempDir()
	c := newClient(srv.URL, false)
	var buf bytes.Buffer
	hasDiffs, err := cmdPush(context.Background(), c, "default", dir, "networkconf", false, false, &buf)
	if err != nil {
		t.Fatalf("cmdPush() error = %v", err)
	}
	if hasDiffs {
		t.Error("hasDiffs = true, want false")
	}
	out := buf.String()
	if !strings.Contains(out, "verifying...") || !strings.Contains(out, "verified") {
		t.Errorf("output = %q, want verifying/verified messages", out)
	}
}

func TestCmdPushVerificationDiffs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			w.Write(unifiResponse([]map[string]any{})) //nolint:errcheck,revive // test handler
			return
		}
		w.Write(unifiResponse([]map[string]any{ //nolint:errcheck,revive // test handler
			{"_id": "abc123", "name": testNameHomeNet, "extra_field": "added-by-controller"},
		}))
	}))
	defer srv.Close()

	dir := t.TempDir()
	if err := writeConfigFile(dir, "networkconf", "homenet", map[string]any{
		"_id": "abc123", "name": testNameHomeNet,
	}); err != nil {
		t.Fatalf("writeConfigFile: %v", err)
	}

	c := newClient(srv.URL, false)
	var buf bytes.Buffer
	hasDiffs, err := cmdPush(context.Background(), c, "default", dir, "networkconf", false, false, &buf)
	if err != nil {
		t.Fatalf("cmdPush() error = %v", err)
	}
	if !hasDiffs {
		t.Error("hasDiffs = false, want true")
	}
	out := buf.String()
	if !strings.Contains(out, "verifying...") {
		t.Errorf("output missing verifying: %q", out)
	}
	if strings.Contains(out, "verified") {
		t.Errorf("output should not contain verified when diffs found: %q", out)
	}
	if !strings.Contains(out, "---") || !strings.Contains(out, "+++") {
		t.Errorf("output missing diff markers: %q", out)
	}
}

func TestCmdPushVerificationError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			w.Write(unifiResponse([]map[string]any{})) //nolint:errcheck,revive // test handler
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error")) //nolint:errcheck,revive // test handler
	}))
	defer srv.Close()

	dir := t.TempDir()
	if err := writeConfigFile(dir, "networkconf", "homenet", map[string]any{
		"_id": "abc123", "name": testNameHomeNet,
	}); err != nil {
		t.Fatalf("writeConfigFile: %v", err)
	}

	c := newClient(srv.URL, false)
	_, err := cmdPush(context.Background(), c, "default", dir, "networkconf", false, false, io.Discard)
	if err == nil {
		t.Fatal("cmdPush() should return error on verification failure")
	}
	if !strings.Contains(err.Error(), "post-push verification:") {
		t.Errorf("error = %v, want post-push verification prefix", err)
	}
}

func TestCmdDiffNoDifferences(t *testing.T) {
	obj := map[string]any{"_id": "n1", "name": testNameHomeNet, "vlan": json.Number("10")}

	srv := httptest.NewServer(testMux(map[string][]map[string]any{
		"/rest/networkconf": {obj},
	}))
	defer srv.Close()

	dir := t.TempDir()
	if err := writeConfigFile(dir, "networkconf", "homenet", obj); err != nil {
		t.Fatalf("writeConfigFile: %v", err)
	}

	c := newClient(srv.URL, false)
	var buf bytes.Buffer
	hasDiffs, err := cmdDiff(context.Background(), c, "default", dir, "networkconf", false, &buf)
	if err != nil {
		t.Fatalf("cmdDiff() error = %v", err)
	}
	if hasDiffs {
		t.Error("hasDiffs = true, want false")
	}
	if buf.Len() != 0 {
		t.Errorf("output should be empty, got %q", buf.String())
	}
}

func TestCmdDiffWithDifferences(t *testing.T) {
	remote := map[string]any{"_id": "n1", "name": testNameHomeNet, "vlan": json.Number("10")}
	local := map[string]any{"_id": "n1", "name": testNameHomeNet, "vlan": json.Number("20")}

	srv := httptest.NewServer(testMux(map[string][]map[string]any{
		"/rest/networkconf": {remote},
	}))
	defer srv.Close()

	dir := t.TempDir()
	if err := writeConfigFile(dir, "networkconf", "homenet", local); err != nil {
		t.Fatalf("writeConfigFile: %v", err)
	}

	c := newClient(srv.URL, false)
	var buf bytes.Buffer
	hasDiffs, err := cmdDiff(context.Background(), c, "default", dir, "networkconf", false, &buf)
	if err != nil {
		t.Fatalf("cmdDiff() error = %v", err)
	}
	if !hasDiffs {
		t.Error("hasDiffs = false, want true")
	}
	out := buf.String()
	if !strings.Contains(out, "---") || !strings.Contains(out, "+++") {
		t.Errorf("output missing diff headers: %q", out)
	}
}

func TestCmdDiffLocalOnly(t *testing.T) {
	srv := httptest.NewServer(testMux(map[string][]map[string]any{
		"/rest/networkconf": {},
	}))
	defer srv.Close()

	dir := t.TempDir()
	if err := writeConfigFile(dir, "networkconf", "homenet", map[string]any{
		"_id": "n1", "name": testNameHomeNet,
	}); err != nil {
		t.Fatalf("writeConfigFile: %v", err)
	}

	c := newClient(srv.URL, false)
	var buf bytes.Buffer
	hasDiffs, err := cmdDiff(context.Background(), c, "default", dir, "networkconf", false, &buf)
	if err != nil {
		t.Fatalf("cmdDiff() error = %v", err)
	}
	if !hasDiffs {
		t.Error("hasDiffs = false, want true for local-only resource")
	}
	if !strings.Contains(buf.String(), "+") {
		t.Errorf("output should contain additions: %q", buf.String())
	}
}

func TestCmdDiffRemoteOnly(t *testing.T) {
	srv := httptest.NewServer(testMux(map[string][]map[string]any{
		"/rest/networkconf": {{"_id": "n1", "name": testNameHomeNet}},
	}))
	defer srv.Close()

	dir := t.TempDir()

	c := newClient(srv.URL, false)
	var buf bytes.Buffer
	hasDiffs, err := cmdDiff(context.Background(), c, "default", dir, "networkconf", false, &buf)
	if err != nil {
		t.Fatalf("cmdDiff() error = %v", err)
	}
	if !hasDiffs {
		t.Error("hasDiffs = false, want true for remote-only resource")
	}
	if !strings.Contains(buf.String(), "-") {
		t.Errorf("output should contain deletions: %q", buf.String())
	}
}

func TestCmdDiffSecretRedaction(t *testing.T) {
	remote := map[string]any{"_id": "w1", "name": "Guest WiFi", "x_passphrase": "secret123"}
	local := map[string]any{"_id": "w1", "name": "Guest WiFi", "x_passphrase": redactedValue}

	srv := httptest.NewServer(testMux(map[string][]map[string]any{
		"/rest/wlanconf": {remote},
	}))
	defer srv.Close()

	dir := t.TempDir()
	if err := writeConfigFile(dir, "wlanconf", "guest-wifi", local); err != nil {
		t.Fatalf("writeConfigFile: %v", err)
	}

	c := newClient(srv.URL, false)
	var buf bytes.Buffer
	hasDiffs, err := cmdDiff(context.Background(), c, "default", dir, "wlanconf", false, &buf)
	if err != nil {
		t.Fatalf("cmdDiff() error = %v", err)
	}
	if hasDiffs {
		t.Errorf("hasDiffs = true, want false (secrets redacted both sides)\noutput: %q", buf.String())
	}
}

func TestCmdDiffSecretChanged(t *testing.T) {
	remote := map[string]any{"_id": "w1", "name": "Guest WiFi", "x_passphrase": "oldsecret"}
	local := map[string]any{"_id": "w1", "name": "Guest WiFi", "x_passphrase": redactedValue}

	srv := httptest.NewServer(testMux(map[string][]map[string]any{
		"/rest/wlanconf": {remote},
	}))
	defer srv.Close()

	dir := t.TempDir()
	if err := writeConfigFile(dir, "wlanconf", "guest-wifi", local); err != nil {
		t.Fatalf("writeConfigFile: %v", err)
	}
	t.Setenv("UNIFI_SYNC_SECRET_GUEST_WIFI_X_PASSPHRASE", "newsecret")

	c := newClient(srv.URL, false)
	var buf bytes.Buffer
	hasDiffs, err := cmdDiff(context.Background(), c, "default", dir, "wlanconf", false, &buf)
	if err != nil {
		t.Fatalf("cmdDiff() error = %v", err)
	}
	if !hasDiffs {
		t.Error("hasDiffs = false, want true (secret changed)")
	}
	if !strings.Contains(buf.String(), "(changed)") {
		t.Errorf("output should contain (changed) annotation: %q", buf.String())
	}
}

func TestCmdDiffSlugCollision(t *testing.T) {
	srv := httptest.NewServer(testMux(map[string][]map[string]any{
		"/rest/networkconf": {
			{"_id": "n1", "name": "Guest WiFi"},
			{"_id": "n2", "name": "Guest-WiFi"},
		},
	}))
	defer srv.Close()

	c := newClient(srv.URL, false)
	_, err := cmdDiff(context.Background(), c, "default", t.TempDir(), "networkconf", false, io.Discard)
	if err == nil {
		t.Error("cmdDiff() should return error on slug collision")
	}
	if !strings.Contains(err.Error(), "slug collision") {
		t.Errorf("error = %v, want slug collision", err)
	}
}

func TestCmdDiffColor(t *testing.T) {
	remote := map[string]any{"_id": "n1", "name": testNameHomeNet, "vlan": json.Number("10")}
	local := map[string]any{"_id": "n1", "name": testNameHomeNet, "vlan": json.Number("20")}

	srv := httptest.NewServer(testMux(map[string][]map[string]any{
		"/rest/networkconf": {remote},
	}))
	defer srv.Close()

	dir := t.TempDir()
	if err := writeConfigFile(dir, "networkconf", "homenet", local); err != nil {
		t.Fatalf("writeConfigFile: %v", err)
	}

	c := newClient(srv.URL, false)
	var buf bytes.Buffer
	_, err := cmdDiff(context.Background(), c, "default", dir, "networkconf", true, &buf)
	if err != nil {
		t.Fatalf("cmdDiff() error = %v", err)
	}
	if !strings.Contains(buf.String(), "\033[") {
		t.Error("output should contain ANSI escape codes when color=true")
	}
}

func TestCmdDiffListError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error")) //nolint:errcheck,revive // test handler
	}))
	defer srv.Close()

	c := newClient(srv.URL, false)
	_, err := cmdDiff(context.Background(), c, "default", t.TempDir(), "networkconf", false, io.Discard)
	if err == nil {
		t.Error("cmdDiff() should return error on list failure")
	}
}

func TestCmdDiffInvalidType(t *testing.T) {
	c := newClient("http://example.com", false)
	_, err := cmdDiff(context.Background(), c, "default", t.TempDir(), "badtype", false, io.Discard)
	if err == nil {
		t.Error("cmdDiff(badtype) should return error")
	}
}

func TestCmdDiffReadError(t *testing.T) {
	srv := httptest.NewServer(testMux(map[string][]map[string]any{
		"/rest/networkconf": {},
	}))
	defer srv.Close()

	dir := t.TempDir()
	subdir := filepath.Join(dir, "networkconf")
	if err := os.MkdirAll(subdir, 0o750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	os.Chmod(subdir, 0o000) //nolint:errcheck,revive // test setup
	t.Cleanup(func() {
		os.Chmod(subdir, 0o750) //nolint:errcheck,gosec,revive // test cleanup
	})

	c := newClient(srv.URL, false)
	_, err := cmdDiff(context.Background(), c, "default", dir, "networkconf", false, io.Discard)
	if err == nil {
		t.Error("cmdDiff() should return error on read failure")
	}
}

func TestCmdDiffMarshalLocalError(t *testing.T) {
	obj := map[string]any{"_id": "n1", "name": testNameHomeNet}
	srv := httptest.NewServer(testMux(map[string][]map[string]any{
		"/rest/networkconf": {obj},
	}))
	defer srv.Close()

	dir := t.TempDir()
	if err := writeConfigFile(dir, "networkconf", "homenet", obj); err != nil {
		t.Fatalf("writeConfigFile: %v", err)
	}

	orig := marshalJSONFn
	t.Cleanup(func() { marshalJSONFn = orig })
	marshalJSONFn = func(map[string]any) ([]byte, error) {
		return nil, errors.New("marshal error")
	}

	c := newClient(srv.URL, false)
	_, err := cmdDiff(context.Background(), c, "default", dir, "networkconf", false, io.Discard)
	if err == nil {
		t.Error("cmdDiff() should return error on local marshal failure")
	}
}

func TestCmdDiffMarshalRemoteError(t *testing.T) {
	srv := httptest.NewServer(testMux(map[string][]map[string]any{
		"/rest/networkconf": {{"_id": "n1", "name": testNameHomeNet}},
	}))
	defer srv.Close()

	dir := t.TempDir()

	orig := marshalJSONFn
	t.Cleanup(func() { marshalJSONFn = orig })
	marshalJSONFn = func(map[string]any) ([]byte, error) {
		return nil, errors.New("marshal error")
	}

	c := newClient(srv.URL, false)
	_, err := cmdDiff(context.Background(), c, "default", dir, "networkconf", false, io.Discard)
	if err == nil {
		t.Error("cmdDiff() should return error on remote marshal failure")
	}
}

func TestCmdDiffSkipsNoNameRemote(t *testing.T) {
	srv := httptest.NewServer(testMux(map[string][]map[string]any{
		"/rest/networkconf": {{"_id": "n1"}},
	}))
	defer srv.Close()

	c := newClient(srv.URL, false)
	var buf bytes.Buffer
	hasDiffs, err := cmdDiff(context.Background(), c, "default", t.TempDir(), "networkconf", false, &buf)
	if err != nil {
		t.Fatalf("cmdDiff() error = %v", err)
	}
	if hasDiffs {
		t.Error("hasDiffs = true, want false for nameless remote items")
	}
}

func TestCmdDiffSkipsEmptySlugRemote(t *testing.T) {
	srv := httptest.NewServer(testMux(map[string][]map[string]any{
		"/rest/networkconf": {{"_id": "n1", "name": "!!!"}},
	}))
	defer srv.Close()

	c := newClient(srv.URL, false)
	var buf bytes.Buffer
	hasDiffs, err := cmdDiff(context.Background(), c, "default", t.TempDir(), "networkconf", false, &buf)
	if err != nil {
		t.Fatalf("cmdDiff() error = %v", err)
	}
	if hasDiffs {
		t.Error("hasDiffs = true, want false for remote item with unsluggable name")
	}
}
