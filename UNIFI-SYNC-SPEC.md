# unifi-sync Build Spec

A CLI tool that manages UniFi Network Controller configuration by storing
complete API JSON objects locally, editing them, and PUTting them back.

## Problem Statement

The UniFi Network Controller API only supports PUT (full object replacement),
not PATCH. All existing Terraform/OpenTofu providers construct sparse objects
containing only the fields they manage, then PUT them. This zeros out every
unmanaged field, breaking features like IPv6, NAT settings, DHCP guard, and
more.

`unifi-sync` solves this by pulling complete JSON objects from the controller,
storing them as files, and pushing them back as-is. Edits happen directly in
the JSON files, so no fields are lost.

## UniFi API Reference

### Authentication

- Base URL: `https://{host}/api/`
- Auth: `POST /api/login` with body `{"username": "...", "password": "..."}`
- Response sets a session cookie; include it on all subsequent requests
- TLS verification must be skippable (self-signed certs on self-hosted controllers)

### Endpoints

All resource endpoints are site-scoped:

```
GET    /api/s/{site}/rest/{resource_type}        → list all
GET    /api/s/{site}/rest/{resource_type}/{_id}   → get one
PUT    /api/s/{site}/rest/{resource_type}/{_id}   → full replace
POST   /api/s/{site}/rest/{resource_type}         → create new
```

Default site name: `default`

### Response Format

```json
{
  "meta": {"rc": "ok"},
  "data": [...]
}
```

On error, `meta.rc` is not `"ok"` and `meta.msg` contains the error message.

## Resource Types

| Type           | Description        | Secret Fields   |
|----------------|--------------------|-----------------|
| `networkconf`  | Networks / VLANs   | (none)          |
| `wlanconf`     | Wireless networks  | `x_passphrase`  |
| `usergroup`    | User groups        | (none)          |

## CLI Design

```
unifi-sync pull  [-type TYPE]              # fetch from controller -> local JSON
unifi-sync push  [-type TYPE] [-dry-run]   # local JSON -> controller
unifi-sync diff  [-type TYPE]              # compare local vs live
```

All commands operate on the `config/` directory in CWD (or as specified by a
`-config` flag).

### Exit Codes

| Code | Meaning                           |
|------|-----------------------------------|
| 0    | Success / no differences (diff)   |
| 1    | Differences found (diff only)     |
| 2    | Error                             |

## Config Directory Layout

```
config/
  networkconf/
    {slug}.json
  wlanconf/
    {slug}.json
  usergroup/
    {slug}.json
```

File names are slugified from the resource's `name` field: lowercase, spaces
replaced with hyphens, non-alphanumeric characters (except hyphens) removed,
consecutive hyphens collapsed.

Examples:
- `"Acme"` -> `acme.json`
- `"Guest WiFi"` -> `guest-wifi.json`
- `"Default"` -> `default.json`

## JSON Format

- Complete API objects stored as-is (all fields preserved)
- `json.MarshalIndent` with 2-space indent
- Go's `encoding/json` sorts `map[string]any` keys alphabetically by default
- Trailing newline after closing brace
- Secret fields replaced with `"__REDACTED__"` (see below)

## Secret Handling

### On `pull`

Before writing to disk, replace values of known secret fields with the string
`"__REDACTED__"`. This prevents secrets from being committed to version control.

### On `push`

Before PUTting, resolve any `"__REDACTED__"` values from environment variables.

Environment variable naming convention:
```
UNIFI_SECRET_{SLUG}_{FIELD}
```

Where `{SLUG}` is the file slug uppercased with hyphens replaced by underscores,
and `{FIELD}` is the JSON field name uppercased.

Examples:
- `config/wlanconf/acme.json` field `x_passphrase` -> `UNIFI_SECRET_Acme_X_PASSPHRASE`

If any `"__REDACTED__"` value cannot be resolved, `push` must abort with exit
code 2 and print which variables are missing.

### On `diff`

Redact secrets in both local and remote copies before comparing, so diffs never
display secret values.

## `.env` File Loading

On startup, load a `.env` file from CWD if it exists:

- Parse `KEY=VALUE` lines (split on first `=`)
- Skip blank lines and lines starting with `#`
- Trim whitespace from keys and values
- Strip optional surrounding quotes (`"` or `'`) from values
- Environment variables already set take precedence over `.env` values

### Required Variables

| Variable         | Description                        |
|------------------|------------------------------------|
| `UNIFI_URL`      | Controller base URL                |
| `UNIFI_USERNAME` | Controller username                |
| `UNIFI_PASSWORD` | Controller password                |

## Create vs Update Logic

- If `_id` is present in the local JSON -> PUT to update the existing resource
- If `_id` is absent -> POST to create a new resource
- After creating, the user should run `pull` to get the server-assigned `_id`
  and any other server-populated fields

## Implementation Guidance

### Language and Dependencies

- **Language:** Go (latest stable, currently 1.26)
- **Dependencies:** Zero external dependencies. Standard library only:
  - `net/http`, `net/http/cookiejar` -- HTTP client and cookie handling
  - `crypto/tls` -- TLS configuration (skip verify)
  - `encoding/json` -- JSON marshal/unmarshal
  - `os`, `path/filepath`, `io` -- filesystem operations
  - `flag` -- CLI argument parsing
  - `fmt`, `strings`, `bufio` -- string processing
  - `slices` -- sorting/comparison helpers
  - `unicode` -- character classification for slugify

### JSON Handling

Use `map[string]any` throughout. No typed structs for API objects. Treat them
as opaque blobs. This is the core design principle: we preserve every field the
controller returns without needing to know its schema.

Use `json.Decoder` with `UseNumber()` to preserve numeric precision (avoids
float64 round-tripping of integers).

### Diff Output

- Unified diff format
- ANSI colors when stdout is a terminal (check `os.Getenv("TERM") != ""` and
  `os.Getenv("NO_COLOR") == ""`)
- Red for removed lines, green for added lines
- Compare normalized JSON (re-marshal both sides with sorted keys and 2-space
  indent before diffing)

### Error Handling

- Fail fast on any error
- On HTTP errors, print the status code and response body to stderr
- On missing config directory or files, print a clear message

### HTTP Client

- Create a single `http.Client` with a `cookiejar.Jar` for session management
- Set `TLSClientConfig: &tls.Config{InsecureSkipVerify: true}`
- Login once at startup, reuse the session cookie for all requests

## Test Strategy

### Unit Tests

Required unit tests (no network access):

- **Slugify:** various inputs including spaces, special characters, unicode
- **Secret redaction:** verify secret fields are replaced with `"__REDACTED__"`
- **Secret injection:** verify `"__REDACTED__"` is resolved from env vars,
  and that missing vars cause an error
- **`.env` parsing:** comments, blank lines, quoted values, precedence over env
- **JSON normalization:** round-trip through marshal/unmarshal preserves all fields,
  `UseNumber()` preserves integer types

### Integration Tests

- Require a running UniFi controller
- Skip when `UNIFI_URL` is not set (or use a build tag)
- Test full pull/push/diff cycle
