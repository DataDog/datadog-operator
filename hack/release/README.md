# Release process

## Steps

- Checkout the repository on the correct branch and changeset (`master`)
- Run localy the command `make VERSION=x.v.z pre-release`
- Commit all the changes generated from the previous command:
    ```console
    $ git add .
    $ git commit -s -m "release vX.Y.X"
    ```
- Uncomment the job `push_latest_to_docker_hub` in `.gitlab-ci.yml` in case you plan to push the image to dockerhub with the `latest` image tag.
- Add release tag, correct format: `git tag vX.Y.Z`
- Push the generated commit and tag to the repostory branch.
    ```console
    $ git push origin vX.Y.Z
    ```

## Other PRs to create

- Create krew PR for the plugin on https://github.com/kubernetes-sigs/krew-index to update the `datadog-plugin.yaml` artifact.
- Create PRs on https://github.com/operator-framework/community-operators for `community` and `upstream` operator.
