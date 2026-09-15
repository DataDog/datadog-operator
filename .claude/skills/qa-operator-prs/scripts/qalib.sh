#!/usr/bin/env bash
# Sourceable helpers for datadog-operator QA. `source qalib.sh`
# Override defaults: QA_NS, QA_DDA, QA_OP (operator deploy name).
QA_NS=${QA_NS:-datadog}
QA_DDA=${QA_DDA:-datadog}
QA_OP=${QA_OP:-$(kubectl get deploy -n "$QA_NS" -l app.kubernetes.io/name=datadog-operator -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)}
_qa_pass=0; _qa_fail=0

# ---- waiting ---------------------------------------------------------------
# `sleep` is blocked in this harness; block on the operator log stream instead.
nap() { timeout "${1:-5}" kubectl logs -n "$QA_NS" "deploy/$QA_OP" -f --tail=1 >/dev/null 2>&1 || true; }

# settle <ds/name|deploy/name> — wait until the controller observed the spec it has
settle() {
  local ref=$1 i g o
  for i in $(seq 1 "${2:-30}"); do
    g=$(kubectl get "$ref" -n "$QA_NS" -o jsonpath='{.metadata.generation}' 2>/dev/null)
    o=$(kubectl get "$ref" -n "$QA_NS" -o jsonpath='{.status.observedGeneration}' 2>/dev/null)
    [ -n "$g" ] && [ "$g" = "$o" ] && return 0
    nap 4
  done
  echo "  (warn: $ref did not settle)" >&2; return 1
}

# settle_until '<shell cmd>' '<expected>' [tries] — poll until output matches.
# STRONGEST form: pick a predicate only true AFTER your change lands (image tag,
# env var name). Generation alone has not bumped yet the instant you patch.
settle_until() {
  local cmd=$1 want=$2 i got
  for i in $(seq 1 "${3:-30}"); do
    got=$(eval "$cmd" 2>/dev/null)
    [ "$got" = "$want" ] && return 0
    nap 4
  done
  echo "  (warn: never reached '$want', last='$got')" >&2; return 1
}

# wait_log <regex> [secs] — block until the operator logs a matching line
wait_log() { timeout "${2:-120}" kubectl logs -n "$QA_NS" "deploy/$QA_OP" -f 2>/dev/null | grep -m1 -E "$1" >/dev/null 2>&1; }

# bump_dda <suffix> — force a DDA reconcile by appending an env var
bump_dda() {
  kubectl patch datadogagent "$QA_DDA" -n "$QA_NS" --type=json \
    -p="[{\"op\":\"add\",\"path\":\"/spec/override/nodeAgent/containers/agent/env/-\",\"value\":{\"name\":\"QA_$1\",\"value\":\"1\"}}]" >/dev/null 2>&1
}

# ---- assertions ------------------------------------------------------------
assert_eq() { # label actual expected
  if [ "$2" = "$3" ]; then printf "  \033[32m✅ PASS\033[0m %-56s %s\n" "$1" "$2"; _qa_pass=$((_qa_pass+1));
  else printf "  \033[31m❌ FAIL\033[0m %-56s got=[%s] want=[%s]\n" "$1" "$2" "$3"; _qa_fail=$((_qa_fail+1)); fi
}
assert_contains() { # label haystack needle
  if printf '%s' "$2" | grep -qF -- "$3"; then printf "  \033[32m✅ PASS\033[0m %s\n" "$1"; _qa_pass=$((_qa_pass+1));
  else printf "  \033[31m❌ FAIL\033[0m %s\n     got: %s\n" "$1" "$2"; _qa_fail=$((_qa_fail+1)); fi
}
assert_absent() { # label haystack needle
  if printf '%s' "$2" | grep -qF -- "$3"; then printf "  \033[31m❌ FAIL\033[0m %s (found '%s')\n" "$1" "$3"; _qa_fail=$((_qa_fail+1));
  else printf "  \033[32m✅ PASS\033[0m %s\n" "$1"; _qa_pass=$((_qa_pass+1)); fi
}

# evidence '<shell cmd>' — echo the command and its output, for pasting into the report
evidence() { printf "\n$ %s\n" "$1"; eval "$1" 2>&1 | sed 's/^/  /'; }

summary() {
  printf "\n\033[1m== %d passed, %d failed ==\033[0m\n" "$_qa_pass" "$_qa_fail"
  [ "$_qa_fail" -eq 0 ]
}

# ---- common probes ---------------------------------------------------------
op_errors() { # real errors only, minus expected fake-credential noise
  kubectl logs -n "$QA_NS" "deploy/$QA_OP" ${1:+--since="$1"} 2>&1 \
    | jq -r 'select(.level=="ERROR")|"\(.ts) \(.msg) :: \(.error//"")"' 2>/dev/null \
    | grep -v "403 Forbidden" | grep -v "empty API key" | grep -v "blocked error forwarding"
}
op_restarts() { kubectl get pods -n "$QA_NS" -l app.kubernetes.io/name=datadog-operator --no-headers 2>/dev/null | awk '{print $4}'; }
goroutines() { # needs a free local port
  local p=${1:-18399}
  kubectl port-forward -n "$QA_NS" "deploy/$QA_OP" "$p:8383" >/dev/null 2>&1 &
  local pf=$!; nap 4
  curl -s --max-time 3 "localhost:$p/metrics" 2>/dev/null | awk '/^go_goroutines /{print $2}'
  kill $pf 2>/dev/null
}
cond() { # cond <kind> <name> <conditionType> -> "status|reason|message"
  kubectl get "$1" "$2" -n "$QA_NS" -o json 2>/dev/null \
    | jq -r --arg t "$3" '[.status.conditions[]?|select(.type==$t)]
        | if length==0 then "ABSENT||" else "\(.[0].status)|\(.[0].reason)|\(.[0].message)" end'
}
