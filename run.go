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

// isTerminal reports whether w writes to a character device (a terminal). It is
// used to suppress ANSI color when output is redirected to a file or pipe.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok || f == nil {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// isTerminalFn is a package var so tests can simulate terminal vs non-terminal
// output independently of the buffers they pass to run.
var isTerminalFn = isTerminal

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

	color := os.Getenv("TERM") != "" && os.Getenv("NO_COLOR") == "" && isTerminalFn(stdout)

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
