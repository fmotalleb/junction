# Junction

**Junction** is a lightweight reverse proxy optimized for efficient TCP and TLS traffic routing. It inspects protocol-level metadata (such as SNI in TLS) to forward encrypted connections to the appropriate backend, without decrypting the traffic. Junction supports both SOCKS5 and SSH proxy protocols (and chaining them), making it ideal for complex egress scenarios where transparent, performant routing is required.

---

## 🌟 **Features**

- 🔐 **Sni Passthrough**  
   No certificate required, reroutes tls packets using sni header.  

- 🧦 **SOCKS5 Proxy Support**  
   Routes traffic using SOCKS5 proxies, with built-in support for VLESS proxies via Docker image.  

- 🔀 **SSH Proxy Support**  
   Routes traffic using SSH connection as proxy.

- 🔗 **Proxy Chain Support**  
   Chain multiple proxies together to create complex routing paths and improve privacy or bypass restrictions.

- 🐳 **Dockerized Deployment**  
   Includes a ready-to-use Docker setup for seamless deployment in any environment.  

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

You can grab one of builds from [Release](https://github.com/FMotalleb/junction/releases) page
or use the shell script (please review [the scripts](https://raw.githubusercontent.com/FMotalleb/junction/refs/heads/main/install.sh) before executing it in your shell, or any script you find online who paste them in the shell without checking)
This script requires `curl`, `tar`, `jq` (if version is missing), `sha256sum` (optional but recommended) and `bash` itself.

Install latest version (requires `jq`):

```bash
bash <<<"$(curl -fL https://raw.githubusercontent.com/FMotalleb/junction/refs/heads/main/install.sh)"
```

```sh
curl -fL https://raw.githubusercontent.com/FMotalleb/junction/refs/heads/main/install.sh | bash
```

or select a version manually:

```bash
VERSION=0.4.2 bash <<<"$(curl -fL https://raw.githubusercontent.com/FMotalleb/junction/refs/heads/main/install.sh)"
```

#### Using Go cli

Simply using

```bash
go install github.com/FMotalleb/junction@latest
```

in this method version variables are missing thus you cannot use `--version (-v)` to acquire version number

#### Docker based

Using:

- [Docker](https://www.docker.com/)  
- [Docker Compose](https://docs.docker.com/compose/)

---

### ❗️ **Docker Image Details (Must Read)**

#### Vless support

every build of this application contains singbox internally, but only start singbox if `core.singbox` is non-empty value.

The Docker image `ghcr.io/fmotalleb/junction:latest-vless` contains a simple bash script entrypoint and basic configuration for sing-box service.
This script is able to parse `VLESS_PROXY` to outbound config or receive each field of VLESS proxy as env parameters (see .env.example).

- Remember: this image requires those env vars to be set.

- `latest-vless`
- `{{ .Version }}-vless`

#### Basic image

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
  You can include multiple config files (even from a remote http source):
  Please note that this list is not loosely typed so you have to declare an array of strings

  - Support Glob pattern matching
  - Support `HTTP` and `HTTPS` with basic authentication

  ```toml
  include = [
    "./*.toml",
    "http://kamand-pwa.dornicademo.ir/config.toml",
  ]
  ```

- **Core**
  Some specific global configurations are stored here
  - singbox: object of singbox config
    [singbox](https://github.com/SagerNet/sing-box/) is a successor to xray
    Its config is complex you can see an example of how to provide a simple config in [example](https://github.com/FMotalleb/junction/blob/main/example) directory

- **Entrypoints**:
  Top-level array defining routing configurations. Each entry includes:

  - **`listen`** (required):
    Bind address for incoming connections. Accepts:
    - Full address: `"IP:port"` (e.g., `"0.0.0.0:8443"`)
    - Port only: `":port"` (binds to `127.0.0.1:port`)
    - Integer: `port` (binds to `127.0.0.1:port`)

  - **`routing`** (required):
    Target hostname resolution method:
    - `sni`: Uses SNI for hostname detection. Default port: `443`
    - `http-header`: Uses HTTP `Host` header. Default port: `80`
    - `tcp-raw`: Raw TCP forwarding. Requires complete `ip:port` in `to` field
    - `udp-raw`: Raw UDP forwarding. Requires complete `ip:port` in `to` field. **Note**: Proxy not supported

  - **`proxy`** (optional):
    Upstream proxy configuration. Accepts:
    - String: Comma-separated proxy chain
    - Array: Ordered list of proxy URIs

    Supported proxy protocols:
    - **SOCKS5**: `socks5://[user:pass@]hostname:port`
    - **SSH**: `ssh://user[:pass]@hostname:port[/path/to/private/key]`
      - Use either password OR key authentication, not both

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

  - **`block_list`** (optional) [only when using sni,http-header]:
    List of hostnames/patterns to block. Supports wildcards (e.g., `"*.example.com"`)

  - **`allow_list`** (optional) [only when using sni,http-header]:
    List of hostnames/patterns to allow. If specified, only listed hosts are allowed.
    - Supports wildcards (e.g., `"*.example.com"`)
    - Block rules are applied before allow rules

**Important Notes**:

- Proxy chains execute in order; incorrect ordering breaks the chain
- `tcp-raw` and `udp-raw` require explicit `ip:port` targets
- `udp-raw` routing doesn't support proxy protocols
- When using `allow_list`, unlisted hosts are implicitly blocked
- Wildcard patterns (e.g., `*.example.com`) match subdomains only, not the base domain

#### **Example: TOML Configuration**

```toml
[[entrypoints]]
listen = "0.0.0.0:8443" # Listen IP:Port address
to = "443"  # Reroutes connections to this port (defaults to 443)
routing = "sni" # Routing method
proxy = "socks5://127.0.0.1:7890" # socks5 proxy address

[[entrypoints]]
listen = ":8080" # Listen on 127.0.0.1:8080
routing = "http-header" 
to = "80" # Defaults from `Host`
proxy = "socks5://127.0.0.1:7890"

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
  proxy: socks5://127.0.0.1:7890  # socks5 proxy address

- routing: http-header
  listen: 8080
  to: "80" # Defaults to 80 
  proxy: socks5://127.0.0.1:7890

```

> You can specify config file path using `--config (-c)` flag (detects config file)
> Default behavior is to read config from `stdin` using `toml` format

---

## 💡 **Docker Environment Variables**

Use environment variables for dynamic runtime configuration. Below is an example `.env` file:

```env
VLESS_PROXY=
HTTP_PORT=80
SNI_PORT=443
```

These variables help configure VLESS proxies and expose specific endpoints for HTTP/HTTPS traffic.

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

A pre-configured `.vscode/launch.json` is included for debugging purposes. To debug:  

1. Open the project in Visual Studio Code.  
2. Use the **"Launch Package"** configuration to start debugging.  

---

### Directory Structure

Junction's project structure is organized as follows:

```
.
├── cmd/ # CLI entry point
├── config/ # Configuration parsing and helpers
├── docker/ # Docker-related files
├── router/ # Routers (sni,http,...) logic
├── server/ # Core server logic
├── main.go # Main entry point
└── docker-compose.yml # Docker Compose configuration
```

---

## 📜 **License**

This project is licensed under the **GNU General Public License v2.0**. Refer to the `LICENSE` file for more details.  
