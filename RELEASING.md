# Releasing

Versions have three parts: v1.0.0, then v1.0.1, v1.0.2… for fixes, and v1.1.0 for new features.

1. Bring main up to date and run the tests:

   ```bash
   git pull --rebase origin main
   go test ./internal/...
   ```

2. Bump `version` in `mygo.json`, and add a section for it at the top of `CHANGELOG.md`, in English and then in Chinese, saying what changed for people using the app. Commit both in a commit titled just the version, e.g. `1.0.1`. The app's update window and the release page show that section; `mygo build` fails without one.

3. Push main first, and tag only once that push has gone through:

   ```bash
   git push origin main
   git tag v1.0.1
   git push origin v1.0.1
   ```

   If a tag went up on the wrong commit, or its run failed, delete the tag and its draft before tagging again:

   ```bash
   git push origin :refs/tags/v1.0.1
   gh release delete v1.0.1 --yes
   ```

4. `.github/workflows/release.yml` builds Apple Silicon macOS and Linux (amd64, arm64), one after the other, and uploads them, their delta updates and the signed update manifests to a draft release. The manifests are signed with the repository secret `MYGO_UPDATER_PRIVATE_KEY`, whose public key is `updates.publicKey` in `mygo.json`. Wait for every job to pass:

   ```bash
   gh run watch "$(gh run list --workflow release.yml -L1 --json databaseId -q '.[0].databaseId')" --exit-status
   ```

   If a job fails, don't publish; fix it first.

5. Publish the draft and mark it latest. Installed apps update to the latest release, so only publish what should reach them:

   ```bash
   gh release edit v1.0.1 --draft=false --latest
   ```

6. Check that the update feed shows the new version:

   ```bash
   curl -sL https://github.com/aronnaxlin/mygoify/releases/latest/download/update-linux-amd64.json
   ```

The macOS app is signed ad hoc and not notarized: there is no Developer ID. README.md says how macOS lets it in.
