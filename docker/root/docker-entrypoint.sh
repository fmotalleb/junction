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

mkdir /etc/junction
env | awk 'BEGIN {
    proxy_address = "127.0.0.1:6980";
}

/^EXPOSE_[0-9]+=/ {
    split($0, parts, "=");
    port = substr(parts[1], 8);
    url = parts[2];
    
    print "[[targets]]";
    print "port =", port;
    print "proxy = \"" proxy_address "\"";
    print "target = \"" url "\"";
    print "";
}' >/etc/junction/config.toml

exec "$@"
