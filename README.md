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

### Prerequisites

Before setting up Junction, make sure you have the following installed:

- [Docker](https://www.docker.com/)  
- [Docker Compose](https://docs.docker.com/compose/)  
- [Go](https://golang.org/) (for local development)  

---

### ❗️ **Docker Image Details (Must Read)**

The Docker image `ghcr.io/fmotalleb/junction` differs from typical images by including **Supervisord** and **Sing-box**, enabling seamless connection to VLESS proxies. Inside the container, VLESS is exposed as a mixed SOCKS/HTTP proxy that Junction uses for routing traffic effectively.

---

### Installation

Steps to install Junction:

1. Clone the repository:

   ```bash
   git clone https://github.com/FMotalleb/junction.git
   cd junction
   ```

2. Configure environment variables using `.env.example`.  

3. Build and run the Docker container:

   ```bash
   docker-compose up --build
   ```

4. Access the proxy server on ports:
   - **80** (HTTP)  
   - **443** (HTTPS)  

---

## 🛠 **Configuration**

### Configuration

Remember that the cli has an `example` sub command that will be updated more than this section,

#### Fields

At the top level, define an array named `entrypoints`. Each entry describes a routing configuration and includes the following fields:

- **`port`**:
  Local port to listen on.

- **`routing`**:
  Defines how the target hostname is resolved. Supported modes:

  - `sni`: Uses SNI for hostname detection. Requires target port. Default: `443`.
  - `http-header`: Uses HTTP `Host` header. Requires target port. Default: from `Host`.
  - `tcp-raw`: Raw TCP forwarding. Requires complete `ip:port`. No defaults.

- **`proxy`**:
  Defines one or more upstream proxies. Supports:

  - A comma-separated string
  - An array of proxy URIs

  Supported proxy types:

  - **SOCKS (RFC-compliant)**:

    ```
    socks5://(user:pass)@hostname:port
    ```

    - Username and password are optional.
  - **SSH (custom URI format)**:

    ```
    ssh://user(:pass)@hostname:port(/path/to/private/key)
    ```

    - Password and private key path are optional.
    - Use either password or key-based authentication.

  Default: `direct` (no proxy)

  e.g: These two are identical
  - `"socks5://user:pass@10.0.0.1:1080,socks5://10.0.0.2:1080,ssh://user@10.0.0.3:22/tmp/key"`
  - `["socks5://user:pass@10.0.0.1:1080", "socks5://10.0.0.2:1080", "ssh://user@10.0.0.3:22/tmp/key"]`

  ```mermaid
  graph LR
     Client --> Proxy1["socks5://user:pass\@10.0.0.1:1080"]
     Proxy1 --> Proxy2["socks5://10.0.0.2:1080"]
     Proxy2 --> Proxy3["ssh://user\@10.0.0.3:22"]
     Proxy3 --> Target["example.com:80"]
  ```

- **`to`**:
  Destination address or port, depending on the selected `routing` mode.

- **`timeout`**:
  Maximum allowed duration for a connection.

  - Default: `24h`
  - Default is overridable via `TIMEOUT` environment variable
  - Format: Go duration syntax (e.g., `5h3m15s`)

**Warnings**:

- The `proxy` list is interpreted in order; misordering may break the chain.
- Only one authentication method should be used per SSH proxy entry.
- `tcp-raw` requires explicit `ip:port`; no inference is made.

#### **Example: TOML Configuration**

```toml
[[entrypoints]]
port = 8443 # Listen port
to = "443"  # Reroutes connections to this port (defaults to 443)
routing = "sni" # Routing method
proxy = "socks5://127.0.0.1:7890" # socks5 proxy address

[[entrypoints]]
port = 8080
routing = "http-header" 
to = "80" # Defaults from `Host`
proxy = "socks5://127.0.0.1:7890"

[[entrypoints]]
port = 8090
routing = "http-header" 
to = "80"
proxy = "direct" # Do not handle using proxy just reverse proxy it directly

[[entrypoints]]
port = 8099
to = "18.19.20.21:22" # Required for tcp-raw
routing = "tcp-raw"  # TCP raw is old behavior where the target address must be specified (used for non-tls non-http requests that do not have any indications for server name nor address)
proxy = "direct" # Do not handle using proxy just reverse proxy it directly
```

#### **Example: YAML Configuration**

```yaml
entrypoints:
- routing: "sni" # Routing method
  port: 8443 # Listen port
  to: "443" # Reroutes connections to this port (defaults to 443)
  proxy: socks5://127.0.0.1:7890  # socks5 proxy address

- routing: http-header
  port: 8080
  to: "80" # Defaults to 80 
  proxy: socks5://127.0.0.1:7890

```

Place the configuration file in the process working directory (file name 'junction.toml') or specify its path using the `--config` flag.

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
