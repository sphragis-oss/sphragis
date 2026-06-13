# Contributing to Sphragis

Thanks for your interest in contributing. This document covers how to set up,
make a change, and get it merged.

By participating you agree to the [Code of Conduct](CODE_OF_CONDUCT.md).

## Prerequisites

- Go 1.26 or newer.
- `git`.

## Local setup

```bash
git clone https://github.com/sphragis-oss/sphragis.git
cd sphragis
make build
make test
```

Common targets:

```bash
make test     # go test ./...
make vet      # go vet ./...
make fmt      # go fmt ./...
make build    # build the sphragis binary
make run      # build and run the gateway
```

## Making a change

1. Open an issue first for anything non-trivial, so the approach can be agreed
   before you invest time.
2. Create a branch from `main`.
3. Keep changes focused. One logical change per pull request.
4. Add or update tests. New behaviour needs test coverage; bug fixes need a test
   that fails before the fix and passes after.
5. Run `make fmt`, `make vet`, and `make test` before pushing.

## Coding style

- Follow [Effective Go](https://go.dev/doc/effective_go) and standard `gofmt`.
- Handle errors explicitly; use the standard library `log/slog` for logging.
- Keep comments to a single line, explaining *why* rather than *what*. Prefer no
  comment over a redundant one.
- Every Go source file starts with the SPDX header:
  `// SPDX-License-Identifier: Apache-2.0`.

## Commit messages and DCO sign-off

This project requires the [Developer Certificate of Origin](https://developercertificate.org/)
(DCO) on every commit. Sign off your commits with:

```bash
git commit -s -m "your message"
```

This adds a `Signed-off-by` trailer certifying you have the right to submit the
work under the project's license. The DCO check in CI enforces this.

Write clear commit messages: a short imperative summary line, then a body
explaining the why if it is not obvious.

## Pull requests

- Fill out the pull request template.
- Make sure CI is green (lint, tests, DCO).
- A maintainer will review. Address feedback by pushing additional commits;
  avoid force-pushing during review so reviewers can see incremental changes.
- PRs are merged by a maintainer once approved and green.

## License

Sphragis is licensed under [Apache 2.0](LICENSE). By contributing, you agree
that your contributions are licensed under the same terms.

## Reporting security issues

Do not file security vulnerabilities as public issues. Follow
[SECURITY.md](SECURITY.md) instead.
