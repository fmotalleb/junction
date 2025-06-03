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

function writeConf() {
  echo "$1
" >>/etc/junction/config.toml
}

writeConf ""

exec "$@"
