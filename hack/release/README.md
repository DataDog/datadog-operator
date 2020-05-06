# Release process

## Steps

- checkout the repository on the correct branch and changeset
- Run localy the `make VERSION=x.v.z pre-release`
- Commit all the changes generated from the previous command:
    ```console
    $ git add .
    $ git commit -s -m "release vX.Y.X"
    ```
- Add release tag, correct format: `git tag vX.Y.Z`
- Push the generated commit and tag to the repostory branch.
    ```console
    $ git push origin vX.Y.Z
    ```

## Other PRs to create

- Create krew PR for the plugin on https://github.com/kubernetes-sigs/krew-index to update the `datadog-plugin.yaml` artifact.
- Create PRs on https://github.com/operator-framework/community-operators for `community` and `upstream` operator.
