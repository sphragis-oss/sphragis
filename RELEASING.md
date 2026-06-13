# Releasing Sphragis

Sphragis follows [Semantic Versioning](https://semver.org). While the project is
pre-1.0, the proxy wire behaviour, CLI surface, audit-log format, and config
environment variables may still change; expect breaking changes in minor
releases until 1.0.0.

## Versioning

Versions are `0.MINOR.PATCH`.

- PATCH (`0.x.Z`): backward-compatible bug fixes, security fixes, docs, and
  internal refactors. No new user-facing capability and no breaking change.
- MINOR (`0.X.0`): new features, and breaking changes (allowed in minor while
  pre-1.0). A minor release may batch several features and fixes together.
- 1.0.0: cut once the proxy wire compatibility, CLI surface, audit-log format,
  and config env vars are committed to stability. After 1.0.0, breaking changes
  require a major bump.

Security fixes ship as soon as they are ready, as a PATCH on the current minor,
even when other work is in progress.

## When to cut a release

Do not cut a release for every change. Land changes on `main` and let them
accumulate under the `[Unreleased]` section of `CHANGELOG.md`. Cut a release
when:

- a meaningful set of features is ready (MINOR), or
- a bug or security issue needs to ship (PATCH).

Only tags trigger a release: the `release` workflow runs on `v*` tags, while
pushes to `main` run CI only. That is what makes batching safe.

## Release process

1. Make sure `main` is green and the working tree is clean.
2. In `CHANGELOG.md`, move the `[Unreleased]` notes under a new `[x.y.z]`
   heading with today's date, and add the version-compare link at the bottom.
3. Commit the changelog with a DCO sign-off: `git commit -s`.
4. Tag and push:

   ```sh
   git tag -a vX.Y.Z -m "vX.Y.Z"
   git push origin main vX.Y.Z
   ```

5. The `release` workflow runs GoReleaser: it builds the binaries, publishes the
   GitHub release with checksums, and pushes the updated Homebrew cask to
   `sphragis-oss/homebrew-sphragis`.
6. Verify the GitHub release assets, the cask version, and `sphragis version`
   from a downloaded binary.

## CHANGELOG flow

`CHANGELOG.md` follows [Keep a Changelog](https://keepachangelog.com). All
user-facing changes land under `[Unreleased]` as they merge; cutting a release
renames that section to the new version and starts a fresh `[Unreleased]`.
