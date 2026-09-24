#!/usr/bin/env bash
# Run the core CI checks in one pod, reusing its tools and Go working directories.
set -euo pipefail

make_command=${1:-make}
active_section=""

close_section() {
    if [[ ${GITLAB_CI:-} == "true" ]]; then
        printf '\033[0Ksection_end:%s:%s\r\033[0K\n' "$(date +%s)" "$active_section"
    fi
}

on_exit() {
    status=$?
    if [[ -n $active_section ]]; then
        close_section
        echo "CI check failed: $active_section (exit $status)" >&2
    fi
    exit "$status"
}
trap on_exit EXIT

run_check() {
    active_section=$1
    title=$2
    shift 2
    if [[ ${GITLAB_CI:-} == "true" ]]; then
        printf '\033[0Ksection_start:%s:%s[collapsed=true]\r\033[0K%s\n' "$(date +%s)" "$active_section" "$title"
    else
        printf '\n==> %s\n' "$title"
    fi
    "$@"
    close_section
    active_section=""
}

check_generated() {
    "$make_command" --no-print-directory "$@"
    # Check immediately: a later formatter/generator must not hide this diff.
    git diff --exit-code
}

run_check build_binaries "Build binaries" "$make_command" --no-print-directory ci-build
run_check check_golang_version "Check Go versions and module files" check_generated update-golang
run_check verify_licenses "Verify licenses" "$make_command" --no-print-directory verify-licenses
run_check generate_code "Check generated code, manifests and documentation" check_generated generate
run_check check_formatting "Check formatting" check_generated fmt
run_check lint "Lint and vet" "$make_command" --no-print-directory lint
run_check unit_tests "Unit tests" "$make_command" --no-print-directory gotest
run_check integration_tests "Integration tests" "$make_command" --no-print-directory integration-tests
