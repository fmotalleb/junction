#!/bin/bash
set -e

if [ -n "$VLESS_PROXY" ]; then
  # ensure destination directory exists
  mkdir -p /etc/junction/singbox.d
  junction parse-url "$VLESS_PROXY" >/etc/junction/singbox.d/10-singbox-outbound-proxy.toml
else
  echo "VLESS_PROXY environment variable is unset or empty; skipping Singbox outbound-proxy generation"
fi

exec "$@"
