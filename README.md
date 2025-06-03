# Junction

**Junction** is a lightweight proxy server designed for efficient HTTP and HTTPS traffic routing. It supports SOCKS5 proxies and dynamically generates SSL certificates to ensure secure communication. Junction is particularly useful in environments where using SOCKS proxies is almost impossible, providing an elegant solution for complex proxy configurations.

---

## 🌟 **Features**

- 🔐 **Sni Passthrough**  
   No certificate required, reroutes tls packets using sni header.  

- 🧦 **SOCKS5 Proxy Support**  
   Routes traffic using SOCKS5 proxies, with built-in support for VLESS proxies via Docker image.  

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

### Config File

Junction supports configuration files in **TOML** or **YAML** formats for flexible entrypoint management.

#### **Example: TOML Configuration**

```toml
[[entrypoints]]
port = 8443 # Listen port
to = 443  # Reroutes connections to this port (defaults to 443)
routing = "sni" # Routing method
proxy = "127.0.0.1:7890" # socks5 proxy address

[[entrypoints]]
port = 8080
to = 80 # Defaults to 80
routing = "http-header" 
proxy = "127.0.0.1:7890"

[[entrypoints]]
port = 8090
to = 80 # Defaults to 80
routing = "http-header" 
proxy = "direct" # Do not handle using proxy just reverse proxy it directly
```

#### **Example: YAML Configuration**

```yaml
entrypoints:
- routing: "sni" # Routing method
  port: 8443 # Listen port
  to: 443 # Reroutes connections to this port (defaults to 443)
  proxy: 127.0.0.1:7890  # socks5 proxy address

- routing: http-header
  port: 8080
  to: 80 # Defaults to 80 
  proxy: 127.0.0.1:7890

```

Place the configuration file in the root directory or specify its path using the `--config` flag.

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
