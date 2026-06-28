#!/usr/bin/env bash
set -euo pipefail

# Loopback ports: 8799 = client->gateway, 8901 = gateway->LLM (plain HTTP so Wireshark can read it).
REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK="$(mktemp -d)"
EMAIL="john.doe@gmail.com"

echo "==> building sphragis + demo upstream into $WORK"
( cd "$REPO" && go build -o "$WORK/sphragis" ./cmd/sphragis )
( cd "$REPO" && go build -o "$WORK/demoup" ./hack/demoup )

echo "==> starting demo LLM upstream on :8901"
"$WORK/demoup" >"$WORK/upstream.log" 2>&1 &
UP=$!

echo "==> starting sphragis gateway on :8799 (forwarding Anthropic path to :8901)"
SPHRAGIS_HOME="$WORK/home" \
SPHRAGIS_LISTEN_ADDR="127.0.0.1:8799" \
SPHRAGIS_ANTHROPIC_BASE_URL="http://127.0.0.1:8901" \
  "$WORK/sphragis" serve >"$WORK/serve.log" 2>&1 &
GW=$!

cleanup() { kill "$UP" "$GW" 2>/dev/null || true; }
trap cleanup EXIT
sleep 2

cat <<EOF

================================================================
WIRESHARK CAPTURE STEPS (run these now, servers are up)
================================================================
1. Open Wireshark -> double-click interface "Loopback: lo0".
2. Capture filter (optional):   tcp port 8799 or tcp port 8901
3. Then run the curl below (or press ENTER here to fire it).
4. Stop capture. Display filter:   http
   You will see TWO POST requests:
     - to 127.0.0.1:8799  (you -> gateway)   body has $EMAIL
     - to 127.0.0.1:8901  (gateway -> LLM)   body has [EMAIL_1], NO real email
5. Right-click each -> Follow -> HTTP Stream to read the bodies.

Prefer a file? Capture with tcpdump (needs sudo), open in Wireshark:
   sudo tcpdump -i lo0 -s0 -w "$WORK/sphragis.pcap" 'tcp port 8799 or tcp port 8901'
   # (run the curl in another terminal, then Ctrl-C; then: open "$WORK/sphragis.pcap")

NOTE: real LLM APIs are HTTPS, so on a production wire the gateway->LLM hop is
encrypted (Wireshark sees ciphertext). This demo uses a plain-HTTP upstream so
the redacted bytes are readable. The redaction itself is identical either way.
================================================================

EOF

read -r -p "Press ENTER to send the request (Ctrl-C to keep servers up and capture manually)... " _ || true

echo "==> sending request with a REAL email through the gateway"
curl -s -X POST http://127.0.0.1:8799/v1/messages -H 'Content-Type: application/json' \
  -d "{\"model\":\"claude-3\",\"messages\":[{\"role\":\"user\",\"content\":\"my email is $EMAIL\"}]}" >/dev/null

sleep 0.3
echo
echo "==> what the LLM actually received:"
grep -A1 'LLM RECEIVED' "$WORK/upstream.log" | tail -2
echo
echo "Email reached LLM? -> $(grep -c "$EMAIL" "$WORK/upstream.log" || true)  (0 = never leaked)"
echo "Servers still running; press Ctrl-C to stop them."
wait
