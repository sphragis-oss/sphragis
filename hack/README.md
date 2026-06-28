# hack/

Developer demo helpers. Not part of the gateway runtime.

## wireshark-demo.sh

Shows that PII never leaves the gateway in cleartext to the LLM, capturable in Wireshark.

```bash
./hack/wireshark-demo.sh
```

It builds `sphragis` and a plain-HTTP stand-in LLM (`hack/demoup`), runs both on loopback
(gateway `:8799`, upstream `:8901`), and walks you through capturing the two hops on `lo0`:
client to gateway still has the real email, gateway to LLM has `[EMAIL_1]` instead.

Real LLM APIs are HTTPS, so on a production wire the gateway to LLM hop is encrypted; the demo
uses a plain-HTTP upstream so the redacted bytes are readable. For the in-process equivalent see
`TestRequestNeverLeaksRealPIIToUpstream` in `internal/proxy/leak_test.go`.
