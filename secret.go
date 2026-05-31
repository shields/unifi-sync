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

var secretFields = map[string][]string{
	// networkconf secrets appear only when a VPN, PPPoE, or WireGuard feature is
	// configured. Public material (x_ca_crt, x_server_crt, x_shared_client_crt,
	// x_dh_key) is deliberately excluded: redacting it would force it to be
	// supplied via env on push for no security benefit.
	"networkconf": {
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
	},
	"wlanconf": {"x_passphrase", "x_iapp_key", "x_wep", "x_wep_key", "x_radius_secret_1"},
}

const redactedValue = "__REDACTED__"

func redactSecrets(obj map[string]any, resourceType string) {
	fields, ok := secretFields[resourceType]
	if !ok {
		return
	}
	for _, f := range fields {
		if _, exists := obj[f]; exists {
			obj[f] = redactedValue
		}
	}
}

func injectSecrets(obj map[string]any, resourceType, slug string) error {
	fields, ok := secretFields[resourceType]
	if !ok {
		return nil
	}
	// Collect all values before mutating obj to avoid partial injection on error
	resolved := make(map[string]string)
	for _, f := range fields {
		val, exists := obj[f]
		if !exists {
			continue
		}
		s, ok := val.(string)
		if !ok || s != redactedValue {
			continue
		}
		envName := secretEnvVar(slug, f)
		envVal, found := os.LookupEnv(envName)
		if !found {
			return fmt.Errorf("missing environment variable %s for secret field %q", envName, f)
		}
		resolved[f] = envVal
	}
	for f, v := range resolved {
		obj[f] = v
	}
	return nil
}

// annotateSecretChanges redacts secret fields in both maps for diff display.
// If the effective local value (resolved from env if __REDACTED__) differs from
// the remote value, the local field is annotated as "__REDACTED__ (changed)".
func annotateSecretChanges(local, remote map[string]any, resourceType, slug string) {
	fields, ok := secretFields[resourceType]
	if !ok {
		return
	}
	for _, f := range fields {
		localVal, hasLocal := local[f]
		remoteVal, hasRemote := remote[f]

		// Resolve effective local value
		var localEff string
		localResolved := false
		if hasLocal {
			if s, ok := localVal.(string); ok {
				if s == redactedValue {
					envName := secretEnvVar(slug, f)
					if envVal, found := os.LookupEnv(envName); found {
						localEff = envVal
						localResolved = true
					}
				} else {
					localEff = s
					localResolved = true
				}
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
				local[f] = redactedValue + " (changed)"
			} else {
				local[f] = redactedValue
			}
		}
		if hasRemote {
			remote[f] = redactedValue
		}
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
