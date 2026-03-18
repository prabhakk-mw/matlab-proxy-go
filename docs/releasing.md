# Releasing

This document describes how to create releases of matlab-proxy-go.

## How Versioning Works

This project uses [Semantic Versioning](https://semver.org/) (`MAJOR.MINOR.PATCH`).

The version reported by `matlab-proxy --version` is determined at build time:

- **Release builds** — The CI pipeline injects the version from the git tag via `-ldflags`. The source code default is ignored.
- **Local builds** — Running `go build` without `-ldflags` produces a binary that reports `0.0.0-dev`, making it clear the binary was not built from the release pipeline.

There is no need to update the version in source code when cutting a release. The git tag is the single source of truth.

## Cutting a Stable Release

1. Ensure `main` is in a releasable state — CI is green, all intended changes are merged.

2. Tag the release:
   ```bash
   git tag v1.0.0
   git push origin v1.0.0
   ```

3. The **Release** workflow runs automatically and:
   - Cross-compiles binaries for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64
   - Injects `1.0.0` into each binary
   - Creates a GitHub Release tagged "Latest"
   - Uploads `.tar.gz` (linux/macOS) and `.zip` (windows) archives as release assets

4. Verify the release at `https://github.com/<org>/matlab-proxy-go/releases`.

## Cutting a Pre-Release (Release Candidate)

Pre-releases use a hyphenated suffix per semver (e.g., `-rc.1`, `-alpha.1`, `-beta.1`).

1. Tag the pre-release:
   ```bash
   git tag v1.0.0-rc.1
   git push origin v1.0.0-rc.1
   ```

2. The same **Release** workflow runs, but detects the hyphen in the tag and marks the GitHub Release as **Pre-release**. This means:
   - It is labeled "Pre-release" in the GitHub UI
   - It does not appear as "Latest release" on the repo page
   - `go install ...@latest` ignores it

3. After testing, promote to stable:
   ```bash
   git tag v1.0.0
   git push origin v1.0.0
   ```

## Version Progression Examples

```
v0.1.0-rc.1  ->  v0.1.0-rc.2  ->  v0.1.0        (release candidates)
v0.2.0-alpha.1  ->  v0.2.0-beta.1  ->  v0.2.0    (alpha to beta to stable)
v1.0.0  ->  v1.0.1                                (patch fix)
v1.0.0  ->  v1.1.0                                (new feature, backward compatible)
v1.0.0  ->  v2.0.0                                (breaking change)
```

## When to Bump Which Number

| Change | Bump | Example |
|---|---|---|
| Bug fix, no API change | PATCH | `1.0.0` -> `1.0.1` |
| New feature, backward compatible | MINOR | `1.0.0` -> `1.1.0` |
| Breaking change (env var renamed, API response changed, flag removed) | MAJOR | `1.0.0` -> `2.0.0` |

## Alternative: Creating a Release via GitHub UI

Instead of tagging from the CLI, you can create a release directly on GitHub:

1. Go to Releases -> "Create a new release"
2. Type the tag name (e.g., `v1.0.0`) — GitHub creates the tag for you
3. Select the target branch (`main`)
4. Write release notes
5. Publish

This also triggers the Release workflow, which attaches the compiled binaries to the release you just created.

**Do not do both** (push a tag from CLI *and* create a release in the UI for the same tag) — this can result in duplicate releases.

---

Copyright 2026 The MathWorks, Inc.
