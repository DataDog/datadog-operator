#!/usr/bin/env bash
# Idempotent QA environment for datadog-operator: kind cluster + RC operator + DDA.
# Safe to re-run; reuses an existing cluster and helm release.
#
#   setup.sh                              # cluster "qa", ns "datadog", latest --devel RC
#   setup.sh --name ap129 --k8s v1.29.14  # pin k8s (version-gated features)
#   setup.sh --dap                        # also enable DatadogAgentProfiles (+ raise goroutine ceiling)
#   setup.sh --no-dda                     # cluster + operator only
set -euo pipefail

NAME=qa; NS=datadog; K8S=""; DAP=0; DDA=1; GOROUTINES=1000
while [ $# -gt 0 ]; do
  case "$1" in
    --name) NAME=$2; shift 2;;
    --ns) NS=$2; shift 2;;
    --k8s) K8S=$2; shift 2;;
    --dap) DAP=1; shift;;
    --no-dda) DDA=0; shift;;
    --goroutines) GOROUTINES=$2; shift 2;;
    -h|--help) sed -n '2,10p' "$0"; exit 0;;
    *) echo "unknown arg: $1" >&2; exit 2;;
  esac
done
CTX="kind-$NAME"
k() { kubectl --context "$CTX" "$@"; }
say() { printf "\n\033[1m==> %s\033[0m\n" "$1"; }

say "kind cluster: $NAME"
if kind get clusters 2>/dev/null | grep -qx "$NAME"; then
  echo "  exists, reusing"
else
  kind create cluster --name "$NAME" ${K8S:+--image "kindest/node:$K8S"} --wait 180s
fi
k version -o json | jq -r '"  server: " + .serverVersion.gitVersion'

say "operator chart (--devel RC)"
helm repo add datadog https://helm.datadoghq.com >/dev/null 2>&1 || true
helm repo update datadog >/dev/null
# `helm install --devel` intermittently fails with EOF fetching the tgz from GitHub;
# pulling first is reliable and lets us verify the version we actually got.
CHART=/tmp/ddop-chart-$NAME
if [ ! -d "$CHART/datadog-operator" ]; then
  rm -rf "$CHART"; helm pull datadog/datadog-operator --devel --untar --untardir "$CHART"
fi
grep -E '^(version|appVersion)' "$CHART/datadog-operator/Chart.yaml" | sed 's/^/  /'

SETS=(--set "maximumGoroutines=$GOROUTINES")
[ "$DAP" = 1 ] && SETS+=(--set datadogAgentProfile.enabled=true --set datadogCRDs.crds.datadogAgentProfiles=true)
if helm --kube-context "$CTX" status ddop -n "$NS" >/dev/null 2>&1; then
  helm --kube-context "$CTX" upgrade ddop "$CHART/datadog-operator" -n "$NS" --reuse-values "${SETS[@]}" --wait --timeout 6m >/dev/null
  echo "  upgraded existing release"
else
  helm --kube-context "$CTX" install ddop "$CHART/datadog-operator" -n "$NS" --create-namespace "${SETS[@]}" --wait --timeout 6m >/dev/null
  echo "  installed"
fi
k get deploy -n "$NS" -l app.kubernetes.io/name=datadog-operator \
  -o jsonpath='{.items[0].spec.template.spec.containers[0].image}' | sed 's/^/  image: /'; echo

say "credentials secret (fake keys are fine)"
k create secret generic datadog-secret -n "$NS" \
  --from-literal=api-key=00000000000000000000000000000000 \
  --from-literal=app-key=0000000000000000000000000000000000000000 \
  --dry-run=client -o yaml | k apply -f - >/dev/null
echo "  datadog-secret ready"

if [ "$DDA" = 1 ]; then
  say "DatadogAgent"
  # DD_HOSTNAME from spec.nodeName is REQUIRED on kind: no cloud metadata + non-FQDN
  # hostname makes the core agent exit with "unable to reliably determine the host name".
  cat <<EOF | k apply -f - >/dev/null
apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
metadata:
  name: datadog
  namespace: $NS
spec:
  global:
    clusterName: $NAME
    credentials:
      apiSecret: {secretName: datadog-secret, keyName: api-key}
  override:
    nodeAgent:
      env:
        - name: DD_HOSTNAME
          valueFrom: {fieldRef: {fieldPath: spec.nodeName}}
      containers:
        agent:
          env:
            - name: QA_SEED          # stable path for JSON-patch spec bumps
              value: "0"
EOF
  # The operator creates the DaemonSet asynchronously; `rollout status` run immediately
  # fails with NotFound and (with `|| true`) would silently skip verifying the agent.
  OPDEP=$(k get deploy -n "$NS" -l app.kubernetes.io/name=datadog-operator -o jsonpath='{.items[0].metadata.name}')
  for _ in $(seq 1 30); do
    k get ds datadog-agent -n "$NS" >/dev/null 2>&1 && break
    timeout 5 k logs -n "$NS" "deploy/$OPDEP" -f --tail=1 >/dev/null 2>&1 || true
  done
  if k get ds datadog-agent -n "$NS" >/dev/null 2>&1; then
    k rollout status ds/datadog-agent -n "$NS" --timeout=420s || echo "  (agent did not become ready — check 'kubectl logs -c agent')"
  else
    echo "  ERROR: operator never created the agent DaemonSet" >&2
  fi
fi

say "state"
k get pods -n "$NS" --no-headers | sed 's/^/  /'
k get datadogagent,datadogagentinternal -n "$NS" --no-headers 2>/dev/null | sed 's/^/  /' || true
cat <<EOF

Context: $CTX   Namespace: $NS
Next:  source $(dirname "$0")/qalib.sh
EOF
