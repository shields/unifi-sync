# unifi-sync

Synchronize UniFi network controller configurations to and from local JSON
files. Supports pulling config snapshots, pushing changes back, and diffing
local versus remote state.

This tool operates on complete configs, so unlike Terraform/OpenTofu providers,
it does not require updating for new fields, and it cannot clobber unknown
fields when writing back. It reads back the config after each write in
order to verify that there are no diffs.

It typically operates in under 500 ms per device.

## Install

```
go install msrl.dev/unifi-sync@latest
```

Or build from source:

```
git clone https://github.com/shields/unifi-sync.git
cd unifi-sync
go build
```

## Quick start

Create a `.env` file with your controller credentials:

```env
UNIFI_SYNC_URL=https://192.168.1.1:8443
UNIFI_SYNC_USERNAME=admin
UNIFI_SYNC_PASSWORD=secretpass
```

Pull the current configuration:

```
unifi-sync pull
```

This writes JSON files to `config/` organized by resource type:

```
config/
  networkconf/
    lan.json
    guest-network.json
  wlanconf/
    home-wifi.json
    guest-wifi.json
  usergroup/
    default.json
```

Review changes before pushing:

```
unifi-sync diff
unifi-sync push -dry-run
unifi-sync push
```

## Commands

| Command | Description                                    |
| ------- | ---------------------------------------------- |
| `pull`  | Fetch remote config and write to local files   |
| `push`  | Upload local config to the controller          |
| `diff`  | Compare local config with remote               |
| `help`  | Show usage (`-h`, `-help`, `--help` also work) |

### Flags

```
-config string   Config directory (default "config")
-type string     Resource type filter (networkconf, wlanconf, usergroup)
```

Push-only:

```
-dry-run         Show planned changes without executing
```

### Exit codes

| Code | Meaning                                                         |
| ---- | --------------------------------------------------------------- |
| 0    | Success                                                         |
| 1    | Differences found (diff), or post-push verification found drift |
| 2    | Error                                                           |

## Environment variables

| Variable                              | Required | Description                                      |
| ------------------------------------- | -------- | ------------------------------------------------ |
| `UNIFI_SYNC_URL`                      | Yes      | Controller URL (e.g. `https://192.168.1.1:8443`) |
| `UNIFI_SYNC_USERNAME`                 | Yes      | Login username                                   |
| `UNIFI_SYNC_PASSWORD`                 | Yes      | Login password                                   |
| `UNIFI_SYNC_SITE`                     | No       | Site name (default: `default`)                   |
| `UNIFI_SYNC_INSECURE_SKIP_TLS_VERIFY` | No       | Set `true` to skip TLS certificate verification  |
| `NO_COLOR`                            | No       | Disable colored diff output                      |

A `.env` file in the current directory is loaded automatically. Variables
already set in the environment take precedence.

## Secret handling

WiFi secrets (`x_passphrase`, `x_iapp_key`, etc.) are automatically redacted to
`"__REDACTED__"` on pull so they can be safely committed to version control.

On push, secrets are injected from environment variables following the naming
convention:

```
UNIFI_SYNC_SECRET_<SLUG>_<FIELD>
```

For example, a network with slug `home-wifi` and field `x_passphrase` reads
from:

```
UNIFI_SYNC_SECRET_HOME_WIFI_X_PASSPHRASE=actualpassword
```

On diff, if a secret's effective value (from the environment) differs from the
remote, it displays as `"__REDACTED__ (changed)"`.

## Resource types

- **networkconf** — Network configurations
- **wlanconf** — WiFi network configurations
- **usergroup** — User groups

Filter to a single type with `-type`:

```
unifi-sync pull -type wlanconf
```

## Post-push verification

After a successful push, unifi-sync automatically pulls the remote config and
diffs it against local files. If differences are found, it exits with code 1 and
prints a warning to stderr. This catches cases where the controller modifies or
rejects values. Dry-run skips verification.

## Proxy support

Standard proxy environment variables (`HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY`)
are respected.
