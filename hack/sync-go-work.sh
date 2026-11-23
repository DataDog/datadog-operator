#!/usr/bin/env bash

# Exit on error, undefined variable, and pipe failure
set -o errexit
set -o nounset
set -o pipefail

SCRIPTS_DIR="$(dirname "$0")"
source "$SCRIPTS_DIR/os-env.sh"

api_go_mod="$ROOT/api/go.mod"
api_kubernetes_requirements=()
api_go_mod_backup=""
api_go_sum="$ROOT/api/go.sum"
api_go_sum_backup=""

cleanup() {
    status=$?
    trap - EXIT

    if [[ $status -ne 0 ]]; then
        if [[ -n $api_go_mod_backup && -f $api_go_mod_backup ]]; then
            cp "$api_go_mod_backup" "$api_go_mod"
        fi
        if [[ -n $api_go_sum_backup && -f $api_go_sum_backup ]]; then
            cp "$api_go_sum_backup" "$api_go_sum"
        fi
    fi

    if [[ -n $api_go_mod_backup && -f $api_go_mod_backup ]]; then
        unlink "$api_go_mod_backup"
    fi
    if [[ -n $api_go_sum_backup && -f $api_go_sum_backup ]]; then
        unlink "$api_go_sum_backup"
    fi

    exit "$status"
}
trap cleanup EXIT

if [[ -f $api_go_mod ]]; then
    # The API module is consumed independently by projects that may use an
    # older Kubernetes dependency set. Preserve only these requirements because
    # go work sync would otherwise promote them to the versions selected by
    # the root module.
    while IFS= read -r requirement; do
        api_kubernetes_requirements+=("$requirement")
    done < <(awk '$1 ~ /^k8s\.io\// || $1 == "sigs.k8s.io/controller-runtime" { print $1 "@" $2 }' "$api_go_mod")

    api_go_mod_backup=$(mktemp)
    cp "$api_go_mod" "$api_go_mod_backup"
    if [[ -f $api_go_sum ]]; then
        api_go_sum_backup=$(mktemp)
        cp "$api_go_sum" "$api_go_sum_backup"
    fi
fi

(cd "$ROOT" && go work sync)

if [[ ${#api_kubernetes_requirements[@]} -gt 0 ]]; then
    for requirement in "${api_kubernetes_requirements[@]}"; do
        go mod edit -require="$requirement" "$api_go_mod"
    done
    (cd "$ROOT/api" && GOWORK=off go mod tidy)
fi
