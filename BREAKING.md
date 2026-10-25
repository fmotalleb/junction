# Breaking changes and migration steps

This file lists configuration or deployment changes required when upgrading to
the reliability and security fixes documented in `PLAN.md`. Apply the relevant
steps before deploying the new version.

## SSH proxy host verification

SSH proxy connections now verify the server host key. Before starting Junction,
install each SSH proxy's verified host key in `~/.ssh/known_hosts` or configure a
`known_hosts` file in the SSH proxy URL. Obtain the fingerprint through a trusted
channel and verify it before adding the key; an unverified `ssh-keyscan` result
does not establish trust.

## Public SNI and HTTP listeners

An unrestricted non-loopback `sni` or `http-header` listener must now declare
that it is intentionally public or restrict its clients. Add an `allow_from`
policy to limit access, or set `public_access = true` to allow any client. For
example:

```toml
[[entrypoints]]
routing = "sni"
listen = "0.0.0.0:8443"
public_access = true
```

Loopback listeners and listeners with `allow_from` rules do not need this opt-in.

## Proxy configuration

- Only `socks5` and `ssh` proxy schemes are supported. Replace unsupported or
  misspelled schemes; Junction now fails startup instead of silently routing
  directly.
- `udp-raw` does not support proxy chains. Remove `proxy` from those entries or
  use a deployment path that supports UDP proxying.
- `destination_allow` and `destination_block` policies are enforced on direct
  routes and cannot be combined with a proxy chain. Choose direct routing with
  the CIDR policy, or enforce the destination policy at the proxy egress.
- If an HTTP or SNI route uses `flexible-port`, add an explicit `allowed_ports`
  list containing every port clients may select with `Junction-Port`.

## Configuration includes

Every listed include must now resolve to a file or remote source that can be
read and parsed. Fix missing paths and unmatched globs; includes are required and
there is no optional-include syntax. Remote includes must use HTTPS. For a
temporary migration from plain HTTP, set `JUNCTION_ALLOW_INSECURE_CONFIG_HTTP=1`
in the process environment, then move the source to HTTPS and remove the
exception.

## Fake DNS listener

Fake DNS now binds to `127.0.0.1:53` by default. If other machines need to query
it, configure an explicit reachable `listen` address and set `allow_from` to the
intended client ranges. Do not expose an unrestricted DNS forwarder.

## Limits and timeout validation

- TCP, SNI, and HTTP listeners now cap concurrent connections at 1024 per
  listener and 64 per source IP. UDP allows up to 4096 sessions per listener and
  64 per source IP. Deployments that need higher concurrency should distribute
  traffic across instances or listeners.
- `UDP_BUFFER`, when set, must be an integer from 1 through 65507. Correct or
  remove any invalid value.
- The effective entry timeout, including the `TIMEOUT` environment fallback,
  must be positive. Set a positive duration or unset an invalid `TIMEOUT` value.
- If an HTTP CONNECT client sends an authority without a port, configure a fixed
  route `to` port or update the client to send the standard `host:port` authority.

## Includes and diagnostics

`junction --dry-run` remains a full configuration export and can contain
credentials. Protect its output as a secret (for example, use file mode `0600`).
For support diagnostics, use `--dry-run --redact-secrets`; the redacted output is
not intended to be loaded as a working configuration.
