# Roadmap

This roadmap describes the direction of Sphragis. It is a living document and
priorities may shift based on community feedback. Dates are intentionally
omitted while the project is early; items are grouped by horizon.

## Now (0.1.x)

- Harden the core proxy and redaction paths.
- Expand detector coverage and test fixtures across the supported wire formats.
- Container image and a basic Kubernetes deployment manifest.

## Next

- Redaction of model output (responses currently pass through unscanned).
- Configurable per-route redaction policy.
- Bundle a reference NER service so name/address/health detection works without
  an external dependency (today it requires `SPHRAGIS_NER_URL`).

## Later

- Verify and prune `.ots` proofs once Bitcoin attestations are available.
- Pluggable audit-log sinks and retention controls.
- Move toward vendor-neutral, community-led governance with the goal of a CNCF
  Sandbox submission.

## Done

- Built-in detectors for email, phone, IBAN, Luhn-validated cards, SSN, IPv4,
  PEM private keys, JWTs, secrets/bearer tokens, and provider API keys.
- Custom exact-match term list for names and codenames.
- External NER integration for name, address, and health entities.
- `sphragis anchor`: Merkle-root anchoring to public OpenTimestamps calendars
  with a `.ots` proof, including an on/off toggle for automatic periodic
  anchoring on a schedule.

## Out of scope for the open project

The open-source project stays focused on the gateway, redaction, and the
tamper-evident audit log. Multi-user administration, SSO/RBAC, team policy
management, and compliance report generation are delivered by a separate
commercial product built on top of this core, not in this repository.

## Contributing to the roadmap

Open an issue labelled `proposal` to suggest or discuss a roadmap item. See
[CONTRIBUTING.md](CONTRIBUTING.md) and [GOVERNANCE.md](GOVERNANCE.md).
