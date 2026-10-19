#!/bin/sh
# Run inside the incus-lab VM (limactl shell incus-lab -- sh < this file), after lab-tls-setup.sh. Records whole
# envelopes over TLS (not the unix socket, which answers as root) so the stub's key sets come from data, not memory.
set -eu
cd /tmp
C="--cert lab-client.crt --key lab-client.key"
for p in /1.0 /1.0/cluster /1.0/images "/1.0/images?recursion=1"; do
  n=$(echo "$p" | tr '/?=' '___')
  echo "== trusted GET $p"; curl -sk $C -w '\nHTTP %{http_code}\n' "https://127.0.0.1:8443$p"
  echo "== untrusted GET $p"; curl -sk -w '\nHTTP %{http_code}\n' "https://127.0.0.1:8443$p"
done
echo "== server certificate (text)"; openssl x509 -in /var/lib/incus/server.crt -noout -subject -issuer -ext subjectAltName,keyUsage,extendedKeyUsage,basicConstraints 2>/dev/null || sudo openssl x509 -in /var/lib/incus/server.crt -noout -subject -issuer -ext subjectAltName,keyUsage,extendedKeyUsage,basicConstraints
echo "== server certificate key"; sudo openssl x509 -in /var/lib/incus/server.crt -noout -text | grep -E 'Public Key Algorithm|NIST CURVE|Signature Algorithm' | sort -u
echo "== client certificate fingerprint (sha256 of DER) — compare auth_user_name"; openssl x509 -in /tmp/lab-client.crt -outform DER | sha256sum
