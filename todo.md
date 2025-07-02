# TODO List

## Entrypoints

* [x] SNI (Server Name Indication) support
* [x] HTTP support
* [x] TCP (with predefined targets)
* [x] UDP support
* [ ] DNS (basic UDP-based server): (currently use singbox)
  * [ ] Fake DNS response generation
  * [ ] DNS request forwarding (requires request filtering to be effective)
  * [ ] DNS over HTTPS (DoH) spoofing
  * [ ] DoH request forwarding

## Proxy Support

* [x] SOCKS5
* [x] Proxy chaining
* [x] SSH tunneling
* [x] Add sing-box engine support to core application
  * [x] Integrate sing-box into codebase and config
  * [x] Generate sing-box config from proxy url
* [ ] Proxy load balancing

## Core Features

* [ ] Handler pipeline:
  * [ ] Support filtering to route a single entrypoint via different proxies or targets
  * [ ] Request transformation/mutation
* [ ] Metrics collection
* [ ] Access logging
* [ ] Monitoring support (for raw protocols)
* [ ] Hot reload configuration

## Performance Enhancements

* [ ] Proxy reuse via proxy pool
* [ ] Connection pooling (limit max concurrent connections per entrypoint)
* [ ] Connection warm-up (optional; can trigger bans from tools like fail2ban)
* [x] Zero alloc sni-parser

## Configuration

* [x] Support multiple configuration formats
* [ ] Read configuration from `stdin`
  * Dropped in favor of include multiple config files
* [x] Simplified CLI support (e.g., one-liner config)
* [x] Dump config
* [x] Loose type config parser
* [x] Include more than one config file
