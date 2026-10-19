#!/bin/sh
# Inside incus-lab, after lab-tls-setup.sh: what an untrusted client and an unknown path get back.
cd /tmp
for p in /1.0/images /1.0/cluster /1.0/projects /1.0/nope; do
  echo "== untrusted GET $p"; curl -sk -w '\nHTTP %{http_code}\n' https://127.0.0.1:8443$p
done
echo "== trusted GET /1.0/nope"; curl -sk -w '\nHTTP %{http_code}\n' --cert lab-client.crt --key lab-client.key https://127.0.0.1:8443/1.0/nope
echo "== trusted GET /1.0/images (no recursion)"; curl -sk -w '\nHTTP %{http_code}\n' --cert lab-client.crt --key lab-client.key https://127.0.0.1:8443/1.0/images
echo "== trusted GET /1.0/cluster/members"; curl -sk -w '\nHTTP %{http_code}\n' --cert lab-client.crt --key lab-client.key https://127.0.0.1:8443/1.0/cluster/members
