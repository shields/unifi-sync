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
	"context"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
)

var (
	resourceTypes = []string{"networkconf", "wlanconf", "usergroup"}
	marshalJSONFn = marshalJSON
)

func selectedTypes(filterType string) ([]string, error) {
	if filterType == "" {
		return resourceTypes, nil
	}
	if !slices.Contains(resourceTypes, filterType) {
		return nil, fmt.Errorf("unknown resource type: %s", filterType)
	}
	return []string{filterType}, nil
}

func cmdPull(ctx context.Context, c *client, site, configDir, filterType string, w io.Writer) error {
	types, err := selectedTypes(filterType)
	if err != nil {
		return err
	}
	for _, rt := range types {
		items, listErr := c.list(ctx, site, rt)
		if listErr != nil {
			return listErr
		}
		slugsSeen := make(map[string]string)
		for _, obj := range items {
			name, _ := obj["name"].(string) //nolint:errcheck // empty string on failure is correct
			if name == "" {
				continue
			}
			slug := slugify(name)
			if existing, ok := slugsSeen[slug]; ok {
				return fmt.Errorf("slug collision in %s: %q and %q both slugify to %q", rt, existing, name, slug)
			}
			slugsSeen[slug] = name
			// obj is a fresh decode from the HTTP response; safe to mutate in place
			redactSecrets(obj, rt)
			if writeErr := writeConfigFile(configDir, rt, slug, obj); writeErr != nil {
				return writeErr
			}
			fmt.Fprintf(w, "pulled %s/%s\n", rt, slug) //nolint:errcheck,revive // writing to stdout
		}
	}
	return nil
}

// cmdPush reads local config files, injects secrets, and sends them to the
// controller. _id present -> PUT (update), _id absent -> POST (create).
// After POST, the server response is written back to capture the _id.
// After a successful push, it pulls back and diffs to verify the controller
// accepted the configuration as-is. Returns true if verification found diffs.
func cmdPush( //nolint:revive // dryRun control flag is inherent to the command design
	ctx context.Context, c *client, site, configDir, filterType string,
	dryRun, color bool, w io.Writer,
) (bool, error) {
	types, err := selectedTypes(filterType)
	if err != nil {
		return false, err
	}
	for _, rt := range types {
		files, readErr := readConfigFiles(configDir, rt)
		if readErr != nil {
			return false, readErr
		}
		slugs := make([]string, 0, len(files))
		for slug := range files {
			slugs = append(slugs, slug)
		}
		slices.Sort(slugs)
		for _, slug := range slugs {
			obj := files[slug]
			// Copy for API use; original retains __REDACTED__ for write-back
			apiObj := make(map[string]any, len(obj))
			maps.Copy(apiObj, obj)
			if injectErr := injectSecrets(apiObj, rt, slug); injectErr != nil {
				return false, injectErr
			}
			id, _ := apiObj["_id"].(string) //nolint:errcheck // empty string on failure is correct
			if id != "" {
				if dryRun {
					fmt.Fprintf(w, "would update %s/%s\n", rt, slug) //nolint:errcheck,revive // writing to stdout
				} else {
					if putErr := c.put(ctx, site, rt, id, apiObj); putErr != nil {
						return false, putErr
					}
					fmt.Fprintf(w, "updated %s/%s\n", rt, slug) //nolint:errcheck,revive // writing to stdout
				}
			} else {
				if dryRun {
					fmt.Fprintf(w, "would create %s/%s\n", rt, slug) //nolint:errcheck,revive // writing to stdout
				} else {
					created, postErr := c.post(ctx, site, rt, apiObj)
					if postErr != nil {
						return false, postErr
					}
					// Merge _id into original local object (preserves __REDACTED__)
					obj["_id"] = created["_id"]
					if writeErr := writeConfigFile(configDir, rt, slug, obj); writeErr != nil {
						return false, writeErr
					}
					fmt.Fprintf(w, "created %s/%s\n", rt, slug) //nolint:errcheck,revive // writing to stdout
				}
			}
		}
	}
	if dryRun {
		return false, nil
	}
	fmt.Fprintln(w, "verifying...") //nolint:errcheck,revive // writing to stdout
	hasDiffs, err := cmdDiff(ctx, c, site, configDir, filterType, color, w)
	if err != nil {
		return false, fmt.Errorf("post-push verification: %w", err)
	}
	if !hasDiffs {
		fmt.Fprintln(w, "verified") //nolint:errcheck,revive // writing to stdout
	}
	return hasDiffs, nil
}

func cmdDiff(
	ctx context.Context, c *client, site, configDir, filterType string,
	color bool, w io.Writer,
) (bool, error) {
	types, err := selectedTypes(filterType)
	if err != nil {
		return false, err
	}
	hasDiffs := false
	for _, rt := range types {
		localFiles, readErr := readConfigFiles(configDir, rt)
		if readErr != nil {
			return false, readErr
		}
		remoteItems, listErr := c.list(ctx, site, rt)
		if listErr != nil {
			return false, listErr
		}

		// Build remote map by slug. Each obj is a fresh decode from the HTTP
		// response, safe to mutate in place for redaction.
		remoteBySlug := make(map[string]map[string]any)
		for _, obj := range remoteItems {
			name, _ := obj["name"].(string) //nolint:errcheck // empty string on failure is correct
			if name == "" {
				continue
			}
			slug := slugify(name)
			if _, exists := remoteBySlug[slug]; exists {
				return false, fmt.Errorf(
					"slug collision in %s: multiple remote resources slugify to %q", rt, slug)
			}
			remoteBySlug[slug] = obj
		}

		allSlugs := make(map[string]bool)
		for slug := range localFiles {
			allSlugs[slug] = true
		}
		for slug := range remoteBySlug {
			allSlugs[slug] = true
		}
		sorted := make([]string, 0, len(allSlugs))
		for slug := range allSlugs {
			sorted = append(sorted, slug)
		}
		slices.Sort(sorted)

		for _, slug := range sorted {
			local, hasLocal := localFiles[slug]
			remote, hasRemote := remoteBySlug[slug]

			// Redact secrets, annotating changes when both sides exist
			switch {
			case hasLocal && hasRemote:
				annotateSecretChanges(local, remote, rt, slug)
			case hasLocal:
				redactSecrets(local, rt)
			case hasRemote:
				redactSecrets(remote, rt)
			default:
			}

			var localLines, remoteLines []string
			if hasLocal {
				localJSON, marshalErr := marshalJSONFn(local)
				if marshalErr != nil {
					return false, marshalErr
				}
				localLines = strings.Split(strings.TrimSuffix(string(localJSON), "\n"), "\n")
			}
			if hasRemote {
				remoteJSON, marshalErr := marshalJSONFn(remote)
				if marshalErr != nil {
					return false, marshalErr
				}
				remoteLines = strings.Split(strings.TrimSuffix(string(remoteJSON), "\n"), "\n")
			}

			ops := computeDiff(remoteLines, localLines)
			output := formatDiff(ops, "live/"+rt+"/"+slug, "local/"+rt+"/"+slug, color)
			if output != "" {
				hasDiffs = true
				fmt.Fprint(w, output) //nolint:errcheck,revive // writing to stdout
			}
		}
	}
	return hasDiffs, nil
}
