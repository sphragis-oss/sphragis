# /// script
# requires-python = ">=3.12"
# dependencies = ["torch", "transformers>=5.2"]
# ///
# SPDX-License-Identifier: Apache-2.0
import json
import logging
import os
import sys
import time
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path

import torch
from transformers import AutoTokenizer

sys.path.insert(0, str(Path(__file__).parent))
from modeling_pii_masking import PiiMaskingModel  # noqa: E402  vendored, loaded without trust_remote_code

MODEL = "perplexity-ai/PII-Tracer"
REVISION = "d25c16f2e57e321f6d2527715c01df9112f956f5"
HOST = os.environ.get("PII_TRACER_HOST", "127.0.0.1")
PORT = int(os.environ.get("PII_TRACER_PORT", "8788"))
MAX_BODY = 4 << 20
CHUNK_CHARS = 6000  # keeps each chunk under the 4096-token window
# sphragis maps PERSON to [NAME_n] and ADDRESS to [ADDRESS_n]; the rest keep their own kind
TYPES = {
    "private_person": "PERSON",
    "private_address": "ADDRESS",
    "private_email": "EMAIL",
    "private_phone": "PHONE",
    "private_url": "URL",
    "private_date": "DATE",
    "account_number": "ACCOUNT_NUMBER",
    "secret": "SECRET",
    "other_pii": "OTHER_PII",
}

log = logging.getLogger("pii-tracer")


def chunks(text: str):
    start = 0
    while start < len(text):
        end = min(start + CHUNK_CHARS, len(text))
        if end < len(text):
            cut = text.rfind("\n", start, end)
            end = cut + 1 if cut > start else end
        yield text[start:end]
        start = end


class Detector:
    def __init__(self):
        device = os.environ.get("PII_TRACER_DEVICE") or ("mps" if torch.backends.mps.is_available() else "cpu")
        self.tokenizer = AutoTokenizer.from_pretrained(MODEL, revision=REVISION)
        self.model = PiiMaskingModel.from_pretrained(MODEL, revision=REVISION).to(device).eval()
        log.info("loaded %s@%s on %s", MODEL, REVISION[:8], device)

    def entities(self, text: str) -> list[dict]:
        out, seen = [], set()
        for part in chunks(text):
            spans, _ = self.model.predict(part, tokenizer=self.tokenizer)
            for s in spans:
                key = (s.label, part[s.start:s.end])
                if key not in seen:
                    seen.add(key)
                    out.append({"type": TYPES.get(s.label, s.label.upper()), "text": key[1]})
        return out


def handler(detector: Detector):
    class Handler(BaseHTTPRequestHandler):
        def do_POST(self):
            length = int(self.headers.get("Content-Length") or 0)
            if not 0 < length <= MAX_BODY:
                return self.reply(413 if length else 411, {"error": "body required, max 4 MiB"})
            try:
                text = json.loads(self.rfile.read(length))["text"]
                if not isinstance(text, str):
                    raise TypeError
            except (ValueError, KeyError, TypeError):
                return self.reply(400, {"error": 'expected {"text": "..."}'})
            start = time.monotonic()
            entities = detector.entities(text)
            # counts only, never entity text, so logs stay PII-free
            log.info("chars=%d entities=%d ms=%d", len(text), len(entities), (time.monotonic() - start) * 1000)
            self.reply(200, {"entities": entities})

        def reply(self, code: int, body: dict):
            data = json.dumps(body, ensure_ascii=False).encode()
            self.send_response(code)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            self.wfile.write(data)

        def log_message(self, *args):
            pass

    return Handler


def main():
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(name)s %(message)s")
    os.environ.setdefault("TOKENIZERS_PARALLELISM", "false")
    server = HTTPServer((HOST, PORT), handler(Detector()))
    log.info("listening on http://%s:%d", HOST, PORT)
    server.serve_forever()


if __name__ == "__main__":
    main()
