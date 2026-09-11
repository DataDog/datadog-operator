#!/usr/bin/env bash

# Exit on error, undefined variable, and pipe failure
set -o errexit
set -o nounset
set -o pipefail

SCRIPTS_DIR="$(dirname "$0")"
source "$SCRIPTS_DIR/os-env.sh"

api_go_mod="$ROOT/api/go.mod"
api_go_mod_backup=""
api_go_sum="$ROOT/api/go.sum"
api_go_sum_backup=""

restore_api_module() {
    if [[ -n $api_go_mod_backup && -f $api_go_mod_backup ]]; then
        cp "$api_go_mod_backup" "$api_go_mod"
        unlink "$api_go_mod_backup"
    fi
    if [[ -n $api_go_sum_backup && -f $api_go_sum_backup ]]; then
        cp "$api_go_sum_backup" "$api_go_sum"
        unlink "$api_go_sum_backup"
    fi
}
trap restore_api_module EXIT

if [[ -f $api_go_mod ]]; then
    # The API module is consumed independently by projects that may use an
    # older Kubernetes dependency set. Preserve its requirements because
    # go work sync would otherwise promote them to the versions selected by
    # the root module.
    api_go_mod_backup=$(mktemp)
    cp "$api_go_mod" "$api_go_mod_backup"
fi
if [[ -f $api_go_sum ]]; then
    api_go_sum_backup=$(mktemp)
    cp "$api_go_sum" "$api_go_sum_backup"
fi

(cd "$ROOT" && go work sync)
