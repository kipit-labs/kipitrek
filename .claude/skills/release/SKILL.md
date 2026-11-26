---
name: release
description: Cut a kipitrek release and deploy it — bump `VERSION` in a `chore(release): X.Y.Z` PR and merge it (the release workflow tags and publishes the image), then bump the image in compose.yaml (kipitiny git sync deploys it). Use when the user says "release", "cut a release", "deploy", or invokes /release [version|major|minor|patch].
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

6. **Deploy.** Prod is the kipitiny project `gpx-viewer` (`manager.mathieuhoyer.fr`), synced from `compose.yaml`
   on `main`. Pin the tag only now that the image exists, so a sync never asks for a missing image:
   ```sh
   git switch -c chore/deploy-X.Y.Z
   # compose.yaml: image: ghcr.io/kipit-labs/kipitrek:X.Y.Z (exact version, never latest); nothing else changes
   git commit -am "chore(deploy): run X.Y.Z on kipitiny"
   git push -u origin chore/deploy-X.Y.Z
   gh pr create --title "chore(deploy): run X.Y.Z on kipitiny" --body "Deploy X.Y.Z."
   gh pr merge --rebase --auto
   ```
   kipitiny polls the branch every 5 minutes; the kipitiny MCP `sync_project_git` (project `gpx-viewer`) deploys at
   once, then check `get_app_status` for `gpx-viewer/app`.

7. **Report**: version, GitHub release URL, image tag, deploy commit and the kipitiny deployment status.
