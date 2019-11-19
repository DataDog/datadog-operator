#!/usr/bin/env bash

set -exo pipefail

export LC_ALL=C

cd $(dirname $0)/..
ROOT=$(git rev-parse --show-toplevel)

$ROOT/bin/wwhrd list

echo Component,Origin,License > LICENSE-3rdparty.csv
echo 'core,"github.com/frapposelli/wwhrd",MIT' >> LICENSE-3rdparty.csv
unset grep
$ROOT/bin/wwhrd list --no-color |& grep "Found License" | awk '{print $6,$5}' | sed -E "s/\x1B\[([0-9]{1,2}(;[0-9]{1,2})?)?[mGK]//g" | sed s/" license="/,/ | sed s/package=/core,/ | sort >> LICENSE-3rdparty.csv
