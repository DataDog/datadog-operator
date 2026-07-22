# OpenKruise in-place update and image pre-pull experiment

Run on 2026-07-22 against `kind-zero-gap-agent-rollout` with Kubernetes
v1.36.1 and OpenKruise v1.9.1. The workload uses one HTTP server and a
separate observer issuing numbered requests approximately every 100 ms. An
outage is reported as the interval between the last successful request before
an update and the first successful request after it. A failed request takes up
to one second because of the observer's timeout.

This is a behavior experiment, not a performance benchmark. The images are
small and each timing case was run once.

## Result

`InPlaceIfPossible` preserved the Pod for image-only changes, but it did not
overlap old and new containers. The old container was stopped before a
replacement could be pulled and started.

| Case | Pod identity | ImagePullJob | Observed result |
|---|---|---|---|
| Nonexistent image, no gate | UID stayed `48b32d66-337b-4945-832b-834e207757d9` | None | Old container exited and the Pod remained in `ImagePullBackOff` until manual rollback |
| Uncached `python:3.13-alpine`, no gate | Same UID; container ID changed to `8ecb557e...` | None | Kubelet pull took 2.559 s; success-to-success traffic gap was 7.845 s |
| Pre-pulled `python:3.14-alpine`, explicit gate | Same UID; container ID changed to `09725b13...` | `desired=1`, `succeeded=1`, `failed=0`, `active=0` before mutation | Image was already present; success-to-success traffic gap was 4.511 s |
| Nonexistent image, explicit gate | Pod UID, container ID `09725b13...`, image, and restart count stayed unchanged | `desired=1`, `succeeded=0`, `failed=1`, `active=0` | DaemonSet was not mutated and traffic remained successful |
| Pre-pull succeeds, process fails | Same UID; container ID changed to `32266554...` | Alpine pre-pull succeeded | Existing Python command exited 127; Pod entered `CrashLoopBackOff` and traffic stayed down until rollback |
| Unsupported environment change | UID changed from `48b32d66-...` to `02b00063-...` | Not applicable | `InPlaceIfPossible` fell back to Pod recreation; traffic gap was 5.635 s |

The single, non-comparable runs observed gaps of 7.845 s and 4.511 s, a
3.333-second difference. The cached activation reported no kubelet pull, but
the runs used different tags and do not establish the delta as a causal effect
of pre-pulling. The cached run still had a container restart and readiness gap.

Exact primary-workload identities, in observation order:

```text
baseline UID:       48b32d66-337b-4945-832b-834e207757d9
baseline container: c23b14b140f52e68af62f8f35c4dd6842b988ab28928677af7d12ded7957c7a1
rollback container: f00d1aa7ef5ae397d8b97b6abff8f6e9d68d00b6fa47b16bc48fe474738b5310
ungated container:  8ecb557e38acd3ad2b12c49c87e4e42229c70bbe17a131a9d14fb3b0b05e8e6c
pre-pulled container: 09725b13397863d1fbfc1725ddd95e45629082ecc044c8d98aa5abcb0698084f
failed-start container: 3226655429bfb858dcbd5e1ce355fd54b9a91c6bf8bd1a06b7b876626e07999b
pre-recreation container: 5153a3b55ea7771f9a00844b66b36b866ab392799d2fdc45c7bc368e5307676c
replacement UID:    02b00063-124a-4120-a241-9e46997cdc35
replacement container: 5c3cb507bcc83554a76cd210b25719767c67f2e6b4022463253bf590989194cd
```

## Evidence

Healthy baseline:

```text
inplace-demo-l4mj7  48b32d66-337b-4945-832b-834e207757d9  true  0
containerd://c23b14b1...  python:3.12-alpine
```

For the ungated missing image, the update was submitted at `15:17:21Z`.
Kubelet reported that the old container finished at `15:17:23Z`, followed by
`ImagePullBackOff`. Observer transitions were:

```text
2026-07-22T15:17:21.134632429Z 977 OK
2026-07-22T15:17:22.236060275Z 978 FAIL
2026-07-22T15:17:50.992993345Z 1005 OK  # only after rollback
```

The uncached valid update was submitted at `15:18:32Z`. The kubelet event was:

```text
Successfully pulled image "python:3.13-alpine" in 2.559s
```

Its request boundary was:

```text
2026-07-22T15:18:33.409066694Z 1411 OK
2026-07-22T15:18:34.532342554Z 1412 FAIL
2026-07-22T15:18:41.253995616Z 1419 OK
```

The standalone successful gate ran before the DaemonSet mutation:

```text
startTime:      2026-07-22T15:19:22Z
completionTime: 2026-07-22T15:19:27Z
desired: 1
active: 0
succeeded: 1
failed: 0
```

After activation, kubelet reported the image was already present. Its request
boundary was:

```text
2026-07-22T15:19:44.684584633Z 2036 OK
2026-07-22T15:19:45.786420014Z 2037 FAIL
2026-07-22T15:19:49.196018154Z 2041 OK
```

The standalone failed gate completed in five seconds:

```text
startTime:      2026-07-22T15:20:27Z
completionTime: 2026-07-22T15:20:32Z
desired: 1
active: 0
succeeded: 0
failed: 1
failedNodes: [zero-gap-agent-rollout-worker]
```

Because the experiment did not patch the DaemonSet after that result, the Pod
remained Ready with the same UID, container ID, restart count, and image. The
next twenty observer requests were all successful.

The activation-failure job successfully cached `alpine:3.22` at `15:21:00Z`.
After activation, the unchanged `python -m http.server` command exited 127 and
the first failed request followed the last success:

```text
2026-07-22T15:21:19.211459847Z 2922 OK
2026-07-22T15:21:20.313133586Z 2923 FAIL
```

Adding an environment variable is not an eligible in-place image change. The
old Pod was deleted and the replacement became ready with a new UID. The
request boundary was:

```text
2026-07-22T15:22:10.048879229Z 3087 OK
2026-07-22T15:22:11.150162924Z 3088 FAIL
2026-07-22T15:22:15.683683204Z 3093 OK
```

## Built-in AdvancedDaemonSet pre-download

OpenKruise v1.9.1 ships `PreDownloadImageForDaemonSetUpdate` as an alpha
feature gate that defaults to false. Merely enabling `ImagePullJobGate` runs
standalone jobs but does not enable automatic AdvancedDaemonSet pre-download.

The automatic code also skips pre-download when all Pods can update in one
batch. A separate two-worker DaemonSet with `maxUnavailable: 1` was therefore
used. After temporarily starting the managers with:

```text
--feature-gates=ImagePullJobGate=true,PreDownloadImageForDaemonSetUpdate=true
```

an update to a nonexistent image produced the owned job
`automatic-predownload-demo-544cd9c95d-server` at `15:26:32Z`. One second
later the job was active and both Pods were still Ready. The controller did not
wait for it: the selected Pod's old container exited at `15:26:34Z`. At
`15:26:41Z` the job had `active=1`, `failed=1`, while that Pod was already in
`ImagePullBackOff`. The other Pod stayed healthy only because
`maxUnavailable: 1` stopped the rollout globally; the per-node availability
invariant was violated on the updated node.

The feature flag was restored to its original value after the test:

```text
--feature-gates=ImagePullJobGate=true
```

## Reproduction

Prerequisites are the `kind-zero-gap-agent-rollout` context, two schedulable
workers named `zero-gap-agent-rollout-worker` and
`zero-gap-agent-rollout-worker2`, OpenKruise v1.9.1 with its manager and node
daemon healthy, and registry access for the fixture images. The checked-in
selectors intentionally depend on those worker names.

```sh
# Reset only the isolated experiment namespace.
kubectl --context kind-zero-gap-agent-rollout delete namespace \
  openkruise-prepull-lab --ignore-not-found --wait=true
kubectl --context kind-zero-gap-agent-rollout apply \
  -f experiments/openkruise-prepull/base.yaml
kubectl --context kind-zero-gap-agent-rollout \
  -n openkruise-prepull-lab wait --for=condition=Ready pod \
  -l app=inplace-demo --timeout=180s

# Missing image without a gate; inspect ImagePullBackOff, then roll back.
kubectl --context kind-zero-gap-agent-rollout \
  -n openkruise-prepull-lab patch daemonset.apps.kruise.io inplace-demo \
  --type=json \
  -p='[{"op":"replace","path":"/spec/template/spec/containers/0/image","value":"python:zero-gap-tag-does-not-exist"}]'
kubectl --context kind-zero-gap-agent-rollout \
  -n openkruise-prepull-lab wait \
  --for=jsonpath='{.status.containerStatuses[0].state.waiting.reason}'=ImagePullBackOff \
  pod -l app=inplace-demo --timeout=180s
kubectl --context kind-zero-gap-agent-rollout \
  -n openkruise-prepull-lab get pod -l app=inplace-demo -o wide
kubectl --context kind-zero-gap-agent-rollout \
  -n openkruise-prepull-lab patch daemonset.apps.kruise.io inplace-demo \
  --type=json \
  -p='[{"op":"replace","path":"/spec/template/spec/containers/0/image","value":"python:3.12-alpine"}]'
kubectl --context kind-zero-gap-agent-rollout \
  -n openkruise-prepull-lab wait \
  --for=jsonpath='{.status.containerStatuses[0].image}'=docker.io/library/python:3.12-alpine \
  pod -l app=inplace-demo --timeout=180s
kubectl --context kind-zero-gap-agent-rollout \
  -n openkruise-prepull-lab wait --for=condition=Ready pod \
  -l app=inplace-demo --timeout=180s

# Ungated image-only update
kubectl --context kind-zero-gap-agent-rollout \
  -n openkruise-prepull-lab patch daemonset.apps.kruise.io inplace-demo \
  --type=json \
  -p='[{"op":"replace","path":"/spec/template/spec/containers/0/image","value":"python:3.13-alpine"}]'
kubectl --context kind-zero-gap-agent-rollout \
  -n openkruise-prepull-lab wait \
  --for=jsonpath='{.status.containerStatuses[0].image}'=docker.io/library/python:3.13-alpine \
  pod -l app=inplace-demo --timeout=180s
kubectl --context kind-zero-gap-agent-rollout \
  -n openkruise-prepull-lab wait --for=condition=Ready pod \
  -l app=inplace-demo --timeout=180s

# Explicit pre-pull gate
kubectl --context kind-zero-gap-agent-rollout apply \
  -f experiments/openkruise-prepull/imagepulljob-good.yaml
kubectl --context kind-zero-gap-agent-rollout \
  -n openkruise-prepull-lab get imagepulljob python-3-14-alpine -o yaml
kubectl --context kind-zero-gap-agent-rollout \
  -n openkruise-prepull-lab wait \
  --for=jsonpath='{.status.completionTime}' imagepulljob/python-3-14-alpine \
  --timeout=360s
test "$(kubectl --context kind-zero-gap-agent-rollout \
  -n openkruise-prepull-lab get imagepulljob python-3-14-alpine \
  -o jsonpath='{.status.desired},{.status.succeeded},{.status.failed},{.status.active}')" \
  = "1,1,0,0"

# Mutate the DaemonSet only after desired > 0, succeeded == desired,
# failed == 0, active == 0, and completionTime is set.
kubectl --context kind-zero-gap-agent-rollout \
  -n openkruise-prepull-lab patch daemonset.apps.kruise.io inplace-demo \
  --type=json \
  -p='[{"op":"replace","path":"/spec/template/spec/containers/0/image","value":"python:3.14-alpine"}]'
kubectl --context kind-zero-gap-agent-rollout \
  -n openkruise-prepull-lab wait \
  --for=jsonpath='{.status.containerStatuses[0].image}'=docker.io/library/python:3.14-alpine \
  pod -l app=inplace-demo --timeout=180s
kubectl --context kind-zero-gap-agent-rollout \
  -n openkruise-prepull-lab wait --for=condition=Ready pod \
  -l app=inplace-demo --timeout=180s

# Failed gate: inspect the result and deliberately do not patch the DaemonSet.
kubectl --context kind-zero-gap-agent-rollout apply \
  -f experiments/openkruise-prepull/imagepulljob-bad.yaml
kubectl --context kind-zero-gap-agent-rollout \
  -n openkruise-prepull-lab get imagepulljob python-missing -o yaml
kubectl --context kind-zero-gap-agent-rollout \
  -n openkruise-prepull-lab wait \
  --for=jsonpath='{.status.completionTime}' imagepulljob/python-missing \
  --timeout=120s
test "$(kubectl --context kind-zero-gap-agent-rollout \
  -n openkruise-prepull-lab get imagepulljob python-missing \
  -o jsonpath='{.status.desired},{.status.succeeded},{.status.failed},{.status.active}')" \
  = "1,0,1,0"

# Successfully cached image with an invalid retained command.
kubectl --context kind-zero-gap-agent-rollout apply \
  -f experiments/openkruise-prepull/imagepulljob-activation-failure.yaml
kubectl --context kind-zero-gap-agent-rollout \
  -n openkruise-prepull-lab wait \
  --for=jsonpath='{.status.completionTime}' imagepulljob/alpine-3-22 \
  --timeout=360s
test "$(kubectl --context kind-zero-gap-agent-rollout \
  -n openkruise-prepull-lab get imagepulljob alpine-3-22 \
  -o jsonpath='{.status.desired},{.status.succeeded},{.status.failed},{.status.active}')" \
  = "1,1,0,0"
kubectl --context kind-zero-gap-agent-rollout \
  -n openkruise-prepull-lab patch daemonset.apps.kruise.io inplace-demo \
  --type=json \
  -p='[{"op":"replace","path":"/spec/template/spec/containers/0/image","value":"alpine:3.22"}]'
kubectl --context kind-zero-gap-agent-rollout \
  -n openkruise-prepull-lab wait \
  --for=jsonpath='{.status.containerStatuses[0].state.waiting.reason}'=CrashLoopBackOff \
  pod -l app=inplace-demo --timeout=180s
kubectl --context kind-zero-gap-agent-rollout \
  -n openkruise-prepull-lab patch daemonset.apps.kruise.io inplace-demo \
  --type=json \
  -p='[{"op":"replace","path":"/spec/template/spec/containers/0/image","value":"python:3.14-alpine"}]'
kubectl --context kind-zero-gap-agent-rollout \
  -n openkruise-prepull-lab wait \
  --for=jsonpath='{.status.containerStatuses[0].image}'=docker.io/library/python:3.14-alpine \
  pod -l app=inplace-demo --timeout=180s
kubectl --context kind-zero-gap-agent-rollout \
  -n openkruise-prepull-lab wait --for=condition=Ready pod \
  -l app=inplace-demo --timeout=180s

# Unsupported Pod-template change. Record and require a new Pod UID.
inplace_uid=$(kubectl --context kind-zero-gap-agent-rollout \
  -n openkruise-prepull-lab get pod -l app=inplace-demo \
  -o jsonpath='{.items[0].metadata.uid}')
kubectl --context kind-zero-gap-agent-rollout \
  -n openkruise-prepull-lab patch daemonset.apps.kruise.io inplace-demo \
  --type=json \
  -p='[{"op":"add","path":"/spec/template/spec/containers/0/env","value":[{"name":"UNSUPPORTED_TEMPLATE_CHANGE","value":"true"}]}]'
while test "$(kubectl --context kind-zero-gap-agent-rollout \
  -n openkruise-prepull-lab get pod -l app=inplace-demo \
  -o jsonpath='{.items[0].metadata.uid}' 2>/dev/null)" = "$inplace_uid"; do sleep 1; done
kubectl --context kind-zero-gap-agent-rollout \
  -n openkruise-prepull-lab wait --for=condition=Ready pod \
  -l app=inplace-demo --timeout=180s

# Two-worker automatic pre-download fixture.
kubectl --context kind-zero-gap-agent-rollout apply \
  -f experiments/openkruise-prepull/automatic-predownload.yaml
# Do not continue until both worker Pods are Ready.
while test "$(kubectl --context kind-zero-gap-agent-rollout \
  -n openkruise-prepull-lab get daemonset.apps.kruise.io \
  automatic-predownload-demo -o jsonpath='{.status.numberReady}')" != "2"; do sleep 1; done
# Verify the current argument before using the observed index from this install.
kubectl --context kind-zero-gap-agent-rollout -n kruise-system get deployment \
  kruise-controller-manager \
  -o jsonpath='{.spec.template.spec.containers[0].args[6]}{"\n"}'
# Abort unless the preceding command prints exactly:
# --feature-gates=ImagePullJobGate=true
kubectl --context kind-zero-gap-agent-rollout -n kruise-system patch deployment \
  kruise-controller-manager --type=json \
  -p='[{"op":"replace","path":"/spec/template/spec/containers/0/args/6","value":"--feature-gates=ImagePullJobGate=true,PreDownloadImageForDaemonSetUpdate=true"}]'
kubectl --context kind-zero-gap-agent-rollout -n kruise-system rollout status \
  deployment/kruise-controller-manager --timeout=180s
kubectl --context kind-zero-gap-agent-rollout -n kruise-system get deployment \
  kruise-controller-manager \
  -o jsonpath='{.spec.template.spec.containers[0].args[6]}{"\n"}'
kubectl --context kind-zero-gap-agent-rollout \
  -n openkruise-prepull-lab patch daemonset.apps.kruise.io \
  automatic-predownload-demo --type=json \
  -p='[{"op":"replace","path":"/spec/template/spec/containers/0/image","value":"python:zero-gap-auto-gate-does-not-exist"}]'
kubectl --context kind-zero-gap-agent-rollout \
  -n openkruise-prepull-lab get imagepulljobs.apps.kruise.io
kubectl --context kind-zero-gap-agent-rollout \
  -n openkruise-prepull-lab get pods -l app=automatic-predownload-demo -o wide

# Observer transition extraction used for each timing boundary.
kubectl --context kind-zero-gap-agent-rollout \
  -n openkruise-prepull-lab logs observer --timestamps \
  --since-time=2026-07-22T15:19:43Z \
  | awk '/ (OK|FAIL)$/ {state=$NF; if (state != previous) {print; previous=state}}'

# Restore the two-worker fixture and original manager feature flags.
kubectl --context kind-zero-gap-agent-rollout apply \
  -f experiments/openkruise-prepull/automatic-predownload.yaml
while test "$(kubectl --context kind-zero-gap-agent-rollout \
  -n openkruise-prepull-lab get daemonset.apps.kruise.io \
  automatic-predownload-demo -o jsonpath='{.status.numberReady}')" != "2"; do sleep 1; done
kubectl --context kind-zero-gap-agent-rollout -n kruise-system patch deployment \
  kruise-controller-manager --type=json \
  -p='[{"op":"replace","path":"/spec/template/spec/containers/0/args/6","value":"--feature-gates=ImagePullJobGate=true"}]'
kubectl --context kind-zero-gap-agent-rollout -n kruise-system rollout status \
  deployment/kruise-controller-manager --timeout=180s
kubectl --context kind-zero-gap-agent-rollout apply \
  -f experiments/openkruise-prepull/base.yaml
kubectl --context kind-zero-gap-agent-rollout \
  -n openkruise-prepull-lab wait \
  --for=jsonpath='{.status.containerStatuses[0].image}'=docker.io/library/python:3.12-alpine \
  pod -l app=inplace-demo --timeout=180s
kubectl --context kind-zero-gap-agent-rollout \
  -n openkruise-prepull-lab get pods -o wide
kubectl --context kind-zero-gap-agent-rollout -n kruise-system get deployment \
  kruise-controller-manager \
  -o jsonpath='{.spec.template.spec.containers[0].args[6]}{"\n"}'
```

For a production gate, use an immutable digest with `IfNotPresent`. A mutable
tag or `Always` can still require registry resolution during activation. Image
garbage collection or a node restart between gate completion and activation
can also evict the cached digest, so this is not a permanent pull guarantee.

## Conclusion

Image pre-pulling is worthwhile preparation. The built-in automatic mechanism
is not an availability gate, while a standalone Operator-managed ImagePullJob
can gate DaemonSet mutation on a successful pull while that cached image
remains available. Neither form validates process startup, and in-place update
still has a restart gap. It is therefore a complementary optimization, not the
zero-gap primitive.
