#!/usr/bin/env bash
set -euo pipefail

SCRIPTS_DIR="$(dirname "$0")"
# Provides $OS,$ARCH,$PLAFORM,$ROOT variables
source "$SCRIPTS_DIR/os-env.sh"

make license 2>&1

DIFF=$(git --no-pager diff LICENSE-3rdparty.csv)
if [[ "${DIFF}x" != "x" ]]
then
    echo "License outdated:" >&2
    git --no-pager diff LICENSE-3rdparty.csv >&2
    exit 2
fi

DIFF=$(git ls-files docs/ --exclude-standard --others)
if [[ "${DIFF}x" != "x" ]]
then
    echo "License removed:" >&2
    echo "${DIFF}" >&2
    exit 2
fi
exit 0
