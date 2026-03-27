# Releasing a New Version

## Prerequisites

- Push access to `kestrelportfolio/cli` on GitHub
- All changes committed and pushed to `main`
- `HOMEBREW_TAP_TOKEN` secret configured on the CLI repo (see [One-Time Setup](#one-time-setup))

## Steps

1. **Decide the version number** — we use [semantic versioning](https://semver.org/):
   - `v0.1.0` → first release
   - `v0.1.1` → bug fix
   - `v0.2.0` → new feature (e.g., new command)
   - `v1.0.0` → stable public release

2. **Tag the release:**
   ```bash
   git tag v0.2.0
   git push --tags
   ```

3. **Wait for GitHub Actions** — the release workflow runs automatically:
   - Builds binaries for macOS (arm64 + amd64) and Linux (arm64 + amd64)
   - Creates a GitHub Release with the binaries attached
   - Pushes an updated Homebrew formula to `kestrelportfolio/homebrew-tap`
   - Generates a changelog from commit messages
   - Takes about 1-2 minutes

4. **Verify the release:**
   - Check https://github.com/kestrelportfolio/cli/releases
   - Download a binary and run `kestrel version` to confirm
   - `brew update && brew upgrade kestrel` should pick up the new version

## Local Testing (without publishing)

To test the full build locally without creating a release:

```bash
make release-snapshot
```

This builds all platform binaries into `dist/` but doesn't publish anything. Useful for verifying the build works before tagging.

## How Version Injection Works

The version string is injected at build time via Go's `-ldflags` mechanism. The variable `internal/api.Version` defaults to `"dev"` in the source code, but GoReleaser (and the Makefile) replace it with the git tag at build time.

- `make build` → `kestrel dev`
- `make build VERSION=0.2.0` → `kestrel 0.2.0`
- `git tag v0.2.0 && push` → GoReleaser builds `kestrel 0.2.0`

## Troubleshooting

**Release workflow didn't trigger:** Make sure you pushed the tag, not just created it locally. `git push --tags` pushes all tags.

**Build failed in CI:** Check the Actions tab on GitHub. Common issues:
- `go mod tidy` needed (GoReleaser runs this, but if there's a mismatch it'll fail)
- Module path doesn't match (if you renamed the GitHub org/repo)

**Want to redo a release:** Delete the tag locally and on GitHub, fix the issue, then re-tag:
```bash
git tag -d v0.2.0
git push origin :refs/tags/v0.2.0
# fix the issue, commit, push
git tag v0.2.0
git push --tags
```

**Homebrew formula didn't update:** Check that the `HOMEBREW_TAP_TOKEN` secret is set and the token hasn't expired. GoReleaser logs in the Actions run will show the error.

## One-Time Setup

These steps only need to be done once when setting up the release pipeline.

### 1. Create a Personal Access Token (PAT)

GoReleaser needs a token to push the Homebrew formula to the `homebrew-tap` repo. The default `GITHUB_TOKEN` in Actions only has access to the CLI repo itself.

1. Go to https://github.com/settings/tokens?type=beta (fine-grained tokens)
2. Create a new token:
   - **Name:** `kestrel-homebrew-tap`
   - **Expiration:** 1 year (set a calendar reminder to rotate)
   - **Repository access:** Select `kestrelportfolio/homebrew-tap` only
   - **Permissions:** Contents → Read and write
3. Copy the token

### 2. Add the token as a repo secret

1. Go to https://github.com/kestrelportfolio/cli/settings/secrets/actions
2. Click "New repository secret"
3. Name: `HOMEBREW_TAP_TOKEN`
4. Value: paste the token from step 1

### 3. Verify Homebrew tap repo

The `kestrelportfolio/homebrew-tap` repo needs a `Formula/` directory. If it doesn't exist, create it:
```bash
gh repo clone kestrelportfolio/homebrew-tap
cd homebrew-tap
mkdir -p Formula
touch Formula/.keep
git add . && git commit -m "Init" && git push
```

After the first release, GoReleaser will push `Formula/kestrel.rb` automatically.
