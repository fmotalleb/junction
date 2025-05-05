#!/bin/bash
set -e

input="$VLESS_PROXY"
if [ -z "$input" ]; then
  echo "PLEASE SET A VLESS_PROXY ENV VARIABLE"
  exit 1
fi

# Parse the input string
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

openssl req -x509 -nodes -days 365 \
  -newkey rsa:2048 \
  -keyout /etc/ssl/private/haproxy.key \
  -out /etc/ssl/certs/haproxy.crt \
  -subj "/CN=*"

mkdir -p /usr/local/etc/ssl/private/
cat /etc/ssl/certs/haproxy.crt /etc/ssl/private/haproxy.key >/usr/local/etc/ssl/private/haproxy.pem
chown haproxy:haproxy /usr/local/etc/ssl/ -R
# exit 1
exec "$@"
