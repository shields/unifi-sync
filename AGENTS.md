# AGENTS.md

This file provides guidance to AI agents when working with code in this repository.

## Build & Test Commands

```bash
go build                    # Build binary
go test ./...               # Run all tests
go test -v ./...            # Verbose test output
go test -run TestFuncName   # Run a single test
go test -cover ./...        # Test with coverage
go test -race ./...         # Race condition detection
go vet ./...                # Static analysis
gofmt -l *.go               # Check formatting
```

## After Every Change

After any code change, run format/lint and tests before considering the work done:

```bash
gofmt -w *.go               # Format code
go vet ./...                 # Lint / static analysis
go test -cover ./...         # Run tests and verify 100.0% coverage
```

All three must pass cleanly. Test coverage must remain at 100.0% of statements — add or update tests as needed to maintain this.

## Architecture

unifi-sync is a CLI tool that synchronizes UniFi network controller configurations to/from local JSON files. It uses only Go standard library (zero external dependencies) and lives in a single package.

### Data Flow

The controller API returns resources wrapped in `{"meta":{...},"data":[...]}` envelopes. On **pull**, resources are fetched, secrets are redacted (replaced with `"__REDACTED__"`), and each resource is written to `<configDir>/<resourceType>/<slug>.json`. On **push**, local files are read, secrets are injected from environment variables (`UNIFI_SYNC_SECRET_<SLUG>_<FIELD>`), and resources are PUT (update) or POST (create, when `_id` is absent). On **diff**, local and remote are compared with secrets handled specially.

### Key Components

- **`run.go`** — CLI entry point: flag parsing, command dispatch, exit codes (0=success, 1=diff found, 2=error)
- **`commands.go`** — `cmdPull`, `cmdPush`, `cmdDiff` orchestration
- **`client.go`** — HTTP client with cookie-jar auth, CSRF token (thread-safe via RWMutex), TLS/proxy support
- **`secret.go`** — Redacts secrets on pull, injects from env vars on push. Secret fields are hardcoded in `secretFields` (e.g. `x_passphrase`, `x_iapp_key`, `x_wep`, `x_wep_key`, `x_radius_secret_1`)
- **`diff.go`** — LCS-based line diff with ANSI color support
- **`config.go`** — Reads/writes per-resource JSON files organized by type and slug
- **`json.go`** — JSON helpers using `json.Number` to preserve numeric precision
- **`slug.go`** — Converts resource names to filesystem-safe slugs
- **`dotenv.go`** — Loads `.env` files, only sets vars not already in environment

### Testing Patterns

Tests use `httptest.NewServer` for mock API servers and table-driven test patterns. Functions like `osExit`, `setenvFunc`, and `marshalJSONFn` are package-level variables to enable test injection. Resource types are: `networkconf`, `wlanconf`, `usergroup`.

### Environment Variables

Required: `UNIFI_SYNC_URL`, `UNIFI_SYNC_USERNAME`, `UNIFI_SYNC_PASSWORD`
Optional: `UNIFI_SYNC_SITE` (default: "default"), `UNIFI_SYNC_INSECURE_SKIP_TLS_VERIFY` ("true" to skip TLS verify), `TERM`/`NO_COLOR` (color control)
