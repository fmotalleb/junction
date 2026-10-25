# Reliability and security improvement plan

Reviewed on 2026-10-25 against commit `9d80cc0`.
Status: implementation in progress. The completed items below reflect changes
made in this worktree; unchecked items remain follow-up work.

## Scope and evidence

Reviewed the Go CLI, configuration loader and its pinned `go-tools` dependency,
all five routers, proxy chains, UDP session management, DNS, TLS parsers,
embedded React configuration editor, installer, container configuration, and CI.
The older `review.md` was cross-checked against current source. Its completed
items are not assumed to prove that neighboring lifecycle or policy issues are
resolved: the router registry still uses global group maps, for example.

The service intentionally forwards connections to configurable or client-selected
destinations. The objective is to enforce the operator's routing and access
policy consistently, preserve intentional private-network routing, and bound
resource use. A public proxy deployment and a fixed private backend need
different destination policies.

Current verification:

| Check | Result |
| --- | --- |
| `GOCACHE=/tmp/junction-review-go-cache go test -mod=readonly -race -count=1 -timeout=90s ./...` | Passed with local socket access. The initial sandbox run could not create loopback sockets; that was an environment restriction. |
| `GOCACHE=/tmp/junction-review-go-cache go vet -mod=readonly ./...` | Passed. |
| Frontend `./node_modules/.bin/tsc -b --pretty false` | Passed using the installed frontend dependencies. |
| Frontend `npm run build` | Passed; reported browser compatibility and `eval` warnings in `@iarna/toml`, and stale Browserslist data. These warnings alone do not establish an exploitable vulnerability. |
| Frontend `npm run lint` | Failed: explicit `any` at `src/utils/config.ts:51,85`, and unused catch variable at line 94. |
| Temporary standalone Go probes | Reproduced the TLS parser panic, unsupported proxy acceptance, IPv6 address hook failure, stale DNS configuration after reparse, and suppressed configuration read errors. |
| Temporary Go overlay test with loopback HTTP servers | Confirmed `http-header` follows a redirect to an origin outside its configured allow rule. |

Probes were kept outside the tracked source. They establish the observations
below, not permanent regression coverage. The existing race suite passes but
does not exercise concurrent tagged-route registration or the adversarial inputs.
Frontend checks used the existing installation, not a fresh `npm ci`. No live
dependency advisory scan, deployed-system penetration test, sustained load test,
or container release smoke test was performed; no claim of dependency safety is
made. Other findings below are based on source inspection and are identified as
such.

## Priorities and delivery order

P0 means an externally triggerable process failure to fix first. P1 means a
security boundary or major availability failure. P2 means protocol correctness,
operational resilience, or preventive hardening. Priorities assume the affected
feature is enabled and reachable; exposure should influence rollout order.

| ID | Priority | Change | Delivery phase |
| --- | --- | --- | --- |
| R1 | P0 | Make SNI parsing safe and handle fragmented ClientHello messages | 1 |
| R2 | P1 | Verify SSH host identities | 1 |
| R3 | P1 | Reject unsupported proxies instead of silently dialing directly | 1 |
| R4 | P1 | Enforce destination policy across redirects and connections | 1 |
| R5 | P1 | Compile deterministic, isolated tagged routes | 2 |
| R6 | P1 | Own shutdown, signal handling, and component failures | 2 |
| R7 | P1 | Bound connections, pending work, sessions, and phase timeouts | 2 |
| R8 | P1 | Validate fresh configuration snapshots and fail on load errors | 1–2 |
| R9 | P1 | Bound reverse-proxy body rewriting and decompression | 1 |
| R10 | P2 | Correct HTTP forwarding, CONNECT, and encoding semantics | 3 |
| R11 | P2 | Fix UDP session ownership and preserve TCP half-closes | 3 |
| R12 | P2 | Restrict DNS clients and return accurate DNS failures | 3 |
| R13 | P2 | Protect configuration sources and redact diagnostic secrets | 3 |
| R14 | P2 | Preserve configuration faithfully in the editor | 4 |
| R15 | P2 | Make CI, installation, and release artifacts verifiable | 4 |

Phase 1 should ship small fixes for R1–R4, R9, and configuration error propagation
from R8 without waiting for a broad rewrite. Phase 2 builds the shared runtime
foundation: R8 validation feeds R5 route compilation, and R6 resource ownership
supports R7 limits. Phase 3 builds on those foundations. Phase 4 closes generator
and delivery gaps; small independent CI and installer fixes can land earlier.
Each item should be a reviewable commit or PR with its listed acceptance checks.

## R1 — Safe, complete SNI parsing

**Evidence: reproduced panic; remaining parser behavior inspected.**
`crypto/tls/extract_sni.go:35` checks `pos > len(handshake)` and then indexes
`handshake[pos]`. Equality therefore panics. A 43-byte input with byte 0 set to
`0x16`, byte 5 set to `0x01`, and all other bytes zero reproduces an index-out-of-
range panic. `router/sni.go:175` passes network input directly to this parser in a
goroutine without panic recovery, so this can terminate the process. Untagged
routes apply source rules first; tagged routes read SNI before selecting a rule.

`isTLSHandshake` does not require the ClientHello handshake type. Record and
handshake lengths are not consistently enforced, and extension bounds can exceed
the declared extension block. `readSNI` assumes one 4096-byte `Read` contains the
whole hello, which rejects valid fragmentation and larger ClientHello messages.
The alternate `ClientHello.Unmarshal` implementation also needs length validation.

- [x] Immediately correct the bounds check and add the exact panic input as a regression.
- [x] Validate record type, ClientHello type, record/handshake/vector lengths,
  extension boundaries, and the SNI name before constructing a dial address.
- [x] Use a bounded, deadline-aware reader that assembles the required handshake
  across TCP reads and TLS records. Preserve every consumed byte for replay.
- [x] Test both parser implementations against the same valid/malformed
  single-record corpus. Fuzz targets cover both public parser entry points.

**Acceptance:** all truncation boundaries and arbitrary fuzz inputs return a
result or error without panic; invalid vector lengths are rejected; valid hellos
split at every byte boundary, across records, and larger than 4096 bytes route
successfully; oversized or slow input is rejected within configured limits.

## R2 — SSH server authentication

**Evidence: source inspection.** `proxy/ssh.go:54,141` installs `ignoreHostKey`,
which accepts every server key. A server impersonator can receive SSH password
authentication and intercept the proxy hop. Public-key authentication does not
send the private key, but still requires authentication of the SSH server.

- [x] Support an operator-supplied `known_hosts` file or pinned server key using
  `knownhosts.New` or `ssh.FixedHostKey`; require verification by default.
- [x] Do not retain an insecure mode. Document migration for existing SSH
  configurations: install the SSH server key in a verified `known_hosts` file
  before startup; explain that an unverified `ssh-keyscan` result is not trust.
- [x] Preload and validate trust/authentication material when compiling routes.
  Support bracketed IPv6 addresses using host/port parsing rather than a colon test.
- [x] Close the underlying connection when the dial context is canceled during
  `ssh.NewClientConn`, including when its original deadline has not expired.

**Acceptance:** trusted keys connect; unknown, changed, and wrong-host keys fail;
nonstandard ports and IPv6 work; cancellation during a silent handshake releases
the socket promptly. Logs do not contain passwords or private-key contents.

## R3 — Fail closed for invalid proxy settings

**Evidence: reproduced unsupported scheme; UDP fallback inspected.**
`proxy/dialer.go:28,120` silently skips an unrecognized scheme and keeps the
previous dialer. A probe using `https://127.0.0.1:9999` returns a non-nil chain
without error. With no other hops, traffic goes directly to the destination.
`connection/udp.go:142` explicitly logs a warning and dials directly even when
proxies were requested. Both behaviors can violate an operator's egress policy.

- [x] Reject unknown schemes, nil URL entries, missing hosts, invalid ports,
  and invalid SSH authentication before opening listeners.
- [x] Reject `udp-raw` with a proxy until UDP proxying is implemented and tested.
  Do not silently substitute a direct connection.
- [x] Build a chain once per compiled route and preserve its documented hop order.

**Acceptance:** malformed or unsupported chains prevent startup with a redacted,
entry-specific error; a test verifies that no direct target connection occurs;
supported SOCKS5/SSH chains still traverse every hop in the declared order.

## R4 — Destination policy at every outbound boundary

**Evidence: redirect bypass reproduced; other policy gaps inspected.**
`router/http_header.go:362` uses `http.Client.Do` with the default redirect policy
after checking only the original destination. A loopback test allowed one exact
host/port, redirected to a different port, and reached that disallowed origin.
`isValidHostname` blocks a small set of local names but accepts numeric IPv4
destinations and does not constrain DNS results. SNI forwards its supplied name
without a comparable destination-address policy.

- [x] Return upstream redirects to the client by default, as `http-to-https`
  already does. If redirect following is configurable, reapply all relevant
  host, port, address, and proxy-route restrictions on every hop.
- [x] Separate normalized hostname matching from destination-port policy.
  Define case, trailing-dot, IDNA, and IP-literal behavior consistently for HTTP
  and SNI. `flexible-port` now requires `allowed_ports`, and `Junction-Port`
  overrides are rejected unless the selected route lists the requested port.
- [x] Add an explicit destination CIDR policy for deployments that must exclude
  loopback, private, link-local, metadata, multicast, or unspecified addresses.
  Preserve intentional private backends through explicit configuration.
- [x] For direct connections, validate resolved addresses and dial an approved
  address without resolving it again. For remotely resolving SOCKS/SSH hops,
  document the trust boundary and enforce address policy at the egress hop or
  refuse modes whose promised restriction cannot be enforced locally.
- [x] Require an explicit public-access choice for unrestricted non-loopback
  SNI/HTTP-header listeners. Existing configs without `allow_from` or
  `public_access = true` fail with a migration hint; the UI and examples expose
  the explicit choice.

**Acceptance:** redirects cannot escape allow/block rules; normalized host variants
behave consistently; flexible ports cannot escape policy; controlled DNS answer
changes cannot redirect a direct dial to a forbidden address; explicitly allowed
private targets and supported remote-resolution modes remain usable.

## R5 — Deterministic and isolated route groups

**Evidence: source inspection; now covered by registry tests and the race suite.**
The original global group maps allowed concurrent registration order to control
listener ownership, had unsynchronized reads, and let one registry reset another.
Grouped HTTP forwarding also reused the listener owner's proxy and settings.
Groups are now prepared in config order, owned by each registry, and validated
before listeners start. Each route keeps its own compiled proxy chain and HTTP
transport; HTTPS targets and replacement rules are prepared once per route.

- [x] Compile all groups in configuration order before starting any listeners.
  Validate that a group's entries agree on routing mode and listen address.
- [x] Move groups into the server/registry instance and publish immutable route
  tables; remove global registration/reset state.
- [x] Make selection return a complete compiled route, including target, proxy
  chain, transport, timeout, features, and access policy. Use that route throughout.
- [x] Precompile HTTPS target URLs and replacers rather than rebuilding per request;
  ensure any rewrite ordering is deterministic.

**Acceptance:** repeated runs choose the same overlapping rule; a non-first HTTP
rule uses its own upstream proxy/port/timeout; incompatible groups fail validation;
concurrent servers and reloads cannot erase each other's groups; concurrent routing
and lifecycle tests pass with `-race`.

## R6 — Service lifecycle, failures, and signals

**Evidence: source inspection.** Both HTTP routers block in `ListenAndServe`
without calling `Shutdown` on context cancellation (`http_header.go:98`,
`http_to_https.go:94`). `BaseContext` only cancels requests, not the listener.
`cmd/helper.go:56` subscribes to `os.Kill`, which cannot be caught, and omits
SIGINT/SIGTERM. `server.handleEntry` logs and discards listener errors, while
`Serve` waits for all components before reporting a DNS error. One dead component
can therefore remain hidden while another runs indefinitely.

- [x] Handle SIGINT and SIGTERM with platform-appropriate signal wiring; preserve
  Unix SIGHUP reload semantics. Signal registration is canceled if logger setup fails.
- [ ] Give each listener, transport, tunnel, UDP manager, and worker an owner.
  On cancellation, stop acceptance, drain within a configured grace period, then
  force-close remaining resources and join workers.
- [x] Use explicit HTTP shutdown and close idle transports. CONNECT relay contexts
  now also inherit service cancellation, so hijacked sockets close with the tunnel.
- [x] Propagate required component failures promptly, cancel siblings, and report
  which entry failed. Distinguish permanent configuration errors from retryable
  runtime errors, and treat normal shutdown as success.
- [ ] Publish readiness only after all required listeners bind; expose component
  failure and reload status in a small health/diagnostic interface.

**Acceptance:** process-level SIGTERM/SIGINT tests exit cleanly within the grace
period; SIGHUP rebinds HTTP listeners without hanging; CONNECT and silent TCP/SNI
clients are released; one failed listener or exhausted DNS retry stops required
siblings and returns a useful error rather than leaving partial service.

## R7 — Resource limits and meaningful timeouts

**Evidence: source inspection.** TCP/SNI spawn unrestricted connection goroutines;
`router/udp_raw.go:80` spawns one goroutine per datagram, and the UDP client table
has no capacity limit. A new-client flood can queue behind `createMu` while each
session retains a socket and response buffer. Entry timeouts default to 24 hours
(`config/config.go:61`). HTTP forwarding uses the server context rather than the
incoming request context, and ordinary HTTP requests have no client timeout.
`newProxyTransport` also lacks a response-header timeout and a connection cap.

- [x] Cap concurrent TCP, SNI, and HTTP connections per listener, and cap UDP
  client sessions with rejection behavior.
- [x] Add per-source limits, bound pending dials and UDP queues/workers, and
  preserve packet order within each UDP flow under load.
  - [x] Cap UDP client sessions per source address as well as per listener.
  - [x] Cap concurrent HTTP, raw TCP, and SNI connections per source address.
  - [x] The read loop processes packets serially and session creation is
    serialized, so there is no unbounded application worker or packet queue.
- [ ] Split connect, handshake/header, body-idle, tunnel-idle, and maximum-lifetime
  settings. Apply a dial deadline to HTTP as well as raw TCP/SNI paths.
- [x] Derive outgoing requests from `r.Context()` and the selected route's limits;
  bound transport connections and response-header waits. Support long-lived
  streams deliberately instead of applying a blanket short total timeout.
- [ ] Validate `UDP_BUFFER` and all capacity/duration settings; reject negative,
  zero where invalid, or excessive values before allocating or serving.
  - [x] Validate `UDP_BUFFER` as an integer from 1 through 65507 before binding.
- [ ] Count active connections/sessions, rejected work, dial/handshake failures,
  timeout reasons, and queued work without client/hostname metric labels that
  create unbounded cardinality. Rate-limit repetitive rejection/error logs.

**Acceptance:** slow clients and upstreams release resources at the relevant
deadline; disconnecting an HTTP client cancels its upstream request; controlled
TCP connection and UDP source floods keep descriptors, queue size, and memory
within configured limits; legitimate streams continue making progress.

## R8 — Strict loading and fresh configuration snapshots

**Evidence: reproduced; dependency behavior inspected.** `cmd/root.go:58,71`
reuses `cfg` across reloads; `config.Parse` decodes into the supplied object.
Parsing a configuration with FakeDNS and then one without it retains FakeDNS.
The pinned loader's `config/conf_raw_reader.go` logs and continues after read or
parse errors: probes returned nil for malformed JSON and for a missing required
include, the latter retaining the main entrypoint. An access-policy include could
therefore disappear without preventing startup. Also, the address hook at
`config/hooks.go:41` splits on every colon; invoking it with `[::1]:8443` returns
`no port`.

- [x] Fix or replace the loader's error handling so missing files, malformed
  input, and failed required includes return errors. Optional includes must be
  explicitly marked optional; define deterministic include precedence. Optional
  include syntax is not supported; all includes are required and precedence is
  include order followed by the including file.
- [x] Decode into a fresh local `Config` on every load; removed fields do not
  merge from the previous value.
- [ ] Validate a reload candidate before publishing it to the active runtime.
  Never replace the last working runtime with an invalid candidate.
- [x] Validate all entrypoints before binding: required fields, router type,
  positive durations, target scheme/host/port, proxy compatibility, groups,
  matcher values, and DNS IP/CIDR data. Report unknown security-related keys;
  consider strict decoding for all unknown fields.
- [x] Parse IPv4 and bracketed IPv6 with standard host/port APIs while preserving
  documented loopback defaults for integer/port-only listener forms.
- [ ] Load and validate a reload candidate before stopping the current runtime.
  Keep the last working configuration on invalid input; stage changes and define
  rollback behavior for bind failures. The current one-minute reloader argument
  is a shutdown grace period, not a debounce interval; correct that documentation.

**Acceptance:** missing policy includes and malformed files fail; reload removal
of DNS, proxies, lists, and optional values takes effect; invalid candidates leave
the working runtime intact; IPv6/port-only inputs round-trip; misspelled access
keys cannot silently create an unrestricted route.

## R9 — Bounded body transformation

**Evidence: source inspection.** `router/http_to_https.go:268,309,332` reads whole
request bodies, response bodies, and decompressed gzip output without size limits.
Even unmodified binary responses are buffered. Read/decompression errors are
ignored in some paths. A large upload or compressed upstream response can exhaust
memory, and buffering prevents streaming behavior.

- [x] Stream unchanged content; only buffer content that actually requires a
  configured textual rewrite. Do not treat missing content type as automatic
  permission to rewrite arbitrary binary data.
- [x] Bound rewritten request bytes, upstream transformation bytes, and decoded
  bytes independently. Reject
  oversized uploads with 413 and report upstream transformation failures as 502.
  Read at most limit plus one byte to distinguish truncation from valid input.
- [x] Reject malformed compression and propagate body read errors. Coordinate
  encoded-body handling with R10 so size checks and headers describe actual bytes.

**Acceptance:** oversized requests, responses, gzip expansion, and corrupt gzip
fail within bounded memory; errors never produce a successful partial rewritten
body; large unchanged downloads and event streams deliver incrementally.

## R10 — HTTP protocol fidelity

**Evidence: source inspection.** `http_header.go:318,327` commits 200 before
hijacking and discards the returned buffered reader/writer. CONNECT payload bytes
already buffered by the HTTP server can be lost. `prepareTargetHost` strips the
client's port and can return a bare host when no target port is configured.
Ordinary forwarding copies request/response headers indiscriminately. The HTTPS
router filters a static request header list but not headers nominated by
`Connection`, and its response path drops `Content-Encoding` even for unsupported
encodings such as Brotli. It replaces bytes while retaining other potentially
stale entity metadata.

- [ ] Prefer `httputil.ReverseProxy` with a `Rewrite` hook for ordinary HTTP paths,
  retaining route selection, custom dialing, and bounded response transformations.
  Strip hop-by-hop headers and all fields named by `Connection` in both directions.
- [x] Strip untrusted forwarded identity headers at the shared outbound header
  boundary; the HTTPS router adds its own host/protocol values from the request.
  Hop-by-hop `Proxy-Authorization` is dropped by the same copier.
- [ ] Define default HTTP port and CONNECT authority behavior explicitly, including
  interaction with fixed route ports and `flexible-port`. Preserve escaped paths,
  target base queries, status semantics, trailers, HEAD, and no-body responses.
- [ ] Check hijack support before committing success, preserve and forward buffered
  CONNECT bytes, and register the tunnel with the runtime owner from R6.
- [ ] Preserve untouched content encodings; decode only supported encodings.
  Recalculate outbound request/response framing after rewrites and remove or
  recompute invalidated validators, digests, range metadata, and lengths.

**Acceptance:** CONNECT with headers and initial payload in one write loses no
bytes; default-port and configured-port requests work; connection-nominated
headers and proxy credentials are absent upstream; forged forwarded headers are
sanitized; gzip/Brotli/binary content, escaped paths, HEAD/204/304, trailers,
streaming, and changed-length request bodies behave correctly.

## R11 — UDP session ownership and TCP half-close behavior

**Evidence: source inspection.** UDP cleanup and idle sweeping remove clients by
address key alone (`connection/udp.go:207,242,260`). An old reader can delete a
replacement session for the same address; a stale sweep can remove a session
refreshed after collection. `Cleanup` does not coordinate with pending creation
or join readers. `utils/copy.go:7` closes both connections when one copy finishes,
so a client that half-closes its upload may lose the backend's remaining response.

- [x] Remove UDP sessions only when the map still holds the same session object;
  recheck activity under the removal lock. Coordinate a closed state with creation.
- [x] Close retired UDP sockets immediately to unblock I/O, bound target writes,
  and join readers plus the idle sweeper before cleanup returns. Use context-aware
  target resolution and cancel pending dials before waiting for creation to finish.
- [x] Treat TCP EOF as directional completion: half-close the destination write
  side when supported, allow the reverse copy to drain, then close both ends.
  Define behavior for proxy wrappers without half-close support and retain
  timeout/cancellation escape paths.

**Acceptance:** delayed old readers cannot delete replacements; a refreshed session
survives a stale sweep; cleanup racing with creation leaves zero sockets/workers;
a backend response sent after client `CloseWrite` arrives in full; cancellation
still stops both copy directions promptly.

## R12 — DNS exposure and response semantics

**Evidence: source inspection.** FakeDNS defaults to `0.0.0.0:53`
(`dns/dns.go:110`) and has no source admission policy; `allowed` restricts fake
answer names, not clients permitted to use the forwarder. A reachable deployment
can act as a public UDP DNS forwarder. Forwarder failures become NXDOMAIN at line
204, and unmatched source CIDRs can produce an A record with no valid IPv4 answer.
`newResolvers` also dereferences answer/CIDR pointers without validating all cases.

- [x] Default new configurations to loopback and document the public-listener migration.
- [x] Add explicit allowed-client policy plus a bounded concurrency limit.
- [x] Return SERVFAIL for temporary upstream failures; reserve NXDOMAIN for an
  authoritative negative result. Define REFUSED or empty-answer behavior when
  no source range matches; do not emit an invalid A record.
- [x] Validate nil entries, nil CIDRs, and IPv4-only A answers before serving.
  Normalize DNS case/trailing dots and handle unsupported question classes and
  multiple questions deliberately.
- [x] Test and document truncated upstream response behavior. Upstream lookups
  retry over TCP when a UDP answer is truncated. The local DNS listener remains
  UDP-only, and the README states that limitation.

**Acceptance:** disallowed clients cannot use either fake answers or recursion;
timeout/refusal/NXDOMAIN produce distinct correct responses; unmatched CIDRs and
invalid IP data do not panic or emit malformed answers; large/truncated replies
follow the documented behavior.

## R13 — Trusted configuration sources and secret-safe diagnostics

**Evidence: dependency/source inspection.** `config.Parse` delegates includes to
`go-tools@v0.1.81`. Its `config/readers.go` permits HTTP/basic-auth sources, uses
`http.DefaultClient` and unbounded reads, and logs source URLs verbatim. Its remote
pattern dispatch also needs regression coverage: `mergeFromPattern` tests
`url.Parse` errors rather than successful HTTP URL parsing. CLI dumps deliberately
serialize configuration, and `server/server.go:86` logs the whole entry on errors.
Review those paths for credentials and sensitive extra configuration.

- [x] Treat remote config as privileged input: require HTTPS by default, bound
  fetch time/body size/include depth and count, constrain redirects, and make any
  insecure HTTP exception explicit. Fix remote include dispatch with an upstream
  change or a narrow maintained replacement; avoid mutating the global HTTP client.
- [x] Preserve cycle detection and test normalized path aliases, relative includes,
  deterministic include order, and including-file precedence. Do not accept
  partial source failures (R8).
- [ ] Redact URL userinfo, sensitive query values, and secret-bearing config fields
  in logs/errors. HTTPS route logs now redact URL credentials and query secrets;
  audit decoder and proxy errors for other secret-bearing values.
- [ ] Clearly distinguish diagnostic output from a full secret-bearing export;
  - [x] Document that `--dry-run` is a full export and recommend mode `0600`.
  - [ ] Provide a redacted diagnostic mode.

**Acceptance:** remote HTTPS includes actually load; stalled, oversized, redirected,
or failed sources fail within limits; logs captured using sentinel credentials
contain no secrets; explicitly requested full exports remain usable and documented.

## R14 — Configuration editor fidelity and validation

**Evidence: source inspection and frontend lint execution.**
`internal/front/src/utils/config.ts:53,93` and `App.tsx` reconstruct the top-level
object as only `{entrypoints}`, dropping imported `core` and include settings.
Types/forms omit `http-to-https`, source access controls, tags, features, and extra
configuration. Imported entry properties are spread and may survive, but top-level
fields do not. Editing an imported unsupported routing mode throws. Import only
checks the outer entrypoint array, and IPv4/timeout validation differs from Go.
The YAML exporter imports a transitive package absent from direct dependencies.

- [x] Preserve all top-level and entry fields through import, edits, reordering,
  and export; block or clearly report unsupported edits instead of losing fields.
- [ ] Align types/validation with the backend schema, including all routers,
  source rules, IPv6, Go durations, tag/feature constraints, and HTTPS targets.
  Validate imported elements and bound file size before accepting them.
  - [x] Match SNI/HTTP target ports, raw TCP/UDP host-port targets, and IPv4,
    bracketed IPv6, port-only, and `:port` listen forms to backend validation.
  - [x] Validate imported `public_access` as a boolean and enforce its UI opt-in.
- [x] Add explicit source-access controls to the UI and make routing-type changes
  preserve policy or require a clear review of any removed rules.
- [x] Declare `yaml` directly and resolve the frontend lint errors.
- [ ] Add unit tests for round trips plus a small browser smoke test for
  import/edit/export errors.
  Review the TOML bundling warnings and use an appropriate browser entry point.

**Acceptance:** a full configuration containing DNS, includes, source rules, tags,
and HTTPS routes survives a round trip and parses with the Go loader; malformed
imports show errors without crashing; unsupported data is preserved; lint and
type-checking run in CI and pass.

## R15 — Reproducible checks, safe installation, and runnable releases

**Evidence: source inspection; fresh installation/release not exercised.**
CI builds the frontend but does not explicitly run frontend lint/type-check/tests.
Dependabot omits npm. Actions use mutable tags, including privileged release jobs.
`internal/front/dist/.keep` allows Go builds with no UI assets: compilation alone
does not prove that the generator works. Compose selects nonexistent Docker stage
`standalone` and passes `junction` as an argument despite the binary ENTRYPOINT;
the Dockerfile expects a prebuilt platform-specific binary. The installer continues
without a checksum utility after calling nonexistent `input` and ignoring failure
(`install.sh:62,63`). Its checksum lookup is not an exact filename match.

- [ ] Add explicit frontend lint, type-check, and targeted tests; retain Go vet,
  race tests, gosec, govulncheck, and clean-diff checks. Run bounded parser fuzzing
  on changes and longer fuzz/load/leak jobs on a scheduled CI job.
- [ ] Separate validation from formatting/dependency updates: current `make ci`
  runs `go mod tidy`, lint fixes, spelling rewrites, generation, and installation.
  Make check-only commands report failures without rewriting the checkout.
- [ ] Add npm dependency updates and advisory scanning alongside Go scanning.
  Pin Actions to reviewed commit SHAs, maintain least-privilege permissions, and
  test security-sensitive release changes before publishing. Select a supported
  Node release and keep the build/runtime toolchain policy explicit.
- [x] Fix Compose to consume a produced image or a documented build context;
  align its stage and command with the Dockerfile. Retain the nonroot runtime,
  consider digest-pinned base images, read-only rootfs, dropped capabilities, and
  only required writable mounts. Ensure examples expose only intended ports.
- [ ] Build assets explicitly for releases, clean obsolete hashed assets, and
  smoke-test the packaged binary's generator page and every referenced asset.
  Define a clear error for binaries intentionally built without UI assets.
- [x] Make checksum verification mandatory and match exactly one expected filename
  and valid digest. Add download timeouts, cleanup traps, narrow extraction, and
  atomic destination replacement. Never overwrite a working binary on failure.
- [ ] Generate SBOM/provenance and consider signed artifacts with installer
  verification; a checksum downloaded beside the binary detects corruption but
  does not independently authenticate a compromised release source.

**Acceptance:** a clean checkout produces tested binaries/images with a functioning
generator; the documented Compose path starts; CI detects frontend errors and
dependency findings; installer tests cover missing checksum tools, absent/duplicate
entries, bad digests, download failure, and preservation of the old binary.

## Completion and rollout gates

1. Convert each reproduced defect into a permanent failing regression before its
   implementation fix. Keep happy-path, malformed-input, cancellation, and policy
   tests local and deterministic; use owned-resource counters instead of global
   goroutine counts or fixed startup sleeps.
2. Require the Go race suite, vet, configured lint/security checks, and frontend
   lint/type-check/build to pass. Record advisory scan failures separately from
   unavailable databases; do not interpret an unavailable scanner as a clean scan.
3. Exercise grouped routing, invalid reload rollback, signals, CONNECT, slow peers,
   HTTP compression, and UDP churn in integration tests. Set measurable resource
   budgets for the intended deployment and verify them under representative load.
4. Publish migration notes for SSH trust, rejected proxy schemes/UDP proxy settings,
   strict include errors, public listener admission, DNS defaults, host/port matching,
   and timeout changes. Preserve deliberate private routing and long-lived streams.
5. Canary the release with readiness, reload outcomes, error rates, active sockets,
   and memory monitored. Keep the prior artifact/config available for rollback.

## Reference guidance

These sources support implementation choices; findings above come from this
checkout and the stated probes, not from an advisory claim about a package version.

- [Go SSH host-key verification](https://pkg.go.dev/golang.org/x/crypto/ssh#ClientConfig)
  and [known_hosts callbacks](https://pkg.go.dev/golang.org/x/crypto/ssh/knownhosts#New).
- [Go reverse-proxy request/header handling](https://pkg.go.dev/net/http/httputil#ReverseProxy).
- [Go HTTP server shutdown and hijacked connections](https://pkg.go.dev/net/http#Server.Shutdown).
- [OWASP destination validation, redirects, and DNS policy](https://cheatsheetseries.owasp.org/cheatsheets/Server_Side_Request_Forgery_Prevention_Cheat_Sheet.html).
