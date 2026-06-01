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
	// usergroup carries no secret-bearing fields; nothing should change.
	obj := map[string]any{
		"name":            "Default",
		"qos_rate_max_up": "0",
	}
	redactSecrets(obj, "usergroup")
	if obj["name"] != "Default" {
		t.Errorf("name was modified: %v", obj["name"])
	}
}

func TestRedactSecretsNetworkconfVPN(t *testing.T) {
	secrets := []string{
		"x_wan_password", "x_pptpc_password", "x_ipsec_pre_shared_key",
		"wireguard_client_preshared_key", "x_wireguard_private_key",
		"x_openvpn_password", "x_openvpn_shared_secret_key", "x_auth_key",
		"x_ca_key", "x_server_key", "x_shared_client_key",
	}
	obj := map[string]any{
		"name":         "Office VPN",
		"x_ca_crt":     "-----BEGIN CERTIFICATE-----", // public, must NOT be redacted
		"wan_username": "pppoe-user",                  // identifier, not a secret
	}
	for _, f := range secrets {
		obj[f] = "value-of-" + f
	}
	redactSecrets(obj, "networkconf")
	for _, f := range secrets {
		if obj[f] != redactedValue {
			t.Errorf("%s = %v, want %q", f, obj[f], redactedValue)
		}
	}
	if obj["x_ca_crt"] != "-----BEGIN CERTIFICATE-----" {
		t.Errorf("x_ca_crt (public cert) was redacted: %v", obj["x_ca_crt"])
	}
	if obj["wan_username"] != "pppoe-user" {
		t.Errorf("wan_username (non-secret) was modified: %v", obj["wan_username"])
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

func TestInjectSecretsNonASCIISlug(t *testing.T) {
	obj := map[string]any{
		"name":         "Café WiFi",
		"x_passphrase": redactedValue,
	}
	// A non-ASCII WLAN name must still map to a valid, settable env var name.
	t.Setenv("UNIFI_SYNC_SECRET_CAF__E9___WIFI_X_PASSPHRASE", "héllo-secret")

	if err := injectSecrets(obj, "wlanconf", "café-wifi"); err != nil {
		t.Fatalf("injectSecrets() error = %v", err)
	}
	if obj["x_passphrase"] != "héllo-secret" {
		t.Errorf("x_passphrase = %v, want injected secret", obj["x_passphrase"])
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
	obj := map[string]any{"name": "Default"}
	err := injectSecrets(obj, "usergroup", "default")
	if err != nil {
		t.Fatalf("injectSecrets() error = %v", err)
	}
}

func TestInjectSecretsNetworkconf(t *testing.T) {
	obj := map[string]any{
		"name":                   "Office VPN",
		"x_ipsec_pre_shared_key": redactedValue,
	}
	t.Setenv("UNIFI_SYNC_SECRET_OFFICE_VPN_X_IPSEC_PRE_SHARED_KEY", "psk-injected")

	if err := injectSecrets(obj, "networkconf", "office-vpn"); err != nil {
		t.Fatalf("injectSecrets() error = %v", err)
	}
	if obj["x_ipsec_pre_shared_key"] != "psk-injected" {
		t.Errorf("x_ipsec_pre_shared_key = %v, want psk-injected", obj["x_ipsec_pre_shared_key"])
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
	local := map[string]any{"name": "Default"}
	remote := map[string]any{"name": "Default"}
	annotateSecretChanges(local, remote, "usergroup", "default")
	if local["name"] != "Default" {
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
		{"wifi-5ghz", "x_passphrase", "UNIFI_SYNC_SECRET_WIFI_5GHZ_X_PASSPHRASE"},
		{"café-wifi", "x_passphrase", "UNIFI_SYNC_SECRET_CAF__E9___WIFI_X_PASSPHRASE"},
		{"home-wifi", "private_preshared_keys_0_password", "UNIFI_SYNC_SECRET_HOME_WIFI_PRIVATE_PRESHARED_KEYS_0_PASSWORD"},
		{"home-wifi", "sae_psk_2_psk", "UNIFI_SYNC_SECRET_HOME_WIFI_SAE_PSK_2_PSK"},
	}
	for _, tt := range tests {
		got := secretEnvVar(tt.slug, tt.field)
		if got != tt.want {
			t.Errorf("secretEnvVar(%q, %q) = %q, want %q", tt.slug, tt.field, got, tt.want)
		}
	}
}

func nestedArray(t *testing.T, m map[string]any, field string) []any {
	t.Helper()
	arr, ok := m[field].([]any)
	if !ok {
		t.Fatalf("%s is not an array", field)
	}
	return arr
}

func nestedElem(t *testing.T, m map[string]any, field string, i int) map[string]any {
	t.Helper()
	elem, ok := nestedArray(t, m, field)[i].(map[string]any)
	if !ok {
		t.Fatalf("%s[%d] is not an object", field, i)
	}
	return elem
}

func ppsk(t *testing.T, m map[string]any, i int) map[string]any {
	t.Helper()
	return nestedElem(t, m, "private_preshared_keys", i)
}

func TestRedactSecretsNestedKeys(t *testing.T) {
	obj := map[string]any{
		"name": "Multi-PSK",
		"private_preshared_keys": []any{
			map[string]any{"networkconf_id": "net0", "password": "ppsk-secret-0"},
			map[string]any{"networkconf_id": "", "password": "ppsk-secret-1"}, // blank id, still redacted
		},
		"sae_psk": []any{
			map[string]any{"id": "sae0", "psk": "sae-secret-0", "vlan": "10"},
		},
	}
	redactSecrets(obj, "wlanconf")

	for i := range nestedArray(t, obj, "private_preshared_keys") {
		if got := ppsk(t, obj, i)["password"]; got != redactedValue {
			t.Errorf("private_preshared_keys[%d].password = %v, want redacted", i, got)
		}
	}
	if ppsk(t, obj, 0)["networkconf_id"] != "net0" {
		t.Error("networkconf_id (non-secret) was modified")
	}
	sae := nestedElem(t, obj, "sae_psk", 0)
	if sae["psk"] != redactedValue {
		t.Errorf("sae_psk[0].psk = %v, want redacted", sae["psk"])
	}
	if sae["vlan"] != "10" {
		t.Error("sae_psk[0].vlan (non-secret) was modified")
	}
}

func TestRedactSecretsNestedNonObjectElement(t *testing.T) {
	// A non-object array entry is skipped without panicking; the object entry
	// after it is still redacted.
	obj := map[string]any{
		"private_preshared_keys": []any{
			"junk",
			map[string]any{"password": "secret"},
		},
	}
	redactSecrets(obj, "wlanconf")
	if got := ppsk(t, obj, 1)["password"]; got != redactedValue {
		t.Errorf("password = %v, want redacted", got)
	}
}

func TestInjectSecretsNestedKeys(t *testing.T) {
	obj := map[string]any{
		"private_preshared_keys": []any{
			map[string]any{"networkconf_id": "net0", "password": redactedValue},
			map[string]any{"networkconf_id": "net1", "password": redactedValue},
		},
		"sae_psk": []any{
			map[string]any{"id": "sae0", "psk": redactedValue},
		},
	}
	t.Setenv("UNIFI_SYNC_SECRET_MULTI_PSK_PRIVATE_PRESHARED_KEYS_0_PASSWORD", "ppsk0")
	t.Setenv("UNIFI_SYNC_SECRET_MULTI_PSK_PRIVATE_PRESHARED_KEYS_1_PASSWORD", "ppsk1")
	t.Setenv("UNIFI_SYNC_SECRET_MULTI_PSK_SAE_PSK_0_PSK", "sae0pass")

	if err := injectSecrets(obj, "wlanconf", "multi-psk"); err != nil {
		t.Fatalf("injectSecrets() error = %v", err)
	}
	if got := ppsk(t, obj, 0)["password"]; got != "ppsk0" {
		t.Errorf("private_preshared_keys[0].password = %v, want ppsk0", got)
	}
	if got := ppsk(t, obj, 1)["password"]; got != "ppsk1" {
		t.Errorf("private_preshared_keys[1].password = %v, want ppsk1", got)
	}
	if got := nestedElem(t, obj, "sae_psk", 0)["psk"]; got != "sae0pass" {
		t.Errorf("sae_psk[0].psk = %v, want sae0pass", got)
	}
}

func TestInjectSecretsNestedMissingEnv(t *testing.T) {
	obj := map[string]any{
		"private_preshared_keys": []any{
			map[string]any{"networkconf_id": "net0", "password": redactedValue},
		},
	}
	// No env var set for index 0.
	if err := injectSecrets(obj, "wlanconf", "multi-psk"); err == nil {
		t.Error("injectSecrets() should error when a nested secret env var is missing")
	}
}

func TestInjectSecretsNestedPlaintextPreserved(t *testing.T) {
	obj := map[string]any{
		"private_preshared_keys": []any{
			map[string]any{"networkconf_id": "net0", "password": "already-plain"},
		},
	}
	if err := injectSecrets(obj, "wlanconf", "multi-psk"); err != nil {
		t.Fatalf("injectSecrets() error = %v", err)
	}
	if got := ppsk(t, obj, 0)["password"]; got != "already-plain" {
		t.Errorf("password = %v, want already-plain (unchanged)", got)
	}
}

func TestAnnotateSecretChangesNestedChanged(t *testing.T) {
	local := map[string]any{
		"private_preshared_keys": []any{
			map[string]any{"networkconf_id": "net0", "password": redactedValue},
		},
	}
	remote := map[string]any{
		"private_preshared_keys": []any{
			map[string]any{"networkconf_id": "net0", "password": "old-ppsk"},
		},
	}
	t.Setenv("UNIFI_SYNC_SECRET_MULTI_PSK_PRIVATE_PRESHARED_KEYS_0_PASSWORD", "new-ppsk")

	annotateSecretChanges(local, remote, "wlanconf", "multi-psk")
	if got := ppsk(t, local, 0)["password"]; got != redactedValue+" (changed)" {
		t.Errorf("local password = %v, want annotated changed", got)
	}
	if got := ppsk(t, remote, 0)["password"]; got != redactedValue {
		t.Errorf("remote password = %v, want redacted", got)
	}
}

func TestAnnotateSecretChangesNestedSame(t *testing.T) {
	local := map[string]any{
		"sae_psk": []any{map[string]any{"id": "s0", "psk": redactedValue}},
	}
	remote := map[string]any{
		"sae_psk": []any{map[string]any{"id": "s0", "psk": "same"}},
	}
	t.Setenv("UNIFI_SYNC_SECRET_W_SAE_PSK_0_PSK", "same")

	annotateSecretChanges(local, remote, "wlanconf", "w")
	if got := nestedElem(t, local, "sae_psk", 0)["psk"]; got != redactedValue {
		t.Errorf("local psk = %v, want plain redacted", got)
	}
}

func TestAnnotateSecretChangesNestedRemoteOnly(t *testing.T) {
	// A remote element with no local counterpart still gets its secret redacted.
	local := map[string]any{"private_preshared_keys": []any{}}
	remote := map[string]any{
		"private_preshared_keys": []any{
			map[string]any{"networkconf_id": "net0", "password": "remote-secret"},
		},
	}
	annotateSecretChanges(local, remote, "wlanconf", "w")
	if got := ppsk(t, remote, 0)["password"]; got != redactedValue {
		t.Errorf("remote password = %v, want redacted", got)
	}
}

func TestAnnotateSecretChangesNestedLocalNoRemoteMatch(t *testing.T) {
	// A local element past the end of the remote array is redacted, not marked
	// changed (there is nothing to compare against).
	local := map[string]any{
		"private_preshared_keys": []any{
			map[string]any{"networkconf_id": "net0", "password": redactedValue},
			map[string]any{"networkconf_id": "net1", "password": redactedValue},
		},
	}
	remote := map[string]any{
		"private_preshared_keys": []any{
			map[string]any{"networkconf_id": "net0", "password": "r0"},
		},
	}
	t.Setenv("UNIFI_SYNC_SECRET_W_PRIVATE_PRESHARED_KEYS_0_PASSWORD", "r0") // equals remote -> unchanged
	t.Setenv("UNIFI_SYNC_SECRET_W_PRIVATE_PRESHARED_KEYS_1_PASSWORD", "x1") // no remote counterpart

	annotateSecretChanges(local, remote, "wlanconf", "w")
	if got := ppsk(t, local, 0)["password"]; got != redactedValue {
		t.Errorf("local[0] password = %v, want plain redacted (matches remote)", got)
	}
	if got := ppsk(t, local, 1)["password"]; got != redactedValue {
		t.Errorf("local[1] password = %v, want plain redacted (no remote counterpart)", got)
	}
}
