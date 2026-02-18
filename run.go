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

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: unifi-sync <pull|push|diff>")
		return 2
	}

	cmd := args[0]
	flagArgs := args[1:]
	var typeFilter, configDir string
	var dryRun bool

	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&typeFilter, "type", "", "resource type filter")
	fs.StringVar(&configDir, "config", "config", "config directory")
	if cmd == "push" {
		fs.BoolVar(&dryRun, "dry-run", false, "show planned changes without executing")
	}

	switch cmd {
	case "pull", "push", "diff":
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
	for _, name := range []string{"UNIFI_URL", "UNIFI_USERNAME", "UNIFI_PASSWORD"} {
		if os.Getenv(name) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		fmt.Fprintf(stderr, "missing required environment variables: %s\n", strings.Join(missing, ", "))
		return 2
	}

	site := os.Getenv("UNIFI_SITE")
	if site == "" {
		site = "default"
	}
	insecure := os.Getenv("UNIFI_INSECURE") == "true"

	ctx := context.Background()
	c := newClient(os.Getenv("UNIFI_URL"), insecure)
	if err := c.login(ctx, os.Getenv("UNIFI_USERNAME"), os.Getenv("UNIFI_PASSWORD")); err != nil {
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
