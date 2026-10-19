#!/bin/sh
# Run inside the incus-lab VM (limactl shell incus-lab -- sh < this file). Lab only: opens Incus on loopback and
# trusts a throwaway client certificate so the TLS answers (untrusted vs trusted) can be recorded for the stub oracle.
set -eu
sudo incus config set core.https_address 127.0.0.1:8443
cd /tmp && rm -f lab-client.crt lab-client.key
openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:secp384r1 -nodes -days 30 \
  -subj "/CN=titan-lab-client" -keyout lab-client.key -out lab-client.crt 2>/dev/null
sudo incus config trust remove titan-lab-client 2>/dev/null || true
sudo incus config trust add-certificate lab-client.crt --name titan-lab-client
echo "== untrusted (no client cert)"; curl -sk https://127.0.0.1:8443/1.0 | head -c 1200; echo
echo "== trusted (client cert)"; curl -sk --cert lab-client.crt --key lab-client.key https://127.0.0.1:8443/1.0 > /tmp/trusted.json; python3 -c "import json;d=json.load(open('/tmp/trusted.json'))['metadata'];print('auth=',d['auth'],'auth_methods=',d.get('auth_methods'),'server_name=',d['environment'].get('server_name'),'server_clustered=',d['environment'].get('server_clustered'))"
echo "== server certificate fingerprint (sha256 of DER)"; openssl x509 -in /var/lib/incus/server.crt -outform DER 2>/dev/null | sha256sum || sudo sh -c 'openssl x509 -in /var/lib/incus/server.crt -outform DER | sha256sum'
echo "== environment.certificate_fingerprint"; python3 -c "import json;print(json.load(open('/tmp/trusted.json'))['metadata']['environment'].get('certificate_fingerprint'))"
echo "== cluster (trusted)"; curl -sk --cert lab-client.crt --key lab-client.key https://127.0.0.1:8443/1.0/cluster; echo
echo "== cluster members (trusted)"; curl -sk --cert lab-client.crt --key lab-client.key https://127.0.0.1:8443/1.0/cluster/members; echo
