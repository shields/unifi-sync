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
	"testing"
)

func TestRedactSecrets(t *testing.T) {
	obj := map[string]any{
		"name":         "HomeNet WiFi",
		"x_passphrase": "supersecret123",
		"enabled":      true,
	}
	redactSecrets(obj, "wlanconf")
	if obj["x_passphrase"] != redactedValue {
		t.Errorf("x_passphrase = %v, want __REDACTED__", obj["x_passphrase"])
	}
	if obj["name"] != "HomeNet WiFi" {
		t.Errorf("name was modified: %v", obj["name"])
	}
}

func TestRedactSecretsAllFields(t *testing.T) {
	obj := map[string]any{
		"name":              "TestNet",
		"x_passphrase":      "wifipass",
		"x_iapp_key":        "iappkey123",
		"x_wep":             "wepkey",
		"x_wep_key":         "wepkey2",
		"x_radius_secret_1": "radiussecret",
		"x_iapp":            true,
		"x_ccode":           "US",
	}
	redactSecrets(obj, "wlanconf")
	for _, f := range []string{"x_passphrase", "x_iapp_key", "x_wep", "x_wep_key", "x_radius_secret_1"} {
		if obj[f] != redactedValue {
			t.Errorf("%s = %v, want %q", f, obj[f], redactedValue)
		}
	}
	if obj["x_iapp"] != true { //nolint:revive // explicit bool comparison for clarity
		t.Error("x_iapp (non-secret) was modified")
	}
	if obj["x_ccode"] != "US" {
		t.Error("x_ccode (non-secret) was modified")
	}
}

func TestRedactSecretsNoSecretFields(t *testing.T) {
	obj := map[string]any{
		"name": "LAN",
		"vlan": "100",
	}
	redactSecrets(obj, "networkconf")
	if obj["name"] != "LAN" {
		t.Errorf("name was modified: %v", obj["name"])
	}
}

func TestRedactSecretsUnknownType(t *testing.T) {
	obj := map[string]any{"name": "test"}
	redactSecrets(obj, "unknowntype")
	if obj["name"] != "test" {
		t.Errorf("name was modified: %v", obj["name"])
	}
}

func TestRedactSecretsMissingField(t *testing.T) {
	obj := map[string]any{"name": "NoPassword"}
	redactSecrets(obj, "wlanconf")
	if _, ok := obj["x_passphrase"]; ok {
		t.Error("should not add x_passphrase if not present")
	}
}

func TestInjectSecrets(t *testing.T) {
	obj := map[string]any{
		"name":         "HomeNet WiFi",
		"x_passphrase": redactedValue,
	}
	t.Setenv("UNIFI_SYNC_SECRET_HOMENET_WIFI_X_PASSPHRASE", "injected123")

	err := injectSecrets(obj, "wlanconf", "homenet-wifi")
	if err != nil {
		t.Fatalf("injectSecrets() error = %v", err)
	}
	if obj["x_passphrase"] != "injected123" {
		t.Errorf("x_passphrase = %v, want injected123", obj["x_passphrase"])
	}
}

func TestInjectSecretsNotRedacted(t *testing.T) {
	obj := map[string]any{
		"name":         "HomeNet WiFi",
		"x_passphrase": "plaintext",
	}

	err := injectSecrets(obj, "wlanconf", "homenet-wifi")
	if err != nil {
		t.Fatalf("injectSecrets() error = %v", err)
	}
	if obj["x_passphrase"] != "plaintext" {
		t.Errorf("x_passphrase = %v, want plaintext (unchanged)", obj["x_passphrase"])
	}
}

func TestInjectSecretsMissingEnv(t *testing.T) {
	obj := map[string]any{
		"name":         "HomeNet WiFi",
		"x_passphrase": redactedValue,
	}

	err := injectSecrets(obj, "wlanconf", "homenet-wifi")
	if err == nil {
		t.Error("injectSecrets() should return error for missing env var")
	}
}

func TestInjectSecretsMissingFieldInObj(t *testing.T) {
	obj := map[string]any{"name": "NoPassField"}
	err := injectSecrets(obj, "wlanconf", "nopassfield")
	if err != nil {
		t.Fatalf("injectSecrets() error = %v", err)
	}
	if _, ok := obj["x_passphrase"]; ok {
		t.Error("should not add x_passphrase if not present in obj")
	}
}

func TestInjectSecretsNoSecretFields(t *testing.T) {
	obj := map[string]any{"name": "LAN"}
	err := injectSecrets(obj, "networkconf", "lan")
	if err != nil {
		t.Fatalf("injectSecrets() error = %v", err)
	}
}

func TestInjectSecretsUnknownType(t *testing.T) {
	obj := map[string]any{"name": "test"}
	err := injectSecrets(obj, "unknowntype", "test")
	if err != nil {
		t.Fatalf("injectSecrets() error = %v", err)
	}
}

func TestAnnotateSecretChangesDifferent(t *testing.T) {
	local := map[string]any{"name": "Guest WiFi", "x_passphrase": redactedValue}
	remote := map[string]any{"name": "Guest WiFi", "x_passphrase": "oldsecret"}
	t.Setenv("UNIFI_SYNC_SECRET_GUEST_WIFI_X_PASSPHRASE", "newsecret")

	annotateSecretChanges(local, remote, "wlanconf", "guest-wifi")
	if local["x_passphrase"] != redactedValue+" (changed)" {
		t.Errorf("local x_passphrase = %v, want annotated changed", local["x_passphrase"])
	}
	if remote["x_passphrase"] != redactedValue {
		t.Errorf("remote x_passphrase = %v, want redacted", remote["x_passphrase"])
	}
}

func TestAnnotateSecretChangesSame(t *testing.T) {
	local := map[string]any{"name": "Guest WiFi", "x_passphrase": redactedValue}
	remote := map[string]any{"name": "Guest WiFi", "x_passphrase": "samesecret"}
	t.Setenv("UNIFI_SYNC_SECRET_GUEST_WIFI_X_PASSPHRASE", "samesecret")

	annotateSecretChanges(local, remote, "wlanconf", "guest-wifi")
	if local["x_passphrase"] != redactedValue {
		t.Errorf("local x_passphrase = %v, want %q (no annotation)", local["x_passphrase"], redactedValue)
	}
	if remote["x_passphrase"] != redactedValue {
		t.Errorf("remote x_passphrase = %v, want redacted", remote["x_passphrase"])
	}
}

func TestAnnotateSecretChangesNoEnvVar(t *testing.T) {
	local := map[string]any{"name": "Guest WiFi", "x_passphrase": redactedValue}
	remote := map[string]any{"name": "Guest WiFi", "x_passphrase": "secret"}

	annotateSecretChanges(local, remote, "wlanconf", "guest-wifi")
	if local["x_passphrase"] != redactedValue {
		t.Errorf("local x_passphrase = %v, want %q", local["x_passphrase"], redactedValue)
	}
	if remote["x_passphrase"] != redactedValue {
		t.Errorf("remote x_passphrase = %v, want redacted", remote["x_passphrase"])
	}
}

func TestAnnotateSecretChangesPlaintextLocal(t *testing.T) {
	local := map[string]any{"name": "Guest WiFi", "x_passphrase": "newsecret"}
	remote := map[string]any{"name": "Guest WiFi", "x_passphrase": "oldsecret"}

	annotateSecretChanges(local, remote, "wlanconf", "guest-wifi")
	if local["x_passphrase"] != redactedValue+" (changed)" {
		t.Errorf("local x_passphrase = %v, want annotated changed", local["x_passphrase"])
	}
}

func TestAnnotateSecretChangesNoSecretType(t *testing.T) {
	local := map[string]any{"name": "HomeNet"}
	remote := map[string]any{"name": "HomeNet"}
	annotateSecretChanges(local, remote, "networkconf", "homenet")
	if local["name"] != "HomeNet" {
		t.Errorf("name was modified: %v", local["name"])
	}
}

func TestAnnotateSecretChangesMissingField(t *testing.T) {
	local := map[string]any{"name": "Guest WiFi"}
	remote := map[string]any{"name": "Guest WiFi", "x_passphrase": "secret"}
	annotateSecretChanges(local, remote, "wlanconf", "guest-wifi")
	if _, ok := local["x_passphrase"]; ok {
		t.Error("should not add x_passphrase to local if not present")
	}
	if remote["x_passphrase"] != redactedValue {
		t.Errorf("remote x_passphrase = %v, want redacted", remote["x_passphrase"])
	}
}

func TestSecretEnvVarName(t *testing.T) {
	tests := []struct {
		slug  string
		field string
		want  string
	}{
		{"homenet-wifi", "x_passphrase", "UNIFI_SYNC_SECRET_HOMENET_WIFI_X_PASSPHRASE"},
		{"guest-network", "x_passphrase", "UNIFI_SYNC_SECRET_GUEST_NETWORK_X_PASSPHRASE"},
		{"msrl", "x_passphrase", "UNIFI_SYNC_SECRET_MSRL_X_PASSPHRASE"},
		{"corp-wifi", "x_iapp_key", "UNIFI_SYNC_SECRET_CORP_WIFI_X_IAPP_KEY"},
		{"legacy-net", "x_wep", "UNIFI_SYNC_SECRET_LEGACY_NET_X_WEP"},
		{"legacy-net", "x_wep_key", "UNIFI_SYNC_SECRET_LEGACY_NET_X_WEP_KEY"},
		{"corp-wifi", "x_radius_secret_1", "UNIFI_SYNC_SECRET_CORP_WIFI_X_RADIUS_SECRET_1"},
	}
	for _, tt := range tests {
		got := secretEnvVar(tt.slug, tt.field)
		if got != tt.want {
			t.Errorf("secretEnvVar(%q, %q) = %q, want %q", tt.slug, tt.field, got, tt.want)
		}
	}
}
