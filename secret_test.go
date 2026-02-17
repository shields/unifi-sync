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
	t.Setenv("UNIFI_SECRET_HOMENET_WIFI_X_PASSPHRASE", "injected123")

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
	// wlanconf has x_passphrase as secret, but object lacks it
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

func TestSecretEnvVarName(t *testing.T) {
	tests := []struct {
		slug  string
		field string
		want  string
	}{
		{"homenet-wifi", "x_passphrase", "UNIFI_SECRET_HOMENET_WIFI_X_PASSPHRASE"},
		{"guest-network", "x_passphrase", "UNIFI_SECRET_GUEST_NETWORK_X_PASSPHRASE"},
		{"msrl", "x_passphrase", "UNIFI_SECRET_MSRL_X_PASSPHRASE"},
	}
	for _, tt := range tests {
		got := secretEnvVar(tt.slug, tt.field)
		if got != tt.want {
			t.Errorf("secretEnvVar(%q, %q) = %q, want %q", tt.slug, tt.field, got, tt.want)
		}
	}
}
