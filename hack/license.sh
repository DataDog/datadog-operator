#!/usr/bin/env bash
set -euo pipefail

SCRIPTS_DIR="$(dirname "$0")"
# Provides $OS,$ARCH,$PLATFORM,$ROOT variables
source "$SCRIPTS_DIR/os-env.sh"

export LC_ALL=C

cd "$(dirname "$0")/.."

"$ROOT/bin/$PLATFORM/wwhrd" list

echo Component,Origin,License > LICENSE-3rdparty.csv
echo 'core,"github.com/frapposelli/wwhrd",MIT' >> LICENSE-3rdparty.csv
unset grep
"$ROOT/bin/$PLATFORM/wwhrd" list --no-color |& grep "Found License" | awk '{print $6,$5}' | sed -E "s/\x1B\[([0-9]{1,2}(;[0-9]{1,2})?)?[mGK]//g" | sed s/" license="/,/ | sed s/package=/core,/ | sort >> LICENSE-3rdparty.csv
