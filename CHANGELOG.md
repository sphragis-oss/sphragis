# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/sphragis-oss/sphragis/commits/main
