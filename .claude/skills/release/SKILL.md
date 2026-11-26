---
name: release
description: Cut a kipitrek release and deploy it — bump `VERSION` in a `chore(release): X.Y.Z` PR and merge it (the release workflow tags and publishes the image), then bump the image in the homelab ops repo (ArgoCD deploys it). Use when the user says "release", "cut a release", "deploy", or invokes /release [version|major|minor|patch].
argument-hint: "[patch|minor|major|X.Y.Z]"
disable-model-invocation: true
---

# Release

A release is a chore PR that bumps the root `VERSION` file. Merging it into `main` runs
`.github/workflows/release.yml`: it tags the merged commit `X.Y.Z`, publishes `ghcr.io/kipit-labs/kipitrek:X.Y.Z` +
`:latest` (amd64, arm64) and the GitHub release with generated notes. It refuses a `VERSION` that isn't above the last
tag. Tags are bare semver, no `v` prefix. Prod only changes in step 6.

## Steps

1. **Preflight** — stop and report if any fails:
   - on `main`, working tree clean (`git status --porcelain` empty)
   - `git fetch --tags origin`, then local `main` == `origin/main`
   - there are commits since the last tag (`git describe --tags --abbrev=0`)
   - `make test` and `make lint` green

2. **Pick the version.** `$ARGUMENTS` may be `X.Y.Z` or `major|minor|patch`. Without one, infer from the
   conventional commits since the last tag (`git log <last>..HEAD --format=%s`): any `!:` or `BREAKING CHANGE` →
   major, any `feat` → minor, otherwise patch.

3. **Confirm with the user**: last tag → new version and the commit subjects included. Wait for a yes.

4. **Release PR, auto-merged:**
   ```sh
   git switch -c chore/release-X.Y.Z
   printf 'X.Y.Z\n' > VERSION
   git commit -am "chore(release): X.Y.Z"
   git push -u origin chore/release-X.Y.Z
   gh pr create --title "chore(release): X.Y.Z" --body "Release X.Y.Z."
   gh pr merge --rebase --auto
   ```
   No AI attribution on the commit or the PR. Never push a tag by hand: the workflow makes it.

5. **Wait for the image.** Once the PR is `MERGED`: `git switch main && git pull --ff-only && git branch -d
   chore/release-X.Y.Z`, then watch the run (`gh run list --workflow release.yml --limit 1`, `gh run watch <id>
   --exit-status`; on failure stop and show `gh run view <id> --log-failed`). Check the image is pullable:
   `docker manifest inspect ghcr.io/kipit-labs/kipitrek:X.Y.Z`.

6. **Deploy.** Prod runs on the homelab k3s cluster: ArgoCD auto-syncs `main` of `~/work/homelab/ops` (origin
   `git.mathieuhoyer.fr/homelab/ops`), so the push is the deploy. Pin the tag only now that the image exists:
   ```sh
   cd ~/work/homelab/ops
   git status --porcelain            # must be empty, else stop
   git checkout main && git pull --ff-only
   docker buildx imagetools inspect ghcr.io/kipit-labs/kipitrek:X.Y.Z --format '{{.Manifest.Digest}}'
   ```
   In `clusters/homelab/infrastructure/gpx-viewer/deployment.yaml`, set the image to
   `ghcr.io/kipit-labs/kipitrek:X.Y.Z@<digest>` (exact version and digest, never `latest`); `git diff` shows only that
   line. Then `git commit -am "chore(gpx-viewer): bump image to X.Y.Z" && git push origin main`. Check the rollout
   from `ssh mat@morpheus`: `sudo -n kubectl -n gpx-viewer rollout status deploy/gpx-viewer`.

7. **Report**: version, GitHub release URL, image tag, ops commit and the rollout status.
