# Release process

## Steps

1. Checkout the repository on the correct branch and changeset (`main`).Creates a new branch if it is the first release for a "minor" version: `git checkout -b vX.Y`.
2. Run the bundle generation:
    - For a release candidate, run the following command, locally: `make VERSION=x.v.z-rc.w bundle`
      For example, to generate the release version `0.3.0-rc.2`, run: `make VERSION=0.3.0-rc.2 bundle`
    - For a release, command is the same with final version: `make VERSION=x.v.z bundle`

3. Commit all the changes generated from the previous command:

   ```console
   $ git add .
   $ git commit -S -m "release vX.Y.X"
   # or
   $ git commit -S -m "release vX.Y.X-rc.W"
   ```

4. Add release tag, correct format: `git tag vX.Y.Z` or `git tag vX.Y.Z-rc.W`
5. Push the generated commit and tag to the repostory branch.

   ```console
   $ git push origin vX.Y
   $ git push origin vX.Y.Z
   ```

## Other PRs to create

- Create krew PR for the plugin on https://github.com/kubernetes-sigs/krew-index to update the `datadog.yaml` artifact. (See [kubernetes-sigs/krew-index#727](https://github.com/kubernetes-sigs/krew-index/pull/727) as an example).
- Create PRs on https://github.com/operator-framework/community-operators for `community` and `upstream` operator.