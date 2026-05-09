#!/usr/bin/env bash
# Regenerate the bundled OPC UA demo client certificate.
#
# Run this whenever the cert is about to expire (validity is 10 years
# from generation). The output is checked into the repo so every clone
# works out of the box.
#
# Output format notes:
#   - Cert is a standard X.509 PEM (`-----BEGIN CERTIFICATE-----`).
#   - Key is written in PKCS#1 (`-----BEGIN RSA PRIVATE KEY-----`).
#     gopcua's PrivateKeyFile loader specifically rejects PKCS#8
#     (`-----BEGIN PRIVATE KEY-----`), so we must use the traditional
#     format. node-opcua accepts both.
#
# DO NOT use this material against any production server — the private
# key is public.

set -euo pipefail

cd "$(dirname "$0")/client"

# Generate cert + key with `req` (writes the key as PKCS#8), then
# convert the key in-place to PKCS#1 so gopcua's loader accepts it.
openssl req \
  -x509 \
  -newkey rsa:2048 \
  -nodes \
  -days 3650 \
  -keyout client.key.pkcs8 \
  -out client.pem \
  -config openssl.cnf

openssl rsa -in client.key.pkcs8 -traditional -out client.key
rm -f client.key.pkcs8

chmod 0644 client.pem client.key

echo
echo "Certificate fingerprint:"
openssl x509 -in client.pem -noout -fingerprint -sha256
echo
echo "Key format:"
head -1 client.key
