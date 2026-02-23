package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

var dotenvPath = ".env"

func printUsage(w io.Writer) {
	fmt.Fprint(w, `usage: unifi-sync <command> [flags]

Commands:
  pull   fetch remote config and write to local files
  push   upload local config to the controller
  diff   compare local config with remote

Flags:
  -config string   config directory (default "config")
  -type string     resource type filter

Push-only flags:
  -dry-run         show planned changes without executing

Environment variables:
  UNIFI_SYNC_URL                       controller URL (required)
  UNIFI_SYNC_USERNAME                  login username (required)
  UNIFI_SYNC_PASSWORD                  login password (required)
  UNIFI_SYNC_SITE                      site name (default "default")
  UNIFI_SYNC_INSECURE_SKIP_TLS_VERIFY  set "true" to skip TLS verification
  NO_COLOR                             disable colored output
`)
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}

	cmd := args[0]
	if cmd == "-h" || cmd == "-help" || cmd == "--help" || cmd == "help" {
		printUsage(stdout)
		return 0
	}

	flagArgs := args[1:]
	var typeFilter, configDir string
	var dryRun bool

	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.StringVar(&typeFilter, "type", "", "resource type filter")
	fs.StringVar(&configDir, "config", "config", "config directory")
	if cmd == "push" {
		fs.BoolVar(&dryRun, "dry-run", false, "show planned changes without executing")
	}

	switch cmd {
	case "pull", "push", "diff":
		// Direct help to stdout; parse errors to stderr.
		fs.SetOutput(stdout)
		for _, a := range flagArgs {
			if a == "-h" || a == "-help" || a == "--help" {
				fs.Usage()
				return 0
			}
		}
		fs.SetOutput(stderr)
		if err := fs.Parse(flagArgs); err != nil {
			return 2
		}
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n", cmd)
		return 2
	}

	if err := loadDotenv(dotenvPath); err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	var missing []string
	for _, name := range []string{"UNIFI_SYNC_URL", "UNIFI_SYNC_USERNAME", "UNIFI_SYNC_PASSWORD"} {
		if os.Getenv(name) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		fmt.Fprintf(stderr, "missing required environment variables: %s\n", strings.Join(missing, ", "))
		return 2
	}

	site := os.Getenv("UNIFI_SYNC_SITE")
	if site == "" {
		site = "default"
	}
	insecure := os.Getenv("UNIFI_SYNC_INSECURE_SKIP_TLS_VERIFY") == "true"

	ctx := context.Background()
	c := newClient(os.Getenv("UNIFI_SYNC_URL"), insecure)
	if err := c.login(ctx, os.Getenv("UNIFI_SYNC_USERNAME"), os.Getenv("UNIFI_SYNC_PASSWORD")); err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	color := os.Getenv("TERM") != "" && os.Getenv("NO_COLOR") == ""

	switch cmd {
	case "pull":
		if err := cmdPull(ctx, c, site, configDir, typeFilter, stdout); err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
	case "push":
		hasDiffs, err := cmdPush(ctx, c, site, configDir, typeFilter, dryRun, color, stdout)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
		if hasDiffs {
			fmt.Fprintln(stderr, "push succeeded but verification found differences")
			return 1
		}
	case "diff":
		hasDiffs, err := cmdDiff(ctx, c, site, configDir, typeFilter, color, stdout)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
		if hasDiffs {
			return 1
		}
	}
	return 0
}
