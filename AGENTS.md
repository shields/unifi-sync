# AGENTS.md

This file provides guidance to AI agents when working with code in this repository.

## Build & Test Commands

```bash
make fmt                    # Format code (gofumpt + prettier)
make lint                   # Run golangci-lint
make test                   # Run tests with race detector + 100% coverage check
make build                  # Build binary
make all                    # deps + fmt + lint + test + build
make deps                   # Download and verify dependencies
make dev                    # fmt + lint + test + build
make check                  # deps + fmt + lint + test
make pre-commit             # fmt + lint + test (matches lefthook)
make coverage-html          # Generate HTML coverage report
make help                   # List all targets
```

Individual commands:

```bash
go test -v ./...            # Verbose test output
go test -run TestFuncName   # Run a single test
```

## After Every Change

After any code change, run the full check before considering the work done:

```bash
make all
```

This downloads dependencies, formats with gofumpt and prettier, lints with golangci-lint (all linters enabled), runs tests with `-race`, enforces 100.0% statement coverage, and builds the binary. All must pass cleanly.

## Architecture

unifi-sync is a CLI tool that synchronizes UniFi network controller configurations to/from local JSON files. It uses only Go standard library (zero external dependencies) and lives in a single package.

### Data Flow

The controller API returns resources wrapped in `{"meta":{...},"data":[...]}` envelopes. On **pull**, resources are fetched, secrets are redacted (replaced with `"__REDACTED__"`), and each resource is written to `<configDir>/<resourceType>/<slug>.json`. On **push**, local files are read, secrets are injected from environment variables (`UNIFI_SYNC_SECRET_<SLUG>_<FIELD>`), and resources are PUT (update) or POST (create, when `_id` is absent). On **diff**, local and remote are compared with secrets handled specially.

### Key Components

- **`run.go`** — CLI entry point: flag parsing, command dispatch, exit codes (0=success, 1=diff found, 2=error)
- **`commands.go`** — `cmdPull`, `cmdPush`, `cmdDiff` orchestration
- **`client.go`** — HTTP client with cookie-jar auth, CSRF token (thread-safe via RWMutex), TLS/proxy support
- **`secret.go`** — Redacts secrets on pull, injects from env vars on push. Secret fields are hardcoded per resource type in `resourceSecrets`: scalar `wlanconf` WiFi secrets (`x_passphrase`, `x_iapp_key`, `x_wep`, …) and `networkconf` VPN/PPPoE/WireGuard secrets (`x_wan_password`, `x_ipsec_pre_shared_key`, `wireguard_client_preshared_key`, …). Public certs/keys (`x_ca_crt`, `x_dh_key`, …) are intentionally not redacted. Per-SSID WiFi keys nested in arrays (`private_preshared_keys[].password`, `sae_psk[].psk`) are carried in each spec's `nested` list and keyed by array index in the env var (`..._PRIVATE_PRESHARED_KEYS_0_PASSWORD`). On push, the local object is deep-copied before secret injection so the on-disk file keeps `__REDACTED__`
- **`diff.go`** — LCS-based line diff with ANSI color support
- **`config.go`** — Reads/writes per-resource JSON files organized by type and slug
- **`json.go`** — JSON helpers using `json.Number` to preserve numeric precision
- **`slug.go`** — Converts resource names to filesystem-safe slugs
- **`dotenv.go`** — Loads `.env` files, only sets vars not already in environment

### Testing Patterns

Tests use `httptest.NewServer` for mock API servers and table-driven test patterns. Functions like `osExit`, `setenvFunc`, and `marshalJSONFn` are package-level variables to enable test injection. Resource types are: `networkconf`, `wlanconf`, `usergroup`.

### Environment Variables

Required: `UNIFI_SYNC_URL`, `UNIFI_SYNC_USERNAME`, `UNIFI_SYNC_PASSWORD`
Optional: `UNIFI_SYNC_SITE` (default: "default"), `UNIFI_SYNC_INSECURE_SKIP_TLS_VERIFY` ("true" to skip TLS verify), `TERM`/`NO_COLOR` (color control; color also requires stdout to be a terminal)
