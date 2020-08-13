# Release process

## Steps

1. Checkout the repository on the correct branch and changeset (`master`)
2. Run pre-release script

    1. Release candidate

       In case of release candidate, run localy commmand `make VERSION=x.v.z RELEASE_CANDIDATE=1 pre-release`

       For example `make VERSION=0.3.0 RELEASE_CANDIDATE=2 pre-release` will generate the release version `0.3.0-rc.2`

    2. Final release

       Run localy the command `make VERSION=x.v.z pre-release`

3. Commit all the changes generated from the previous command:

   ```console
   $ git add .
   $ git commit -s -m "release vX.Y.X"
   # or
   $ git commit -s -m "release vX.Y.X-rc.W"
   ```

4. Uncomment the job `push_latest_to_docker_hub` in `.gitlab-ci.yml` in case you plan to push the image to dockerhub with the `latest` image tag.

5. Add release tag, correct format: `git tag vX.Y.Z` or `git tag vX.Y.Z-rc.W`
6. Push the generated commit and tag to the repostory branch.

   ```console
   $ git push origin vX.Y.Z
   ```

## Other PRs to create

- Create krew PR for the plugin on https://github.com/kubernetes-sigs/krew-index to update the `datadog.yaml` artifact. (See [kubernetes-sigs/krew-index#727](https://github.com/kubernetes-sigs/krew-index/pull/727) as an example)
- Create PRs on https://github.com/operator-framework/community-operators for `community` and `upstream` operator.
