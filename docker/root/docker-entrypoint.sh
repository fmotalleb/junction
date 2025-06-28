#!/bin/bash
set -e

input="$VLESS_PROXY"
if [ -n "$input" ]; then
  echo "PLEASE SET A VLESS_PROXY ENV VARIABLE"
  junction parse-url "$VLESS_PROXY" >/etc/junction/singbox.d/10-singbox-outbound-proxy.toml
fi

exec "$@"
