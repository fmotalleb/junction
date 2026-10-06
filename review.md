# Junction — Project Review

**Reviewed:** 2026-10-06 at `f63df89`; H1 and H2 fixed in the same session (changes below are uncommitted)
**Scope:** Go codebase (35 files, ~4.2k LOC), embedded React config UI, build/CI, tests, docs
**Method:** code reading, `go build ./...`, `go vet ./...`, `go test ./... -race`, `golangci-lint`, `go tool cover`, fresh-clone smoke test in a separate git worktree

---

## 1. Verdict

Junction is a small, focused and well-documented reverse proxy with clear package
boundaries and an unusually strict lint configuration for a project of this size.
Configuration ergonomics (multi-format, decode hooks, validation, `--dry-run`) and
release automation (goreleaser + CodeQL + dependabot) are strong.

The weak spots were **build onboarding** (a fresh clone did not compile) and
**toolchain drift** (the pinned `golangci-lint` crashed once everything targeted Go
1.27) — both **fixed in this session (H1, H2)** — plus **test coverage** (22.9%, with
five packages at 0%) and **long-running resource lifecycle**, which historically leaked
sockets/goroutines until the process had to be restarted: the 12-commit series before
this review fixed that class of bug, but the surrounding patterns (per-connection
goroutines without owners, global mutable state) still invite regressions.

With H1 and H2 closed, the remaining blocker for `make ci` is **L6** — two lint
findings (`goconst` in `router/http_header_test.go`, `gocyclo` 27 in
`(*httpToHTTPSProxy).ServeHTTP`) that the linter now reports instead of crashing.

---

## 2. Architecture

| Package | Role | LOC | Tests |
|---|---|---:|---|
| `cmd/` | cobra CLI (`run`, `dump`, `generator`, `example`) | ~400 | none |
| `config/` | config model, decode hooks, include files, matchers | ~570 | none |
| `server/` | composition root: entrypoints + FakeDNS + retry/backoff | 88 | none |
| `router/` | the five routers: sni, http-header, http-to-https, tcp-raw, udp-raw | ~1,500 | partial |
| `proxy/` | SOCKS5/SSH dialers, dial chain with deadlines | ~450 | good |
| `connection/` | UDP client manager (per-client conn, idle cleanup) | 246 | none |
| `dns/` | FakeDNS + CIDR-based answers + upstream forwarder | 255 | none |
| `crypto/tls/` | zero-alloc ClientHello/SNI parser (+ benchmarks) | ~300 | good |
| `internal/front/` | Vite/React config generator UI, embedded via `go:embed` | TS/React | none |
| `utils/`, `main.go` | `Copy` helper, entrypoint | small | none |

**Request path:** `cmd` → `config.Parse` → `server.Serve` → `router.Handle` (first
matching `init()`-registered handler) → per-connection goroutine → `dialTarget` →
`proxy.Chain` (optional SOCKS5/SSH hops) → `relayTraffic` (bidirectional `utils.Copy`
under `errgroup`, guarded by `ctx`).

**Design notes:** handler selection is a global slice built at `init()` time
(`router/router.go:17`), grouped `sni`/`http-*` entrypoints share one listener through
package-level maps (`router/http_to_https.go:26`), and `router.Reset()` undoes that
global state on shutdown. It works, but it is process-global mutable state rather than
a value threaded through the call graph.

---

## 3. Strengths

- **Small, readable code.** ~4.2k lines for a multi-protocol proxy; functions are
  short, logging is consistent (zap + `log.FromContext`), errors are wrapped.
- **Strict quality gates.** `.golangci.yml` enables ~40 linters including `noctx`,
  `errorlint`, `govet enable-all`, `funlen`, `gocyclo`, plus `gofumpt`/`goimports`
  with a local prefix; CI runs lint **and** `git diff --exit-code` so `--fix`
  cannot silently rewrite code.
- **CI/release coverage.** `build.yml` (build+lint+test), `codeql.yml`, `release.yml`
  (goreleaser → GitHub Releases + GHCR), `dependabot.yml`.
- **Docs.** README carries badges, install paths (with a review-the-script warning),
  a full configuration reference, feature flags and examples; `crypto/tls/BENCHMARK.md`
  documents parser performance.
- **Tests were just added where they matter most.** Leak regression tests
  (`router/resource_test.go`, `router/http_transport_test.go`,
  `proxy/dialer_test.go`, `proxy/ssh_dialer_test.go`) and the suite is race-clean.
- **Careful installer.** `install.sh` verifies SHA-256 when available, uses
  `set -euo pipefail`, does not require root.
- **Secrets hygiene.** `.env`, `*.local.*` and `dist/` are gitignored; nothing
  sensitive found in tracked config files (only the placeholder
  `ssh://test:password@test2:22` example).

---

## 4. Findings

### High

**H1 — A fresh clone does not build.**
`internal/front/serve.go:18` embeds `dist/*`, but `dist/` is gitignored
(`.gitignore:20`). Verified in a clean worktree:

```
$ git worktree add /tmp/fresh HEAD && cd /tmp/fresh && go build ./...
internal/front/serve.go:18:12: pattern dist/*: no matching files found
exit=1
```

This also breaks `go vet`, `go test` and lint for anyone who has not run the frontend
build first.

**Status: fixed.** Committed placeholder `internal/front/dist/.keep` (re-included via
`!dist/.keep` in `internal/front/.gitignore`), embed switched to `//go:embed all:dist`
so the dot-file counts, and `build.emptyOutDir: false` added to `vite.config.ts` so
`vite build` does not delete the placeholder (otherwise CI's `gen` + `diff` steps would
see a deleted tracked file). Verified: clean worktree → `go build ./...` and
`go vet ./...` pass; `npm run build` leaves `dist/.keep` in place and the tree clean.

**H2 — `make lint` / `go tool golangci-lint` crashes on Go 1.27 (now CI-breaking).**
`go.mod` pins golangci-lint `v2.12.2` via the `tool` directive; it (bundled staticcheck
`v0.7.0`) panics while building IR for stdlib `internal/poll` under Go 1.27.1:

```
level=error msg="Running error: can't run linter goanalysis_metalinter
  buildir: package \"poll\": unexpected expr: *ast.KeyValueExpr"
exit=3
```

Until `4730faf` this was survivable because `go.mod` said `go 1.26.0` and the workflows
pinned `1.26`. After the upgrade both the module (`go 1.27.1`) and CI
(`build.yml`/`codeql.yml`/`release.yml` → `go-version: "1.27"`) target Go 1.27, so
`make ci` will now fail at the `lint` step on every push — while the system binary
(`golangci-lint 2.13.2`) still runs fine and reports findings, proving the config is
not at fault.

**Status: fixed.** `go get github.com/golangci/golangci-lint/v2@v2.13.2 && go mod tidy`
pulled staticcheck `v0.8.1`; the panic is gone and `go tool golangci-lint run ./...`
now completes (exit 1 = findings) both in the main tree and in a clean worktree.
It reports the same 2 findings as the system binary — no new ones from the bump.

### Medium

**M1 — Test coverage is 22.9%, with five packages at 0%.**
`config`, `dns`, `server`, `connection` and `utils` have no tests at all; `router` is at
24.9% and `proxy` at 47.9%. The untested packages are exactly the ones that own
lifecycle decisions (backoff/panic in `server.runDNS`, idle cleanup in
`connection/udp.go`, matcher semantics in `config.EntryPoint.Allowed*`). *Fix:* add
table tests for `config` matchers/validation and `dns` answer selection — both are pure
functions and cheap to test.
**Status: fixed.** Table tests now cover the pure lifecycle/decision code:
`config/config_test.go` (allow/block matchers for hosts and clients, timeout and
target defaults, the one-liner `Decode` form), `dns/dns_test.go` (resolver
validation, CIDR answer selection, `IsAllowed`, `Serve` config validation and
cancellation), `server/server_test.go` (open-listener warning, empty-config
behavior) and `connection/udp_test.go` (idle sweep). Total coverage 22.9% →
**41.0%**; `config` 0 → 44.2%, `dns` 0 → 47.6%, `server` 0 → 45.9%,
`connection` 0 → 86.5%.

Writing those tests also flushed out three real defects, all fixed:
`EntryPoint.Decode` silently dropped every proxy in the 4/5-segment one-liner form
(`result["proxy"] = append(r, proxy)` was then overwritten by the empty `r`),
README advertised a `regexp:` matcher prefix the matcher package rejects (now
`regex:`), and `dns.Serve` never closed its listener on cancellation — the
close-on-`ctx.Done` goroutine was commented out, so shutdown hung in
`server.Serve`.

**M2 — Lifecycle is still owned by many anonymous goroutines.**
The recent fixes bound dials, handshakes and relays, but the pattern remains:
`connection/udp.go:121-124` spawns one response reader *and* one cleanup timer per UDP
client key, and `internal/front/serve.go:30` takes a `ctx` it never uses —
`server.ListenAndServe()` at `internal/front/serve.go:52` never sees cancellation and
never calls `Shutdown`. *Fix:* prefer a single sweep loop over per-client timers, and
wire `ctx` to `http.Server.Shutdown`.
**Status: fixed.** `front.Serve` now owns a private mux, honors `ctx` by calling
`http.Server.Shutdown` with a 5s grace period and drops `ConnState` to `Debug`.
The UDP side uses one manager-level sweeper (`sweepIdleClients`) instead of a timer
goroutine per client key.

**M3 — `gosec` and `mnd` are explicitly disabled** (`.golangci.yml:22,26`). The project
does compensate with CodeQL and `make vuln` (govulncheck), so the risk is acceptable,
but a network-facing proxy is the kind of code where `gosec` earns its keep (int conversions
for ports, `http.ListenAndServe` on user input, etc.). *Fix:* re-enable `gosec` with a
narrow exclusion list, or record why it was turned off.
**Status: fixed.** `gosec` is enabled in `.golangci.yml` (`mnd` stays disabled) and
the run is clean: `G112` fixed on the test HTTP server, while `G304`
(operator-supplied SSH key path), `G106` (in-process SSH fixture) and the two
`G704` SSRF-taint hits in the routers are suppressed with reasons — forwarding to a
client-chosen destination is the product, gated by
`allow_list`/`block_list`/`allow_from`.

**M4 — Unpinned build-time dependencies.**
`Makefile:49` and `.github/workflows/release.yml` install `goreleaser@latest` at build
time; `internal/front/serve.go:15` runs `npm i` instead of `npm ci`. Both make builds
non-reproducible and expose CI to upstream breakage. *Fix:* pin goreleaser (version
file or `go tool`), switch to `npm ci`, and commit `package-lock.json` usage (the lock
file is already tracked).
**Status: fixed.** `GORELEASER_VERSION ?= v2.18.2` in the Makefile and an explicit
`@v2.18.2` in `.github/workflows/release.yml`; `//go:generate npm ci` replaces
`npm i`.

**M5 — Leak tests assert global goroutine counts with sleeps.**
`router/resource_test.go:100` polls `runtime.NumGoroutine()` against a captured `base`
with a ±5/10 tolerance after `time.Sleep(300ms)` calls. On a loaded CI runner unrelated
goroutines (netpoll, test framework) can push the count past the tolerance and turn the
test flaky. They passed here (`go test -race`), but they will be the first tests to
misbehave under `-count=10`. *Fix:* assert on *specific* tracked resources (owned
connections/listeners) rather than the process-global count, or keep the count check as
a `t.Log` diagnostic.
**Status: fixed.** The leak tests assert the resource: the HTTP test requires the
origin to accept ≤3 connections for 30 requests (pooling), and the TCP test
requires every target-side connection to close once the peers hang up. Process
goroutine counts are only logged. Verified with `-race -count=3`.

**M6 — Safe-by-default is inverted for a public deployment.**
`allow_from` is optional and the default is allow-all (`config/config.go:73-91`), while
the default entry timeout is 24 hours (`config/config.go:61`, `constants.Day`). An
`http-header`/`tcp-raw` listener published on `0.0.0.0` without `allow_from` is an open
proxy, and a tunnel to a silent peer can stay open for a day (keepalive now mitigates
this). *Fix:* log a loud warning at startup when a listener binds a non-loopback address
with no `allow_from`, and consider a shorter default idle timeout.
**Status: fixed.** `server.warnOpenListener` logs a loud warning when an entry
point binds a non-loopback address without `allow_from` (unit tested for public,
loopback and restricted listeners). The 24h default timeout is left as is —
keepalive and the idle sweeps now bound silent tunnels.

### Low

**L1 — `init()`-registered global state** (`router/router.go:17`,
`router/http_to_https.go:30`). Convenient, but it makes ordering implicit and forces
`router.Reset()` bookkeeping. A small registry object passed to `server.Serve` would
be easier to test.
**Status: fixed.** `router.NewRegistry()` returns a `Registry` holding every
built-in router and its reset hooks; the package-level `init()` registration and
the `Handle`/`Reset` globals are gone. `server.Serve` builds one registry per
run.

**L2 — Panics in package init / CLI wiring.** `cmd/run.go:92` panics if
`MarkFlagRequired` fails; cobra's flag registration is repeated boilerplate that could
use `BindPFlag`.
**Status: fixed.** `requireOrPanic` is deleted; `MarkFlagRequired` failures are
collected into `errFlagWiring` during `init` and returned from `RunE`, so a wiring
error becomes a normal command failure instead of a panic.

**L3 — Editor/agent artifacts are tracked:** `.vscode/launch.json`,
`internal/front/.bolt/config.json`, `internal/front/.bolt/prompt`. Harmless, but they
belong in `.gitignore`.
**Status: fixed.** The three files are untracked (`git rm --cached`, staged for the
next commit) and now ignored (`.vscode/launch.json`, `.bolt/`); the README debug
section no longer claims the launch config ships with the repo.

**L4 — README is slightly stale.** The directory tree (`README.md:396`) lists a
`docker/` directory that does not exist (the repo has `Dockerfile`/`docker-compose.yml`
at the root), and `todo.md` still lists hot reload as unchecked although
`cmd/root.go:68` wires `reloader.WithOsSignal` with a 1-minute debounce.
**Status: fixed.** The directory tree lists the real top level (no `docker/`;
`Dockerfile` and `docker-compose.yml` live at the root) and `todo.md` marks hot
reload done with its debounce note.

**L5 — `make spell` only reaches top-level markdown.** `Makefile:54` expands `**.md`
without `globstar`, so nested files such as `crypto/tls/BENCHMARK.md` are skipped, and
`-w` rewrites files in place (CI then relies on the `diff` step to fail).
**Status: fixed.** `make spell` expands `git ls-files --cached --others
--exclude-standard "*.md"`, which reaches nested files such as
`crypto/tls/BENCHMARK.md` while skipping `node_modules`; `-w` is kept so CI's
`diff` step still fails on a typo it had to rewrite.

**L6 — Lint findings remain** on `main` at `4730faf`: `gocyclo` 27 for
`(*httpToHTTPSProxy).ServeHTTP` (`router/http_to_https.go:171`, limit is 15),
`perfsprint` at `:332`, `staticcheck ST1023` at `:242`, `goconst` in
`router/http_header_test.go`. The `perfsprint`/`ST1023` pair is already being fixed in
the working tree (2 findings left), but the `gocyclo` 27 requires a real refactor.
Since CI runs `lint --fix` + `diff`, any *fixable* finding fails the build after
silently rewriting files.
**Status: fixed.** `ServeHTTP` is split into `resolveRoute`, `forward`,
`decodeBody`, `replaceTextBody` and `copyResponseHeaders` (complexity back under
the limit of 15) and `goconst` in `router/http_header_test.go` now uses an
`exampleHost` constant. `go tool golangci-lint run ./...` reports **0 issues**
(exit 0), so `make ci` can pass its lint + diff gate.

**L7 — DNS failure mode is `l.Panic`.** `server/server.go:63` panics the whole process
if FakeDNS keeps crashing after capped exponential backoff. Defensible for a
network-critical component, but it should be documented (and arguably a clean
`os.Exit(1)` with an error message rather than a stack trace).
**Status: fixed.** `runDNS` returns the error, `server.Serve` propagates it, and
the process exits through cobra with a message instead of `l.Panic`.

**L8 — `front.Serve` logs every connection state change at `Info`**
(`internal/front/serve.go:42`) and registers on `http.DefaultServeMux`
(`:36`) — a second `Serve` call in the same process would panic.
**Status: fixed.** Covered by the M2/L8 work above: private mux, `Debug` level
connection logging, `ctx`-driven shutdown.

---

## 5. Testing & CI

| Check | Result |
|---|---|
| `go build ./...` | pass (after frontend build) |
| `go vet ./...` | pass |
| `go test ./... -race -count=1` | pass |
| `go test ./... -cover` | **41.0%** total (was 22.9%); `config` 44.2%, `dns` 47.6%, `server` 45.9%, `connection` 86.5% (all were 0%) |
| `go test -race -count=3` (router, connection, dns, server, config) | pass — no timing flake in the rewritten leak tests |
| `golangci-lint run ./...` (2.13.2, pinned **and** PATH) | **0 issues**, exit 0 (was: 4 findings, then a Go 1.27 crash) |
| `golangci-lint fmt --diff` | clean |
| `make spell` | pass — reaches nested markdown via `git ls-files` |
| fresh-tree `go build ./...` / `go vet ./...` | **pass** from tracked files only (H1) |
| `npm run build` | pass, `dist/.keep` survives (`emptyOutDir: false`) |

Toolchain note: `go.mod` is `go 1.27.1`, workflows are `go-version: "1.27"`, and the
pinned golangci-lint is now `v2.13.2` (staticcheck `v0.8.1`) — module, CI and tools all
agree, and a clean clone builds and lints without a frontend build step.

CI runs `make ci` = `mod gen install build spell lint test` + a dirty-tree check, which
is a good "everything must be reproducible" gate; note it needs network (npm, goreleaser)
and Node 20. Test suite covers the new leak regressions plus SNI parsing benchmarks;
there is no integration test that starts a real chain (SOCKS5/SSH hop) end-to-end — the
`proxy` tests stop at the dialer.

---

## 6. Security

- **Access control** exists per entrypoint (`allow_list`/`block_list` for hostnames,
  `allow_from`/`block_from` for clients) and is enforced in every router — but default
  is allow-all (M6).
- **No TLS termination** for SNI passthrough (by design), so no certificate handling
  bugs; `crypto/tls` parses only the ClientHello and is fuzz-adjacent but tested.
- **Tooling:** CodeQL for Go + govulncheck in `make precommit`; `gosec` is now on
  and clean (M3 fixed), `mnd` stays off.
- **Supply chain:** release tarballs ship checksums and the installer verifies them;
  CI actions are pinned by tag (some with commit comments); goreleaser is pinned to
  `v2.18.2` and the frontend build uses `npm ci` (M4 fixed).
- **Config generator UI** defaults to `127.0.0.1:8080` and only produces text client
  side — low risk; the flag help explicitly suggests `0.0.0.0` for publishing, which is
  worth a warning since there is no authentication.

---

## 7. Performance

- Zero-alloc SNI parser with published benchmarks; relays use `utils.Copy` in both
  directions under `errgroup`, now with context enforcement, keepalive and dial
  deadlines (the `WSAENOBUFS`/700 MB growth bug is fixed by the recent series).
- Shared `http.Transport` with pooled connections for `http-header`; `http-to-https`
  closes idle connections per request (correct, but it forgoes pooling — acceptable
  given its per-request rewriting).
- No metrics/exported instrumentation yet (`todo.md` lists it); for a long-running
  proxy, connection counts and relay bytes would make the next leak much easier to
  spot than "memory grows until it dies".

---

## 8. Prioritized roadmap

1. ~~**H1** make `go build ./...` work from a clean clone~~ — **done** (`dist/.keep` + `all:dist` + `emptyOutDir: false`).
2. ~~**H2 (CI-breaking)** bump the pinned golangci-lint~~ — **done** (`v2.13.2`, staticcheck `v0.8.1`).
3. ~~**M1** tests for `config` matchers + `dns` answers; **M5** resource-specific leak tests~~ — **done** (coverage 22.9% → 41.0%).
4. ~~**M6** warn on non-loopback listeners without `allow_from`~~ — **done** (24h default kept).
5. ~~**M4/M3** pin goreleaser, `npm ci`, re-enable `gosec`~~ — **done** (v2.18.2, `gosec` clean).
6. ~~**M2** wire `ctx` into `front.Serve`, consolidate per-client UDP timers~~ — **done**.
7. ~~**L6** refactor `ServeHTTP`, clear `goconst`~~ — **done**: lint is 0 issues, `make ci` can pass.
8. ~~Docs hygiene: README tree, `todo.md` hot reload, `make spell` scope, editor artifacts~~ — **done** (L3/L4/L5).
9. Remaining low-priority follow-ups: shorter default idle timeout than `constants.Day`,
   per-entrypoint connection metrics (see `todo.md`), and an end-to-end test that
   starts a real SOCKS5/SSH chain.

---

## 9. Reproduction

```bash
git clone <repo> && cd junction
go build ./...                     # works: embedded dist/.keep placeholder (H1)
go tool golangci-lint run ./...    # 0 issues (H2 + L6 + M3)
go tool golangci-lint fmt --diff   # clean
go test -race -count=1 ./...       # pass (under Go 1.27.1)
go test -race -count=3 ./router/ ./connection/ ./dns/ ./server/ ./config/
go tool cover -func=<coverprofile> # 41.0%
git checkout-index -a --prefix=/tmp/fresh/   # tracked-files-only build (H1)
make spell                         # nested markdown, skips node_modules (L5)
npm run build                      # vite keeps dist/.keep (emptyOutDir: false)
```
