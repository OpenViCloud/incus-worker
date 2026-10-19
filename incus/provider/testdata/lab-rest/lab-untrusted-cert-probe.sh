#!/bin/sh
# Run inside the incus-lab VM after lab-tls-setup.sh. A client that presents a certificate Incus does not trust, and
# the one route the earlier probes left out (cluster members without trust).
set -eu
cd /tmp && rm -f lab-stranger.crt lab-stranger.key
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:secp384r1 -nodes -days 1 \
  -subj "/CN=titan-lab-stranger" -keyout lab-stranger.key -out lab-stranger.crt 2>/dev/null
S="--cert lab-stranger.crt --key lab-stranger.key"
for p in /1.0 /1.0/cluster /1.0/cluster/members /1.0/images "/1.0/images?recursion=1" /1.0/nope; do
  echo "== stranger-cert GET $p"; curl -sk $S -w '\nHTTP %{http_code}\n' "https://127.0.0.1:8443$p"
done
echo "== no-cert GET /1.0/cluster/members"; curl -sk -w '\nHTTP %{http_code}\n' https://127.0.0.1:8443/1.0/cluster/members
