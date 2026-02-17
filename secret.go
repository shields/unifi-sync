package main

import (
	"fmt"
	"os"
	"strings"
)

var secretFields = map[string][]string{
	"wlanconf": {"x_passphrase"},
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

// secretEnvVar builds the env var name for a secret field.
// Slugs only contain [a-z0-9-] (produced by slugify), so hyphen is the only
// character that needs replacing.
func secretEnvVar(slug, field string) string {
	slugPart := strings.ToUpper(strings.ReplaceAll(slug, "-", "_"))
	fieldPart := strings.ToUpper(field)
	return "UNIFI_SECRET_" + slugPart + "_" + fieldPart
}
