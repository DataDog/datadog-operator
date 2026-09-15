---
name: qa-operator-prs
description: Use when asked to QA, test, or verify a Datadog Operator PR or release candidate on a local cluster — including "QA this PR", "test this on kind", or when handed a datadog-operator PR link and asked whether the feature works.
---

# QA Datadog Operator PRs

Manual QA on a local kind cluster against the RC operator image.

**Core principle:** every claim needs a command, its real output, and a control that proves the behavior came from the feature and not from a default. "It looks right" is not a result.

## 0. First: is the PR even in the image you're about to run?

```bash
SHA=$(gh pr view <N> --repo DataDog/datadog-operator --json mergeCommit -q .mergeCommit.oid)
git fetch origin --tags -q && git tag --contains "$SHA" | head -3      # expect vX.Y.Z-rc.N
```

No RC tag → the `--devel` image does not contain the change. Stop and say so.

For CRD changes also confirm the shipped chart matches the merged CRD (they drift):

```bash
diff <(kubectl get crd <name> -o json | jq -S '.spec.versions[0].schema.openAPIV3Schema') \
     <(git show "$SHA:config/crd/bases/v1/<file>.yaml" | python3 -c 'import sys,yaml,json;print(json.dumps(yaml.safe_load(sys.stdin)["spec"]["versions"][0]["schema"]["openAPIV3Schema"],sort_keys=True,indent=1))')
```

## 1. Setup

One command, idempotent, reuses an existing cluster:

```bash
.claude/skills/qa-operator-prs/scripts/setup.sh            # cluster qa, ns datadog
.claude/skills/qa-operator-prs/scripts/setup.sh --name foo --k8s v1.29.14   # pin k8s for version gates
```

The raw sequence it runs (use directly if you need to vary it):

```bash
kind create cluster --name qa --wait 120s
helm repo add datadog https://helm.datadoghq.com && helm repo update datadog
helm install ddop datadog/datadog-operator --devel -n datadog --create-namespace --wait
kubectl create secret generic datadog-secret -n datadog \
  --from-literal=api-key=00000000000000000000000000000000 \
  --from-literal=app-key=0000000000000000000000000000000000000000
```

Then apply a **DatadogAgent** (never a DatadogAgentInternal — see gotchas):

```yaml
apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
metadata: {name: datadog, namespace: datadog}
spec:
  global:
    credentials:
      apiSecret: {secretName: datadog-secret, keyName: api-key}
  override:
    nodeAgent:
      env:                                    # REQUIRED on kind or the agent crashloops
        - name: DD_HOSTNAME
          valueFrom: {fieldRef: {fieldPath: spec.nodeName}}
```

Fake keys are fine. Expect `Unable to get credentials` at startup and recurring `403 Forbidden` from the metrics forwarder — both are noise, not findings.

## 2. Derive assertions from the diff, not the PR body

Read the actual change and list what must be observably true. **Verify every key the test plan names actually exists in code** — grep the literal:

```bash
grep -rn '"the.annotation/key"' --include="*.go" .     # 0 hits = the test plan is wrong
```

PR test plans have named annotations that do not exist. Annotations are unvalidated, so a wrong key fails silently and looks like a broken feature.

## 3. Settle before you read

The single largest source of wrong conclusions. A read taken before the operator reconciles reports the *old* object.

```bash
source .claude/skills/qa-operator-prs/scripts/qalib.sh
settle ds/datadog-agent                       # generation == observedGeneration
settle_until 'kubectl get ds datadog-agent -n datadog -o json | jq -r .spec.template.spec.containers[0].image' \
             'gcr.io/datadoghq/agent:7.81.1'  # value-specific predicate — strongest form
```

Pick a predicate that can only be true after the change lands (an image registry, an env var name). Generation alone is not enough: it has not bumped yet at the instant you patch.

`sleep` is blocked in this harness. To block on an event use:

```bash
timeout 120 kubectl logs -n datadog deploy/ddop-datadog-operator -f 2>/dev/null | grep -m1 "Starting workers"
```

## 4. Always run a control

| Result shape | Control that makes it mean something |
|---|---|
| Feature ON produces X | Run with feature OFF, show X absent (attribution) |
| "Nothing was created" / bug is gone | Positive case with real data — proves the operator processed the field at all, not that it ignored it |
| Version-gated behavior | Second cluster on the other side of the gate (`--k8s v1.29.14`) |

Without the negative case you cannot distinguish "feature works" from "default already did that".

## 5. Report: commands + outputs

Give the user evidence they can re-run, not prose. Required shape:

```
## <what was verified>
$ <exact command>
<real output, trimmed to the relevant lines>

| assertion | result |
|---|---|
| system-probe has appArmorProfile.type=Unconfined | ✅ Unconfined |
| deprecated annotation removed | ✅ {} |
```

- Lead with the assertion table; put raw output under it.
- State which axes you did **not** test and why.
- Separate *the PR is broken* from *the environment tripped* — label incidental findings clearly.
- Package repeatable checks as a script and run it before handing it over (`assert_eq`/`summary` in `qalib.sh` print a PASS/FAIL tally).

## Gotchas

Read `gotchas.md` in this directory before debugging anything weird — every entry cost real time.

Highest-frequency four:

| Symptom | Fix |
|---|---|
| `helm install --devel` fails `EOF` fetching the tgz | `helm pull datadog/datadog-operator --devel --untar --untardir /tmp/c` then install from `/tmp/c/datadog-operator` |
| `zsh: no matches found: env[0].name=...` | Quote it: `--set 'env[0].name=X'` |
| Agent crashloops, `unable to reliably determine the host name` | `DD_HOSTNAME` from `spec.nodeName` (above) |
| Operator restarts repeatedly, liveness 500, `too many goroutines: 402 > limit: 400` | `--set maximumGoroutines=1000` (DAPs push steady state to ~405) |
