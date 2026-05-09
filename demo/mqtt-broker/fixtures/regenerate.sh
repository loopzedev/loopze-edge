#!/usr/bin/env bash
# Regenerate the bundled Mosquitto demo CA, server cert, and client cert.
#
# All output is checked into the repo so the demo "just works" after a
# fresh clone. Validity is 10 years — re-run this when the certs are
# about to expire.
#
# Output formats:
#   - certs are PEM (`-----BEGIN CERTIFICATE-----`)
#   - keys are PKCS#1 (`-----BEGIN RSA PRIVATE KEY-----`) so LOOPZE's
#     gopcua-style loaders accept them everywhere we use this material
#     for cert-based auth tests.
#
# DO NOT use this material against any production broker — the private
# keys are public.

set -euo pipefail

cd "$(dirname "$0")"

# ── 1. Self-signed CA ───────────────────────────────────────────────────
echo "▸ Generating CA"
openssl req \
  -x509 \
  -newkey rsa:2048 \
  -nodes \
  -days 3650 \
  -keyout ca/ca.key.pkcs8 \
  -out    ca/ca.pem \
  -config ca/openssl.cnf
openssl rsa -in ca/ca.key.pkcs8 -traditional -out ca/ca.key
rm -f ca/ca.key.pkcs8

# ── 2. Server cert + key (signed by CA) ─────────────────────────────────
echo "▸ Generating server cert"
openssl req \
  -newkey rsa:2048 \
  -nodes \
  -keyout server/server.key.pkcs8 \
  -out    server/server.csr \
  -config server/openssl.cnf
openssl rsa -in server/server.key.pkcs8 -traditional -out server/server.key
rm -f server/server.key.pkcs8

openssl x509 -req \
  -in server/server.csr \
  -CA ca/ca.pem -CAkey ca/ca.key -CAcreateserial \
  -out server/server.pem \
  -days 3650 \
  -extensions v3_req -extfile server/openssl.cnf
rm -f server/server.csr ca/ca.srl

# ── 3. Client cert + key (signed by CA) ─────────────────────────────────
echo "▸ Generating client cert"
openssl req \
  -newkey rsa:2048 \
  -nodes \
  -keyout client/client.key.pkcs8 \
  -out    client/client.csr \
  -config client/openssl.cnf
openssl rsa -in client/client.key.pkcs8 -traditional -out client/client.key
rm -f client/client.key.pkcs8

openssl x509 -req \
  -in client/client.csr \
  -CA ca/ca.pem -CAkey ca/ca.key -CAcreateserial \
  -out client/client.pem \
  -days 3650 \
  -extensions v3_req -extfile client/openssl.cnf
rm -f client/client.csr ca/ca.srl

chmod 0644 ca/ca.pem ca/ca.key
chmod 0644 server/server.pem server/server.key
chmod 0644 client/client.pem client/client.key

echo
echo "Fingerprints:"
for f in ca/ca.pem server/server.pem client/client.pem; do
  printf "  %-22s " "$f"
  openssl x509 -in "$f" -noout -fingerprint -sha256 | cut -d= -f2
done
