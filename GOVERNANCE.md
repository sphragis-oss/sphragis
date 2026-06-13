# Governance

This document describes how the Sphragis project is governed. It is intentionally
lightweight for an early-stage project and will evolve as the community grows,
with the goal of vendor-neutral, community-led governance suitable for a CNCF
project.

## Roles

### Contributor

Anyone who contributes code, documentation, issues, reviews, or other work.
Contributors do not need any special permissions. See
[CONTRIBUTING.md](CONTRIBUTING.md) to get started.

### Reviewer

A contributor with a sustained track record who is trusted to review pull
requests in a given area. Reviewers can approve PRs but do not merge.

### Maintainer

A reviewer with merge rights and responsibility for the project's direction,
releases, and health. Current maintainers are listed in
[MAINTAINERS.md](MAINTAINERS.md).

Maintainers are expected to:

- Review and merge contributions.
- Triage issues and shepherd releases.
- Uphold the [Code of Conduct](CODE_OF_CONDUCT.md).
- Respond to security reports per [SECURITY.md](SECURITY.md).

## Becoming a maintainer

A contributor may be nominated as a maintainer by an existing maintainer after a
sustained record of high-quality contributions and reviews. Nomination is
approved by lazy consensus of the current maintainers (no objection within 7
days). New maintainers are added via a pull request to
[MAINTAINERS.md](MAINTAINERS.md).

## Decision making

The project uses lazy consensus: most changes proceed through normal pull
request review. A maintainer approval and passing CI are sufficient to merge.

For larger decisions (architecture changes, breaking changes, governance
changes, adding or removing a maintainer), open an issue or PR labelled
`proposal`. The change is adopted if there are no sustained objections from
maintainers within 7 days. If consensus cannot be reached, a simple majority
vote of maintainers decides, with the project lead breaking ties.

## Project lead

While the project is small, the founding maintainer acts as project lead and
tie-breaker. This role dissolves into the maintainer group as governance
matures.

## Code of Conduct

All participation is governed by the [Code of Conduct](CODE_OF_CONDUCT.md).

## Changes to this document

Changes to governance are made by pull request and follow the larger-decision
process above.
