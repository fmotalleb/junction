#!/bin/bash

set -e

mkdir /etc/ssl/private -p
mkdir /etc/ssl/certs -p

CA_KEY="${CA_KEY:-/etc/ssl/private/ca-key.pem}"
CA_CERT="${CA_CERT:-/etc/ssl/certs/ca-cert.pem}"

cat <<EOF >/usr/lib/ssl/openssl.cnf
[ req ]
default_md = sha256
distinguished_name = req_distinguished_name

[ req_distinguished_name ]
countryName                     = US
stateOrProvinceName             = California
localityName                    = San Francisco
organizationName                = InnerOrg
commonName                      = inner.local
EOF

DAYS_VALID=3650
SUBJECT="/C=US/ST=California/L=San Francisco/O=InnerOrg/CN=inner.local"

if [ ! -f "$CA_KEY" ]; then
  openssl genrsa -out "${CA_KEY}" 2048
fi

if [ ! -f "$CA_CERT" ]; then
  openssl req -x509 -new -nodes \
    -key "${CA_KEY}" \
    -sha256 \
    -days "${DAYS_VALID}" \
    -subj "${SUBJECT}" \
    -out "${CA_CERT}"
fi
