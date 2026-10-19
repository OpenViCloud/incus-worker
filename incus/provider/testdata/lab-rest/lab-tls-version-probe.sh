#!/bin/sh
# Run inside the incus-lab VM after lab-tls-setup.sh: which TLS versions the Incus API accepts.
for v in tls1_2 tls1_3; do
  echo "== -$v"; echo | openssl s_client -connect 127.0.0.1:8443 -$v 2>&1 | grep -E 'Protocol|alert|Cipher is|no protocols' | head -3
done
