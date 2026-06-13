# Security Policy

## Reporting a vulnerability

Please report security vulnerabilities privately. Do not open a public issue for
a suspected vulnerability.

- Preferred: open a private [GitHub Security Advisory](https://github.com/sphragis-oss/sphragis/security/advisories/new).
- Alternatively, email **nonicked@protonmail.com** with the details.

Please include:

- A description of the issue and its impact.
- Steps to reproduce, or a proof of concept.
- Affected version or commit.
- Any suggested remediation, if you have one.

You will receive an acknowledgement within 5 business days. We will keep you
informed as we investigate, agree a disclosure timeline, and ship a fix. We aim
to triage within 10 business days and to credit reporters who wish to be named.

## Supported versions

Sphragis is pre-1.0. Until a stable release line exists, only the latest
released version receives security fixes.

| Version | Supported |
|---------|-----------|
| latest release | yes |
| older | no |

## Scope

Sphragis is a self-hosted proxy that runs inside your own trust boundary. The
most security-relevant areas are:

- Redaction correctness (personal data leaking upstream despite redaction).
- Audit-log integrity (the hash chain or verification being defeated).
- The proxy mishandling credentials or upstream connections.

Reports in these areas are especially valued.
