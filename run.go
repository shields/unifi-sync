package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

const cmdNamePush = "push"

var dotenvPath = ".env"

func printUsage(w io.Writer) {
	fmt.Fprint(w, //nolint:errcheck,revive // writing to stdout/stderr
		"usage: unifi-sync <command> [flags]\n"+
			"\n"+
			"Commands:\n"+
			"  pull   fetch remote config and write to local files\n"+
			"  push   upload local config to the controller\n"+
			"  diff   compare local config with remote\n"+
			"\n"+
			"Flags:\n"+
			"  -config string   config directory (default \"config\")\n"+
			"  -type string     resource type filter\n"+
			"\n"+
			"Push-only flags:\n"+
			"  -dry-run         show planned changes without executing\n"+
			"\n"+
			"Environment variables:\n"+
			"  UNIFI_SYNC_URL                       controller URL (required)\n"+
			"  UNIFI_SYNC_USERNAME                  login username (required)\n"+
			"  UNIFI_SYNC_PASSWORD                  login password (required)\n"+
			"  UNIFI_SYNC_SITE                      site name (default \"default\")\n"+
			"  UNIFI_SYNC_INSECURE_SKIP_TLS_VERIFY  set \"true\" to skip TLS verification\n"+
			"  NO_COLOR                             disable colored output\n")
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
	if cmd == cmdNamePush {
		fs.BoolVar(&dryRun, "dry-run", false, "show planned changes without executing")
	}

	switch cmd {
	case "pull", cmdNamePush, "diff":
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
		//nolint:gosec // cmd is from CLI args, not rendered in HTML
		fmt.Fprintf(stderr, "unknown command: %s\n", cmd) //nolint:errcheck,revive // writing to stderr
		return 2
	}

	if err := loadDotenv(dotenvPath); err != nil {
		fmt.Fprintln(stderr, err) //nolint:errcheck,revive // writing to stderr
		return 2
	}

	var missing []string
	for _, name := range []string{"UNIFI_SYNC_URL", "UNIFI_SYNC_USERNAME", "UNIFI_SYNC_PASSWORD"} {
		if os.Getenv(name) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		fmt.Fprintf(stderr, //nolint:errcheck,revive // writing to stderr
			"missing required environment variables: %s\n",
			strings.Join(missing, ", "))
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
		fmt.Fprintln(stderr, err) //nolint:errcheck,revive // writing to stderr
		return 2
	}

	color := os.Getenv("TERM") != "" && os.Getenv("NO_COLOR") == ""

	switch cmd {
	case "pull":
		if err := cmdPull(ctx, c, site, configDir, typeFilter, stdout); err != nil {
			fmt.Fprintln(stderr, err) //nolint:errcheck,revive // writing to stderr
			return 2
		}
	case cmdNamePush:
		hasDiffs, err := cmdPush(ctx, c, site, configDir, typeFilter, dryRun, color, stdout)
		if err != nil {
			fmt.Fprintln(stderr, err) //nolint:errcheck,revive // writing to stderr
			return 2
		}
		if hasDiffs {
			//nolint:errcheck,revive // writing to stderr
			fmt.Fprintln(stderr, "push succeeded but verification found differences")
			return 1
		}
	case "diff":
		hasDiffs, err := cmdDiff(ctx, c, site, configDir, typeFilter, color, stdout)
		if err != nil {
			fmt.Fprintln(stderr, err) //nolint:errcheck,revive // writing to stderr
			return 2
		}
		if hasDiffs {
			return 1
		}
	default:
	}
	return 0
}
