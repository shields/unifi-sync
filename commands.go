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
		items, err := c.list(ctx, site, rt)
		if err != nil {
			return err
		}
		slugsSeen := make(map[string]string)
		for _, obj := range items {
			name, _ := obj["name"].(string)
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
			if err := writeConfigFile(configDir, rt, slug, obj); err != nil {
				return err
			}
			fmt.Fprintf(w, "pulled %s/%s\n", rt, slug)
		}
	}
	return nil
}

// cmdPush reads local config files, injects secrets, and sends them to the
// controller. _id present → PUT (update), _id absent → POST (create).
// After POST, the server response is written back to capture the _id.
// After a successful push, it pulls back and diffs to verify the controller
// accepted the configuration as-is. Returns true if verification found diffs.
func cmdPush(ctx context.Context, c *client, site, configDir, filterType string, dryRun, color bool, w io.Writer) (bool, error) {
	types, err := selectedTypes(filterType)
	if err != nil {
		return false, err
	}
	for _, rt := range types {
		files, err := readConfigFiles(configDir, rt)
		if err != nil {
			return false, err
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
			if err := injectSecrets(apiObj, rt, slug); err != nil {
				return false, err
			}
			id, _ := apiObj["_id"].(string)
			if id != "" {
				if dryRun {
					fmt.Fprintf(w, "would update %s/%s\n", rt, slug)
				} else {
					if err := c.put(ctx, site, rt, id, apiObj); err != nil {
						return false, err
					}
					fmt.Fprintf(w, "updated %s/%s\n", rt, slug)
				}
			} else {
				if dryRun {
					fmt.Fprintf(w, "would create %s/%s\n", rt, slug)
				} else {
					created, err := c.post(ctx, site, rt, apiObj)
					if err != nil {
						return false, err
					}
					// Merge _id into original local object (preserves __REDACTED__)
					obj["_id"] = created["_id"]
					if err := writeConfigFile(configDir, rt, slug, obj); err != nil {
						return false, err
					}
					fmt.Fprintf(w, "created %s/%s\n", rt, slug)
				}
			}
		}
	}
	if dryRun {
		return false, nil
	}
	fmt.Fprintln(w, "verifying...")
	hasDiffs, err := cmdDiff(ctx, c, site, configDir, filterType, color, w)
	if err != nil {
		return false, fmt.Errorf("post-push verification: %w", err)
	}
	if !hasDiffs {
		fmt.Fprintln(w, "verified")
	}
	return hasDiffs, nil
}

func cmdDiff(ctx context.Context, c *client, site, configDir, filterType string, color bool, w io.Writer) (bool, error) {
	types, err := selectedTypes(filterType)
	if err != nil {
		return false, err
	}
	hasDiffs := false
	for _, rt := range types {
		localFiles, err := readConfigFiles(configDir, rt)
		if err != nil {
			return false, err
		}
		remoteItems, err := c.list(ctx, site, rt)
		if err != nil {
			return false, err
		}

		// Build remote map by slug. Each obj is a fresh decode from the HTTP
		// response, safe to mutate in place for redaction.
		remoteBySlug := make(map[string]map[string]any)
		for _, obj := range remoteItems {
			name, _ := obj["name"].(string)
			if name == "" {
				continue
			}
			slug := slugify(name)
			if _, exists := remoteBySlug[slug]; exists {
				return false, fmt.Errorf("slug collision in %s: multiple remote resources slugify to %q", rt, slug)
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
			if hasLocal && hasRemote {
				annotateSecretChanges(local, remote, rt, slug)
			} else if hasLocal {
				redactSecrets(local, rt)
			} else if hasRemote {
				redactSecrets(remote, rt)
			}

			var localLines, remoteLines []string
			if hasLocal {
				localJSON, err := marshalJSONFn(local)
				if err != nil {
					return false, err
				}
				localLines = strings.Split(strings.TrimSuffix(string(localJSON), "\n"), "\n")
			}
			if hasRemote {
				remoteJSON, err := marshalJSONFn(remote)
				if err != nil {
					return false, err
				}
				remoteLines = strings.Split(strings.TrimSuffix(string(remoteJSON), "\n"), "\n")
			}

			ops := computeDiff(remoteLines, localLines)
			output := formatDiff(ops, "live/"+rt+"/"+slug, "local/"+rt+"/"+slug, color)
			if output != "" {
				hasDiffs = true
				fmt.Fprint(w, output)
			}
		}
	}
	return hasDiffs, nil
}
