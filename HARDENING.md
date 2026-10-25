# Hardening Junction

Junction keeps legacy-compatible behavior by default. Set `JUNCTION_HARDENED=1`
in the process environment to opt into the security controls listed below. The
setting applies when Junction loads configuration and starts its listeners; set
it on the service/container environment before launch.

```sh
JUNCTION_HARDENED=1 junction run --config config.toml
```

## Controls enabled in hardened mode

- **SSH proxy identity:** verify SSH host keys using `~/.ssh/known_hosts` or the
  `known_hosts` query parameter in the SSH proxy URL. Add keys only after
  verifying their fingerprints through a trusted channel.
- **Public listeners:** non-loopback SNI and HTTP-header listeners require
  `allow_from` rules or `public_access = true` to explicitly allow all clients.
- **Proxy failures:** unsupported proxy schemes fail startup. UDP routes that
  specify a proxy chain fail instead of falling back to direct traffic.
- **Configuration sources:** remote includes require HTTPS, redirects are
  rejected, and remote fetches and configuration sizes are bounded. Missing,
  malformed, or unmatched includes fail loading instead of being skipped. For a
  temporary HTTP migration, set `JUNCTION_ALLOW_INSECURE_CONFIG_HTTP=1` as well.
- **Fake DNS:** the default listener binds to `127.0.0.1:53`. An explicitly
  non-loopback listener requires `allow_from` client rules, and concurrent DNS
  requests are capped at 256.
- **Resource limits:** TCP, SNI, and HTTP listeners cap concurrent connections
  at 1024 per listener and 64 per source IP. UDP allows 4096 sessions per
  listener and 64 per source IP. HTTP header and upstream response-header waits
  are capped at 30 seconds, upstream connections per host at 100, individual
  proxy dials at 30 seconds, and rewritten request/response bodies at 8 MiB.
- **Configuration validation:** `UDP_BUFFER` must be between 1 and 65507 bytes,
  effective timeouts must be positive, and `flexible-port` requires an explicit
  `allowed_ports` list. Targetless CONNECT requests must include a valid port in
  the authority.

## Additional deployment settings

Hardening mode does not choose an application-specific source allowlist or
destination CIDR policy. Configure `allow_from`, `destination_allow`, and
`destination_block` for the clients and destinations this deployment intends to
serve. Destination CIDR policy is enforced on direct routes; it cannot be
verified through a proxy chain that resolves destinations remotely.

`junction --dry-run` remains a full configuration export and can contain
credentials. Use restrictive file permissions for full exports. Use
`--dry-run --redact-secrets` for sanitized support diagnostics; that output is
not a replayable configuration.
