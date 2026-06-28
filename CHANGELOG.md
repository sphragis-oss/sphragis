# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.6.0] - 2026-06-28

### Added

- Configurable upstream auto-detection. The gateway now resolves the target
  provider from auth headers (`x-goog-api-key`, `anthropic-version`), the Gemini
  `?key=` query, and the request's model name, in addition to the path. This
  routes shared paths like `/v1/models` to the right backend (previously they
  always fell through to OpenAI). Enabled by default; disable for path-only
  routing with `SPHRAGIS_ROUTE_AUTODETECT=false` or `route_autodetect: false`.

### Fixed

- NER redaction could leak PII. Overlapping entities (e.g. `Maria` and
  `Maria Papadopoulou`) and substring matches inside already-emitted tokens
  could leave text exposed or corrupt a token. Entities are now tokenized
  longest-first and only within plain-text spans, never rewriting an existing
  `[KIND_n]` token.
- Vault persistence rewrote the entire sealed database on every token
  assignment (O(N^2) I/O on large requests). Assignments are now batched and
  flushed once per request.
- Streaming responses were buffered until a newline, stalling long outputs that
  contained none. The stream redactor now flushes at the last whitespace
  boundary outside a reserved tail, so long lines stream incrementally without
  splitting a value across chunks.

## [0.5.1] - 2026-06-20

### Added

- Opt-in built-in NER (`SPHRAGIS_NER_BUILTIN=true` or `ner_builtin: true`).
  Dependency-free name and street-address detection using an embedded gazetteer
  of common given names plus conservative heuristics (titles, trigger phrases,
  `<number> <Street>`). Precision-biased (a name matches only when followed by a
  capitalized surname) and off by default. The external `SPHRAGIS_NER_URL`
  service stays available for higher accuracy and health terms.

## [0.5.0] - 2026-06-20

### Added

- Google Gemini support. The gateway redacts the Gemini `generateContent` /
  `streamGenerateContent` format (request `contents[].parts[].text`,
  `systemInstruction`, and tool call/response args; response and SSE
  `candidates[].content.parts[].text`) and routes `/v1beta/...` paths to
  `SPHRAGIS_GOOGLE_BASE_URL` (default
  `https://generativelanguage.googleapis.com`).
- Azure OpenAI and Ollama now work through the gateway: both speak the OpenAI
  wire format, so point `SPHRAGIS_UPSTREAM_BASE_URL` at the Azure resource or
  Ollama and existing redaction applies.
- Supply-chain hardening for releases. Each release now carries keyless cosign
  signatures over the checksums and the container image, an SBOM (`*.sbom.json`,
  via syft) per archive, and SLSA build-provenance attestations for both the
  binaries and the image (verifiable with `cosign verify` / `gh attestation
  verify`). See "Verifying releases" in the README.
- CI now runs the redaction fuzz target (`FuzzRedact`) on every push and pull
  request.

### Changed

- The upstream request now forwards the full request URI including the query
  string, so Gemini's `?key=` and Azure's `?api-version=` reach the provider.

## [0.4.6] - 2026-06-20

### Added

- Read-only web UI at `/ui` (served by `sphragis serve`). A single
  self-contained page with a redaction playground (paste text, see the tokens
  and per-kind counts; the preview is never logged or forwarded) and an audit
  view (chain status, record count, Merkle root, per-kind totals, recent
  requests by metadata only). No external assets or dependencies. New
  endpoints: `GET /ui`, `POST /ui/redact`, `GET /ui/audit`.

### Fixed

- `sphragis serve` now creates the state directory (`SPHRAGIS_HOME`) on startup,
  like the daemon does. Previously a foreground `serve` against a fresh home
  failed with `open .../audit.jsonl: no such file or directory`.

## [0.4.5] - 2026-06-20

### Added

- Opt-in EU PII pack (`SPHRAGIS_EU_PACK=true` or `eu_pack: true`). Adds
  EU-specific detectors that run before the built-ins: EU VAT numbers (all 27
  member states, country-prefixed), Greek AMKA (11 digits, birthdate prefix,
  Luhn-checked), and Greek AFM / tax id (9 digits, modulo-11 check digit). Off
  by default, since these patterns can match unrelated numbers in non-EU data.

## [0.4.0] - 2026-06-20

### Added

- Official container image. A multi-arch (`linux/amd64`, `linux/arm64`)
  distroless image is published to `ghcr.io/sphragis-oss/sphragis` on each
  release, built by GoReleaser. It runs as a non-root user and listens on
  `:8787`; mount a volume at `/data` (the image's `SPHRAGIS_HOME`) to persist
  the audit log and vault. A `Dockerfile` for building the image from source is
  included in the repository.

### Removed

- `ROADMAP.md`. Direction is now tracked via GitHub issues and milestones
  instead of a checked-in roadmap document.

## [0.3.0] - 2026-06-14

### Added

- Reversible tokenization (opt-in). With a 32-byte key (`SPHRAGIS_VAULT_KEY` or
  `SPHRAGIS_VAULT_KEYFILE`) the gateway records each token's original value in a
  local vault sealed with AES-256-GCM, and tokens become gateway-global and
  unique. `sphragis reveal <file>` restores originals inside the trust boundary.
  Without a key, nothing changes and no originals are stored.
- Prometheus metrics at `/metrics` (redaction counts by kind and direction,
  requests by route, upstream latency, audit-append failures), as zero-dependency
  text exposition.
- Optional YAML config file (`~/.sphragis/sphragis.yaml` or `SPHRAGIS_CONFIG`).
  Precedence is env > file > default, so env-only setups are unchanged.
- Response (model output) redaction. JSON responses and streamed SSE bodies are
  now scanned before reaching the client, so PII the model emits does not land in
  the calling app or its logs. Covers OpenAI chat completions, OpenAI Responses,
  Anthropic Messages, and legacy Text Completions. For streams, assistant text is
  buffered across chunks and flushed at line boundaries with stable token
  numbering, so a value split across two SSE deltas is still tokenized. Streamed
  bodies use the regex/custom detectors only (NER runs on non-streamed bodies).
- Multi-provider auto-routing: a single gateway routes by request path, sending
  Anthropic paths (`/v1/messages`, `/v1/complete`) to
  `SPHRAGIS_ANTHROPIC_BASE_URL` (default `https://api.anthropic.com`) and OpenAI
  paths to `SPHRAGIS_OPENAI_BASE_URL` (default `https://api.openai.com`). One
  `sphragis` instance now protects Claude Code and Codex at the same time into a
  single audit log. `SPHRAGIS_UPSTREAM_BASE_URL` still overrides all routes.

### Changed

- `sphragis status` logo redrawn to match the new project seal: a dashed cream
  ring around redaction bars with the middle bar in red, replacing the gold
  shield.

### Fixed

- Forward all client request headers upstream so provider auth works for both
  protocols. Previously only `Authorization` was passed through, which broke
  Anthropic's `x-api-key` / `anthropic-version` auth used by Claude Code.

## [0.2.0] - 2026-06-13

### Added

- Cobra-based CLI with grouped help (Gateway / Audit / Other Commands),
  `--version`/`-v`, and generated shell completion (`completion`).
- `sphragis status` renders a colored shield logo with the gateway state,
  audit-chain health, redaction totals, and an Errors/Warnings summary laid out
  beside the logo (cilium-style). Color auto-disables on non-TTY or `NO_COLOR`;
  force it with `FORCE_COLOR`/`CLICOLOR_FORCE`.
- `anchor` is now a parent command with `now`, `on`, `off`, and `status`
  subcommands.

### Changed

- Exclude the CodeQL `go/weak-sensitive-data-hashing` query via
  `.github/codeql/codeql-config.yml`. It repeatedly flagged the audit log's
  SHA-256 hash-chain (`chainHash`) and payload hash as insecure password
  hashing. SHA-256 is the correct, intended algorithm for tamper-evident
  chaining and Merkle roots, and the project stores no passwords or
  credentials, so the finding is a false positive. Per-alert dismissals did not
  hold because each change to the hashing path produced a new alert
  fingerprint. Every other CodeQL query stays active.

## [0.1.1] - 2026-06-13

### Added

- `sphragis version` command (also `-v`/`--version`), with the version stamped
  into release binaries at build time.

### Security

- Include the redaction counts (`pii_redacted`) in the audit-log hash chain, so
  the record of what was redacted is now tamper-evident. In 0.1.0 these counts
  sat outside the chained digest and could be altered without breaking
  verification. This changes the chain-hash input, so audit logs written by
  0.1.0 will not verify under 0.1.1.

## [0.1.0] - 2026-06-13

### Added

- OpenAI- and Anthropic-compatible proxy that redacts personal data in request
  bodies before forwarding upstream.
- Path-based redaction across OpenAI chat completions, OpenAI Responses,
  Anthropic Messages, Message Batches, and legacy Text Completions.
- Built-in detectors, replacing matches with stable `[KIND_n]` tokens: email,
  phone, IBAN, Luhn-validated card numbers, US SSN, IPv4, PEM private keys, JWTs,
  keyed secrets and bearer tokens, and provider API keys (OpenAI/Anthropic, AWS,
  GitHub, Google, Slack, Stripe, SendGrid).
- Custom exact-match term list via `SPHRAGIS_CUSTOM_TERMS_FILE` for names and
  codenames.
- Optional external NER detector (`SPHRAGIS_NER_URL`) for name, address, and
  health entities; fails open so an NER outage never blocks regex redaction.
- Tamper-evident, hash-chained append-only audit log with per-record SHA-256
  chaining and fail-closed behaviour when a record cannot be written.
- `sphragis verify` command that replays the log, checks chain integrity, and
  prints the Merkle root of payload hashes.
- `sphragis anchor` command that stamps the audit-log Merkle root to public
  OpenTimestamps calendars and writes a `.ots` proof. Calendars are overridable
  via `SPHRAGIS_OTS_CALENDARS`.
- Streaming (SSE) responses passed through with per-chunk flushing.
- Configuration via environment variables.

### Technical details

- Written in Go 1.26.
- Dependency: `github.com/nbd-wtf/opentimestamps` for Merkle-root anchoring.
- Apache 2.0 licensed, SPDX headers on all source files.

[Unreleased]: https://github.com/sphragis-oss/sphragis/compare/v0.5.1...HEAD
[0.5.1]: https://github.com/sphragis-oss/sphragis/compare/v0.5.0...v0.5.1
[0.5.0]: https://github.com/sphragis-oss/sphragis/compare/v0.4.6...v0.5.0
[0.4.6]: https://github.com/sphragis-oss/sphragis/compare/v0.4.5...v0.4.6
[0.4.5]: https://github.com/sphragis-oss/sphragis/compare/v0.4.0...v0.4.5
[0.4.0]: https://github.com/sphragis-oss/sphragis/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/sphragis-oss/sphragis/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/sphragis-oss/sphragis/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/sphragis-oss/sphragis/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/sphragis-oss/sphragis/releases/tag/v0.1.0
