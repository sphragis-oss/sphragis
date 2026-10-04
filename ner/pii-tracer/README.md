# PII-Tracer NER service

Local NER sidecar for `SPHRAGIS_NER_URL`, backed by Perplexity's
[PII-Tracer](https://huggingface.co/perplexity-ai/PII-Tracer) (~600M bidirectional
Qwen3 encoder, 9 PII categories). It finds names, addresses and other free-text
PII that the regex detectors cannot match.

## Run

```bash
uv run --script ner/pii-tracer/server.py   # listens on 127.0.0.1:8788
export SPHRAGIS_NER_URL=http://127.0.0.1:8788
./sphragis serve
```

The first start downloads `torch` and the model weights (safetensors) into the
Hugging Face cache. It uses Apple `mps` when available, otherwise CPU.

```bash
curl -s localhost:8788 -d '{"text":"Hi, I am Maria Papadopoulou, Ermou 12, Athens"}'
# {"entities": [{"type": "PERSON", "text": "Maria Papadopoulou"}, {"type": "ADDRESS", "text": "Ermou 12, Athens"}]}
```

| PII-Tracer label | Returned type | Sphragis token |
|---|---|---|
| `private_person` | `PERSON` | `[NAME_n]` |
| `private_address` | `ADDRESS` | `[ADDRESS_n]` |
| `private_email`, `private_phone`, `private_url`, `private_date` | `EMAIL`, `PHONE`, `URL`, `DATE` | `[EMAIL_n]` ... |
| `account_number`, `secret`, `other_pii` | `ACCOUNT_NUMBER`, `SECRET`, `OTHER_PII` | `[ACCOUNT_NUMBER_n]` ... |

| Variable | Default |
|---|---|
| `PII_TRACER_HOST` | `127.0.0.1` |
| `PII_TRACER_PORT` | `8788` |
| `PII_TRACER_DEVICE` | `mps` if available, else `cpu` |

## Supply chain

`modeling_pii_masking.py` and `LICENSE` (MIT) are vendored unmodified from the
model repo at revision `d25c16f2e57e321f6d2527715c01df9112f956f5`, and the weights
are pinned to the same revision. The model class is imported from this directory,
so nothing is loaded with `trust_remote_code`. To upgrade, diff the upstream file,
review it, and bump `REVISION` in `server.py` together with the vendored copy.

## Limits

- Sphragis gives NER 5 s per text field and fails open, so a slow call means
  regex-only redaction for that field. Input is chunked at 6000 characters.
- NER does not run on streamed request bodies (see `internal/redact/stream.go`).
- Results are cached in memory (4096 texts). Sphragis redacts every field of
  every request, and agents resend the whole conversation each turn, so only
  new text pays the model cost. A field that timed out is still computed and
  cached, so it is redacted from the next request on.
- Measured on Apple M-series `mps`: ~0.13 ms per character, so fields over
  ~37k characters exceed the 5 s budget. CPU is ~50x slower.
- English is reliable. Greek works on full sentences but is experimental: short
  inputs can produce wrong span boundaries (same output on `mps` and CPU, so it
  is the model, not the device).
- Sphragis queries only the text between existing tokens, so a span never
  straddles a token. The cost is less context per call, which can over-redact
  a neighbouring word or give a repeated name a new number.
- Detection is probabilistic. The service logs counts and timings only, never
  entity text.
