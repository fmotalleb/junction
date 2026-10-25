# Junction

[![build](https://github.com/fmotalleb/junction/actions/workflows/build.yml/badge.svg)](https://github.com/fmotalleb/junction/actions/workflows/build.yml) [![release](https://github.com/fmotalleb/junction/actions/workflows/release.yml/badge.svg)](https://github.com/fmotalleb/junction/actions/workflows/release.yml) [![CodeQL](https://github.com/fmotalleb/junction/actions/workflows/codeql.yml/badge.svg)](https://github.com/fmotalleb/junction/actions/workflows/codeql.yml)

**Junction** is a lightweight reverse proxy optimized for efficient TCP and TLS traffic routing. It inspects protocol-level metadata (such as SNI in TLS) to forward encrypted connections to the appropriate backend, without decrypting the traffic. Junction supports both SOCKS5 and SSH proxy protocols (and chaining them), making it ideal for complex egress scenarios where transparent, performant routing is required.

---

## 🌟 **Features**

- 🔐 **Sni Passthrough**  
   No certificate required, reroutes tls packets using sni header.

- 🧦 **SOCKS5 Proxy Support**  
   Routes traffic using SOCKS5 proxies.

- 🔀 **SSH Proxy Support**  
   Routes traffic using SSH connection as proxy.

- 🔗 **Proxy Chain Support**  
   Chain multiple proxies together to create complex routing paths and improve privacy or bypass restrictions.

- 🐳 **Dockerized Deployment**  
   Includes a ready-to-use Docker setup for seamless deployment in any environment.

- 🌐 **Internal Fake DNS**  
   Fake DNS with ability to match only some domains, change result based on client's ip address,
   and forward unmatched requests to an optional forwarder DNS.

---

## 📋 **Table of Contents**

1. [Getting Started](#-getting-started)
2. [Configuration](#-configuration)
3. [Environment Variables](#-docker-environment-variables)
4. [Usage](#%EF%B8%8F-usage)
5. [Development](#-development)
6. [License](#-license)

---

## 🚀 **Getting Started**

### Installation

#### Standalone binary installation

You can grab one of builds from [Release](https://github.com/fmotalleb/junction/releases) page
or use the shell script (please review [the scripts](https://raw.githubusercontent.com/fmotalleb/junction/refs/heads/main/install.sh) before executing it in your shell, or any script you find online who paste them in the shell without checking)
This script requires `curl`, `tar`, `jq` (if version is missing), `sha256sum` (optional but recommended) and `bash` itself.

Install latest version (requires `jq`):

```bash
bash <<<"$(curl -fL https://raw.githubusercontent.com/fmotalleb/junction/refs/heads/main/install.sh)"
```

```sh
curl -fL https://raw.githubusercontent.com/fmotalleb/junction/refs/heads/main/install.sh | bash
```

or select a version manually:

```bash
VERSION=0.4.2 bash <<<"$(curl -fL https://raw.githubusercontent.com/fmotalleb/junction/refs/heads/main/install.sh)"
```

#### Using Go cli

Simply using

```bash
go install github.com/fmotalleb/junction@latest
```

in this method version variables are missing thus you cannot use `--version (-v)` to acquire version number

#### Docker based

Using:

- [Docker](https://www.docker.com/)
- [Docker Compose](https://docs.docker.com/compose/)

---

### ❗️ **Docker Image Details (Must Read)**

contains junction itself based on distroless images by [google](gcr.io/distroless/base-debian12:nonroot)

- `latest`
- `latest-distroless`
- `{{ .Version }}-distroless`

---

### Run Docker container

```bash
# Documented example of config file
docker run --rm -it ghcr.io/fmotalleb/junction:latest example

# Save config file to
docker run --rm ghcr.io/fmotalleb/junction:latest example > config.toml
docker run --rm -it \
   -v "./config.toml:/config.toml" \
   --network host \ # or map each port manually
   ghcr.io/fmotalleb/junction:latest -c /config.toml
```

---

## 🛠 **Configuration**

### Configuration

Remember that the cli has an `example` sub command that will be updated more than this section,
Config specifications defined here may not be complete
Most parameters are loosely typed
arrays can be defined as single items and it will be mapped to array internally
some objects are able to parse themselves from strings

#### Run SubCommand

Simplest way to run the server is using `run` sub command

```bash
junction run --help # show help for this sub command
# Simple example of run command that listens on port 8443
#   thru socks5 proxy on port 7890 of localhost
#   transfers the request to port 443
#   of the found hostname using `sni` packets
junction run --listen 8443 \
             --proxy socks5://127.0.0.1:7890 \
             --target 443 \
             --routing sni
```

#### Fields

- **Include**
  You can include multiple config files, including remote HTTPS sources.
  Include files are merged in listed order; the including file overrides included values.
  Every listed include is required: a missing file, unmatched glob, or parse error
  aborts loading. Optional include syntax is not supported. Parsing builds a fresh
  configuration snapshot, so removed fields do not carry over from a previous load.

  - Support Glob pattern matching
  - Remote sources require `HTTPS` and support basic authentication. For temporary
    migration only, `JUNCTION_ALLOW_INSECURE_CONFIG_HTTP=1` enables plain HTTP.

  ```toml
  include = [
    "./*.toml",
    "https://remote-server.com/config.toml",
  ]
  ```

- **Core**
  Some specific global configurations are stored here

  - fake_dns: object or string of fake DNS config:
    In order to create a simple dns server to manipulate requests into this server (needs manual IP configuration)
    By default the DNS listener binds to `127.0.0.1:53`. Set `listen` explicitly
    to expose it on another interface; public bindings require `allow_from` client
    rules and are limited to 256 concurrent DNS requests. Keep network firewall
    rules in place as well.
    Limitations:
    - A Record support (for now since i don't use AAAA records)
    - UDP listener; DNS-over-TCP clients are not supported
    - UDP upstream forwarding with automatic TCP retry when the upstream marks
      its UDP response truncated
    Config:
      - `listen`: UDP listen address, requires a UDP ip:port
      - `answer`: IPv4 Answer, requires a single IPv4 address
        alternatively this field is able to receive array(or just an object) as value
        - `answer`: Single IPv4 to answer
        - `from`: CIDR address of remote user that queries DNS
           Order is important on masking networks

        ```toml
        [[core.fake_dns.answer]]
        answer = "127.0.0.1"
        from = "127.0.0.1/24"
        
        [[core.fake_dns.answer]]
        answer = "192.168.1.1"
        from = [
          "192.168.2.0/24",
          "128.1.1.0/24"
        ]
        ```

      - `forwarder`: Upstream DNS server for unresolvable/not-allowed queries, (e.g. `8.8.8.8:53`), if omitted will return empty response
      - `allowed`: Allowed list matcher
        - Supports wildcards (e.g., `"*.example.com"`)
        - Supports Regular Expression (e.g. `"regex:allowed"`, `"grep:.+google.com^"`)
      - `allow_from`: Required for non-loopback listeners; matcher rules apply to
        client IP addresses (for example `"192.168.1.*"` or `"regex:^10\\."`).
  - singbox: object of singbox config
    [singbox](https://github.com/SagerNet/sing-box/) is a successor to xray
    Its config is complex you can see an example of how to provide a simple config in [example](https://github.com/fmotalleb/junction/blob/main/example) directory

- **Entrypoints**:
  Top-level array defining routing configurations. Each entry includes:

  - **`listen`** (required):
    Bind address for incoming connections. Accepts:

    - Full address: `"IP:port"` (e.g., `"0.0.0.0:8443"`)
    - Port only: `":port"` (binds to `127.0.0.1:port`)
    - Integer: `port` (binds to `127.0.0.1:port`)

    Unrestricted non-loopback `sni` and `http-header` listeners must set
    `public_access = true`. Add `allow_from` rules to restrict clients instead.
    Existing public configurations without either setting now fail startup and
    must be migrated deliberately.

  - **`routing`** (required):
    Target hostname resolution method:

    - `sni`: Uses SNI for hostname detection. Default port: `443`
    - `http-header`: Uses HTTP `Host` header. Default port: `80`
    - `tcp-raw`: Raw TCP forwarding. Requires complete `ip:port` in `to` field
    - `udp-raw`: Raw UDP forwarding. Requires complete `ip:port` in `to` field. Proxy chains are not supported. Sessions are capped at 4096 per listener and 64 per source IP. `UDP_BUFFER` controls the upstream response buffer and must be 1–65507 bytes (default 65507).
    - TCP, SNI, and HTTP listeners accept at most 1024 concurrent connections
      each, with a per-source cap of 64 connections.
  - **`tag`** (optional):
      The tag attribute groups multiple entrypoints so they share a single listening socket while applying different domain-matching rules. A tag represents a routing group evaluated on the same port.
      Rules are checked in configuration order; the first matching entrypoint is used.
      All entrypoints in a tag group must:
        - use the same listen address
        - use the same routing mode (`sni`, `http-header`, or `http-to-https`)
        - specify the same tag identifier
      Tags are not supported for `tcp-raw` or `udp-raw`.

  - **`proxy`** (optional):
    Upstream proxy configuration. Accepts:

    - String: Comma-separated proxy chain
    - Array: Ordered list of proxy URIs

    Supported proxy protocols:

    - **SOCKS5**: `socks5://[user:pass@]hostname:port`
    - **SSH**: `ssh://user[:pass]@hostname:port[/path/to/private/key]?known_hosts=/path/to/known_hosts`
      - Use either password OR key authentication, not both
      - Server keys must be present in `~/.ssh/known_hosts` or the configured `known_hosts` file
      - Host-key verification is mandatory; configurations that previously relied
        on accepting unknown SSH keys must add the verified server key before
        starting. Obtain the fingerprint through a trusted channel, verify the
        entry gathered with `ssh-keyscan`, then install it in the default or
        configured `known_hosts` file. Do not trust an unverified scan result.

    Default: `direct` (no proxy)

    Example proxy chains (equivalent):

    - `"socks5://user:pass@10.0.0.1:1080,socks5://10.0.0.2:1080,ssh://user@10.0.0.3:22/tmp/key"`
    - `["socks5://user:pass@10.0.0.1:1080", "socks5://10.0.0.2:1080", "ssh://user@10.0.0.3:22/tmp/key"]`

    ```mermaid
    graph LR
      Client --> Proxy1["socks5://user:pass@10.0.0.1:1080"]
      Proxy1 --> Proxy2["socks5://10.0.0.2:1080"]
      Proxy2 --> Proxy3["ssh://user@10.0.0.3:22"]
      Proxy3 --> Target["example.com:80"]
    ```

  - **`to`** (required):
    Target destination:

    - For `sni`/`http-header`: Port number (string)
    - For `tcp-raw`/`udp-raw`: Complete `"ip:port"` address

  - **`timeout`** (optional):
    Connection timeout duration:

    - Default: `24h` (or `TIMEOUT` environment variable)
    - Format: Go duration syntax (e.g., `"50s"`, `"5h3m15s"`)

  - **`features`** (optional):
    List of feature flags that enable routing-specific behavior.

    - `http-header`:
      - `flexible-port`: Allows per-request port override using the `Junction-Port` HTTP header. Requires `allowed_ports`; requests for other ports are rejected.
        If the header is missing or empty, the `to` value (or the router default) is used.
    - Other routers currently ignore feature flags (unknown features are ignored with a warning).

  - **`block_list`** (optional) [only when using sni,http-header]:
    List of hostnames/patterns to block.
    - Supports wildcards (e.g., `"*.example.com"`, `"glob:*.example.com"`)
    - Supports Regular Expression (contain check) (e.g. `"regex:badword"`, `"grep:bad.+word"`)
  - **`allow_list`** (optional) [only when using sni,http-header]:
    List of hostnames/patterns to allow. If specified, only listed hosts are allowed.
    - Supports wildcards (e.g., `"*.example.com"`)
    - Supports Regular Expression (e.g. `"regex:allowed"`, `"grep:.+google.com^"`)
    - Block rules are applied before allow rules
  - **`block_from`** (optional):
    List of client address patterns to block (applies to all routers).
    - Supports wildcards and regular expressions (same matcher rules as `block_list`)
    - Block rules are applied before allow rules
  - **`allow_from`** (optional):
    List of client address patterns to allow (applies to all routers). If specified, only listed clients are allowed.
    - Supports wildcards and regular expressions (same matcher rules as `allow_list`)
    - Block rules are applied before allow rules

  - **`destination_allow`** and **`destination_block`** (optional):
    CIDR policy applied to resolved destination addresses on direct routes.
    If `destination_allow` is set, only addresses in those ranges are dialed;
    `destination_block` always takes precedence. The proxy resolves the hostname
    and dials the checked address directly. These rules cannot be combined with
    a proxy chain because the chain may resolve destinations remotely; startup
    fails if both are configured.

**Important Notes**:

- Proxy chains execute in order; incorrect ordering breaks the chain
- `tcp-raw` and `udp-raw` require explicit `ip:port` targets
- `udp-raw` routing doesn't support proxy protocols
- When using `allow_list`, unlisted hosts are implicitly blocked
- When using `allow_from`, unlisted clients are implicitly blocked
- Wildcard patterns (e.g., `*.example.com`) match subdomains only, not the base domain

#### **Example: TOML Configuration**

```toml
[[entrypoints]]
listen = "0.0.0.0:8443" # Listen IP:Port address
public_access = true # Explicitly allow any client to use this SNI listener
to = "443"  # Reroutes connections to this port (defaults to 443)
routing = "sni" # Routing method
proxy = "socks5://127.0.0.1:7890" # socks5 proxy address

[[entrypoints]]
listen = ":8080" # Listen on 127.0.0.1:8080
routing = "http-header"
to = "80" # Defaults from `Host`
proxy = "socks5://127.0.0.1:7890"
features = ["flexible-port"] # Allow per-request port override via Junction-Port header
allowed_ports = ["80", "443"] # Required; Junction-Port must match one of these
allow_from = ["127.0.0.1", "10.0.*", "regex:^192\\.168\\.1\\."]

[[entrypoints]]
listen = 8090 # Listen on 127.0.0.1:8090
routing = "http-header"
to = "80"
proxy = "direct" # Do not handle using proxy just reverse proxy it directly

[[entrypoints]]
listen = 8099
to = "18.19.20.21:22" # Required for tcp-raw
routing = "tcp-raw"  # TCP raw is old behavior where the target address must be specified (used for non-tls non-http requests that do not have any indications for server name nor address)
proxy = "direct" # Do not handle using proxy just reverse proxy it directly
```

#### **Example: YAML Configuration**

```yaml
entrypoints:
  - routing: "sni" # Routing method
    listen: 8443 # Listen ip addr (default ip is 127.0.0.1 if omitted)
    to: "443" # Reroutes connections to this port (defaults to 443)
    proxy: socks5://127.0.0.1:7890 # socks5 proxy address

  - routing: http-header
    listen: 8080
    to: "80" # Defaults to 80
    proxy: socks5://127.0.0.1:7890
```

JSON Schema for the configuration file is available at `config.schema.json`.

> You can specify config file path using `--config (-c)` flag (detects config file)
> Default behavior is to read config from `stdin` using `toml` format
> `--dry-run` writes the full configuration to stdout, including passwords and
> secret fields. Treat it as an export, not a redacted diagnostic; if saving it,
> use a private file owned by the service user with mode `0600`.

---

## ▶️ **Usage**

### **Running Locally**

1. Build the Go application:

   ```bash
   go build -o junction
   ```

2. Run the application:

   ```bash
   ./junction --config=config.toml
   ```

---

### **Running with Docker**

To build and launch the Docker container:

```bash
docker-compose up --build
```

Once running, the application will be accessible on the configured ports.

---

## 🛠 **Development**

### Debugging with VS Code

Create a local `.vscode/launch.json` (it is git-ignored) with a **"Launch Package"**
configuration, or start under Delve directly:

```bash
dlv debug ./main.go -- --config=config.toml
```

---

### Directory Structure

Junction's project structure is organized as follows:

```
.
├── cmd/               # CLI entry point
├── config/            # Configuration parsing and helpers
├── connection/        # TCP/UDP connection and idle-cleanup helpers
├── crypto/            # ClientHello parser used for SNI routing
├── dns/               # FakeDNS server and request forwarding
├── example/           # Example configuration files
├── internal/          # Embedded configuration UI (Vite/React)
├── proxy/             # SOCKS5 and SSH dialers
├── router/            # Routers (sni, http-header, tcp-raw, udp, ...)
├── server/            # Server lifecycle and entrypoint wiring
├── utils/             # Shared helpers (copy, ...)
├── Dockerfile         # Container image build
├── docker-compose.yml # Docker Compose configuration
└── main.go            # Main entry point
```

---

## 📜 **License**

This project is licensed under the **GNU General Public License v2.0**. Refer to the `LICENSE` file for more details.
