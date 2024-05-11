#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail
set -e

ROOT=$(git rev-parse --show-toplevel)
cd $ROOT

./hack/license.sh

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
    echo ${DIFF} >&2
    exit 2
fi
exit 0
