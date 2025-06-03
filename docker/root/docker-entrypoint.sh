#!/bin/bash
set -e

input="$VLESS_PROXY"
if [ -z "$input" ]; then
  echo "PLEASE SET A VLESS_PROXY ENV VARIABLE"
  exit 1
fi

# Proxy config
sanitized="$(sed <<<"$input" -e 's|.*://||')"

IFS="?" read <<<"$sanitized" authority query

IFS='@:' read uuid host port <<<"$authority"

export UUID="$uuid"
export SERVER_ADDRESS="$host"
export PORT="$port"

params=$(sed <<<"$query" -E 's/([^#]+)(#.*)?/\1/' | tr '&' '\n')
eval "$params"
export HOSTNAME="${host:-$SERVER_ADDRESS}"
export SNI="${sni:-$HOSTNAME}"

tls_on="$([ "${security:-''}" == 'tls' ] && echo -n 'true' || echo -n 'false')"
export TLS_ENABLED="$tls_on"
export TRANSPORT="${type:-"ws"}"
export TRANSPORT_PATH="${path:-'/'}"
export ENCRYPTION="${encryption:-'none'}"
packetEncoding=$(echo -n "${packetEncoding:-'xudp'}" | tr -d "'\"")
export PACKET_ENCODING="${packetEncoding:-'xudp'}"

envsubst </etc/singbox/config.json.template >/etc/singbox/config.json

# Server Config
mkdir /etc/junction || true

: "${HTTP_PORT:=80}"
: "${SNI_PORT:=443}"

if [ -f "/etc/junction/config.toml" ]; then
  echo "config file already exists, skipping config generation"
elif [ -n "$JUNCTION_CONF_B64" ]; then
  base64 -d <<<"$JUNCTION_CONF_B64" >/etc/junction/config.toml
else
  cat <<EOF >/etc/junction/config.toml
[[entrypoints]]
routing = "sni"
port = $SNI_PORT
to = 443
proxy = "127.0.0.1:6980"

[[entrypoints]]
routing = "http-header"
port = $HTTP_PORT
to = 80
proxy = "127.0.0.1:6980"
EOF
fi
exec "$@"
