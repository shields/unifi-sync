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
	"fmt"
	"os"
	"strings"
)

// nestedSecret locates a secret field inside the elements of an array-of-objects
// field (for example sae_psk[].psk).
type nestedSecret struct {
	arrayField  string
	secretField string
}

// secretSpec describes the secret-bearing fields of one resource type: plain
// top-level string fields, plus secrets nested inside arrays of objects.
type secretSpec struct {
	scalar []string
	nested []nestedSecret
}

// resourceSecrets maps each resource type to its secret fields. Scalar fields
// are redacted on pull and injected on push verbatim. Nested fields live inside
// arrays whose elements carry no reliably-unique, non-empty identifier of their
// own (networkconf_id and id may both be blank), so each is addressed by its
// element's position: the env var is UNIFI_SYNC_SECRET_<slug>_<ARRAY>_<INDEX>_<FIELD>.
var resourceSecrets = map[string]secretSpec{
	// networkconf secrets appear only when a VPN, PPPoE, or WireGuard feature is
	// configured. Public material (x_ca_crt, x_server_crt, x_shared_client_crt,
	// x_dh_key) is deliberately excluded: redacting it would force it to be
	// supplied via env on push for no security benefit.
	"networkconf": {scalar: []string{
		"x_wan_password",                 // PPPoE/WAN password
		"x_pptpc_password",               // PPTP client password
		"x_ipsec_pre_shared_key",         // IPsec pre-shared key
		"wireguard_client_preshared_key", // WireGuard client pre-shared key
		"x_wireguard_private_key",        // WireGuard private key
		"x_openvpn_password",             // OpenVPN password
		"x_openvpn_shared_secret_key",    // OpenVPN static key
		"x_auth_key",                     // VPN tls-auth key
		"x_ca_key",                       // CA private key
		"x_server_key",                   // server private key
		"x_shared_client_key",            // shared-client private key
	}},
	"wlanconf": {
		scalar: []string{"x_passphrase", "x_iapp_key", "x_wep", "x_wep_key", "x_radius_secret_1"},
		nested: []nestedSecret{
			{arrayField: "private_preshared_keys", secretField: "password"}, // WPA2 PPSK
			{arrayField: "sae_psk", secretField: "psk"},                     // WPA3 SAE
		},
	},
}

const redactedValue = "__REDACTED__"

// redactField replaces m[field] with the redaction placeholder when present.
func redactField(m map[string]any, field string) {
	if _, exists := m[field]; exists {
		m[field] = redactedValue
	}
}

func redactSecrets(obj map[string]any, resourceType string) {
	spec := resourceSecrets[resourceType]
	for _, f := range spec.scalar {
		redactField(obj, f)
	}
	for _, ns := range spec.nested {
		for _, ent := range nestedSecretEntries(obj, ns) {
			redactField(ent.elem, ns.secretField)
		}
	}
}

// nestedEntry is an object element of a nested secret array paired with its index
// in that array.
type nestedEntry struct {
	elem  map[string]any
	index int
}

// nestedSecretEntries returns the object elements of obj[ns.arrayField] with
// their array index. Non-object entries are skipped but still consume their
// index, so index matches the element's position in the stored file.
func nestedSecretEntries(obj map[string]any, ns nestedSecret) []nestedEntry {
	arr, ok := obj[ns.arrayField].([]any)
	if !ok {
		return nil
	}
	entries := make([]nestedEntry, 0, len(arr))
	for i, e := range arr {
		if m, ok := e.(map[string]any); ok {
			entries = append(entries, nestedEntry{elem: m, index: i})
		}
	}
	return entries
}

// nestedFieldKey builds the secretEnvVar field component for a nested secret at
// the given array index, e.g. "private_preshared_keys_0_password".
func nestedFieldKey(ns nestedSecret, index int) string {
	return fmt.Sprintf("%s_%d_%s", ns.arrayField, index, ns.secretField)
}

// resolveSecret returns the environment value to inject for m[field]. ok is
// false without error when the field is absent or not redacted (nothing to
// inject); a redacted field with no corresponding env var is an error.
func resolveSecret(m map[string]any, field, envName string) (value string, ok bool, err error) {
	s, isStr := m[field].(string)
	if !isStr || s != redactedValue {
		return "", false, nil
	}
	envVal, found := os.LookupEnv(envName)
	if !found {
		return "", false, fmt.Errorf("missing environment variable %s for secret field %q", envName, field)
	}
	return envVal, true, nil
}

func injectSecrets(obj map[string]any, resourceType, slug string) error {
	spec := resourceSecrets[resourceType]

	// Resolve every secret before mutating obj so that a missing env var aborts
	// the whole resource rather than leaving it partially injected.
	type pending struct {
		m     map[string]any
		field string
		value string
	}
	var resolved []pending

	for _, f := range spec.scalar {
		value, ok, err := resolveSecret(obj, f, secretEnvVar(slug, f))
		if err != nil {
			return err
		}
		if ok {
			resolved = append(resolved, pending{obj, f, value})
		}
	}
	for _, ns := range spec.nested {
		for _, ent := range nestedSecretEntries(obj, ns) {
			value, ok, err := resolveSecret(ent.elem, ns.secretField, secretEnvVar(slug, nestedFieldKey(ns, ent.index)))
			if err != nil {
				return err
			}
			if ok {
				resolved = append(resolved, pending{ent.elem, ns.secretField, value})
			}
		}
	}

	for _, p := range resolved {
		p.m[p.field] = p.value
	}
	return nil
}

// annotateSecretChanges redacts secret fields in both maps for diff display. A
// local field whose effective value (resolved from the environment when stored
// as __REDACTED__) differs from the remote value is annotated
// "__REDACTED__ (changed)".
func annotateSecretChanges(local, remote map[string]any, resourceType, slug string) {
	spec := resourceSecrets[resourceType]
	for _, f := range spec.scalar {
		annotateField(local, remote, f, secretEnvVar(slug, f))
	}
	for _, ns := range spec.nested {
		annotateNested(local, remote, ns, slug)
	}
}

// annotateField redacts field in local and remote for display, marking local as
// "(changed)" when its effective value (resolved from envName when stored as
// __REDACTED__) differs from remote.
func annotateField(local, remote map[string]any, field, envName string) {
	localVal, hasLocal := local[field]
	remoteVal, hasRemote := remote[field]

	var localEff string
	localResolved := false
	if s, ok := localVal.(string); ok {
		if s == redactedValue {
			if envVal, found := os.LookupEnv(envName); found {
				localEff = envVal
				localResolved = true
			}
		} else {
			localEff = s
			localResolved = true
		}
	}

	changed := false
	if localResolved && hasRemote {
		if remoteStr, ok := remoteVal.(string); ok && localEff != remoteStr {
			changed = true
		}
	}

	if hasLocal {
		if changed {
			local[field] = redactedValue + " (changed)"
		} else {
			local[field] = redactedValue
		}
	}
	if hasRemote {
		remote[field] = redactedValue
	}
}

// annotateNested applies annotateField to each element of a nested secret array,
// matching local and remote elements by index. Every remote element's secret is
// redacted, including those with no local counterpart, so none leaks into the
// diff.
func annotateNested(local, remote map[string]any, ns nestedSecret, slug string) {
	remoteVals := make(map[int]any)
	for _, ent := range nestedSecretEntries(remote, ns) {
		if v, exists := ent.elem[ns.secretField]; exists {
			remoteVals[ent.index] = v
			ent.elem[ns.secretField] = redactedValue
		}
	}
	for _, ent := range nestedSecretEntries(local, ns) {
		remoteElem := make(map[string]any)
		if v, ok := remoteVals[ent.index]; ok {
			remoteElem[ns.secretField] = v
		}
		annotateField(ent.elem, remoteElem, ns.secretField, secretEnvVar(slug, nestedFieldKey(ns, ent.index)))
	}
}

// secretEnvVar builds the env var name for a secret field. Slugs may contain
// Unicode letters (slugify preserves them), so any rune that is not an ASCII
// letter, digit, or hyphen is encoded as __<HEX>__ (its uppercase hex code
// point) to keep the name a valid POSIX identifier. ASCII slugs are unchanged,
// e.g. "guest-wifi" -> "GUEST_WIFI"; "café" -> "CAF__E9__".
func secretEnvVar(slug, field string) string {
	var b strings.Builder
	_, _ = b.WriteString("UNIFI_SYNC_SECRET_")
	for _, r := range slug {
		switch {
		case r >= 'a' && r <= 'z':
			_, _ = b.WriteRune(r - 'a' + 'A')
		case r >= '0' && r <= '9':
			_, _ = b.WriteRune(r)
		case r == '-':
			_ = b.WriteByte('_')
		default:
			_, _ = fmt.Fprintf(&b, "__%X__", r)
		}
	}
	_ = b.WriteByte('_')
	_, _ = b.WriteString(strings.ToUpper(field))
	return b.String()
}
