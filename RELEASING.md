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
- Create PRs on https://github.com/redhat-openshift-ecosystem/community-operators-prod for `OpenShift Community` and https://github.com/k8s-operatorhub/community-operators for `Kubernetes OperatorHub`.

## Testing the generating bundle

### Using `operator-sdk`

Testing the delivery can be done locally by using `kind` and the generated (or published) Docker bundle:

```
# When using a local image
# kind load docker-image <image-name>
./bin/operator-sdk olm install # If not already installed in your cluster.
./bin/operator-sdk run bundle docker.io/datadog/operator-bundle:0.7.1
```

### Reproducing upstream CI

The Ansible playbook used to test operators in `Kubernetes OperatorHub` can easily be played on its own.
Documentation is available at: https://k8s-operatorhub.github.io/community-operators/operator-test-suite

To test the Operator itself, the `kiwi` test suite is the most useful one but you can replay any of them.

Here is a small how-to to run this from scratch on an empty Ubuntu VM:
```
sudo apt update && sudo apt install ansible docker.io
sudo usermod -a -G docker ubuntu
# Close and re-open connection
sudo sysctl net/netfilter/nf_conntrack_max=131072 # Fix startup issues in `kube-proxy`
git clone https://github.com/k8s-operatorhub/community-operators.git
cd community-operators
OPP_AUTO_PACKAGEMANIFEST_CLUSTER_VERSION_LABEL=1 OPP_PRODUCTION_TYPE=k8s bash <(curl -sL https://raw.githubusercontent.com/redhat-openshift-ecosystem/community-operators-pipeline/ci/latest/ci/scripts/opp.sh) <suite_name, ex: kiwi> <operator_path, ex: operators/datadog-operator/0.7.1>
```
